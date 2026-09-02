-- The two things that hang off an adoption case, and they are NOT the same kind
-- of thing. That difference is the whole design of this migration.
--
--   * `application_events` is the TRAIL: what happened, when, and who did it.
--     Append-only, enforced by four layers, because a record of what happened
--     that can be rewritten is not a record of what happened.
--   * `application_notes` is WORKING MEMORY: a person writing "called, no
--     answer". Fully editable, because people make typos and change their minds.
--
-- Treating notes as evidence would make the shelter's own scratchpad immutable;
-- treating events as notes would leave the audit trail rewritable by whoever
-- wants it to say something else. §4.5 names `application_events` and
-- `audit_log` as the append-only pair, and this migration builds the first.
--
-- BOTH reference `adoption_applications` with ON DELETE RESTRICT, and that is
-- not the default choice repeated out of habit -- it is what makes 00009's
-- CASCADE safe. See the comment on the keys below.

-- +goose Up

-- +goose StatementBegin

-- ---------------------------------------------------------------------------
-- application_events -- the append-only timeline.
-- ---------------------------------------------------------------------------

CREATE TABLE application_events (
    id             uuid        NOT NULL PRIMARY KEY,
    shelter_id     uuid        NOT NULL REFERENCES shelters (id),

    -- Denormalised from `adoption_applications`, and paired with it below (D5).
    application_id uuid        NOT NULL,

    -- NOT a closed union, and this is a deliberate departure from the habit of
    -- this schema rather than an omission.
    --
    -- `pets.status` and `adoption_applications.status` are CHECKed because the
    -- domain layer BRANCHES on them: a value outside the set reaches a switch
    -- with no arm for it. An event type is the opposite shape -- the domain
    -- EMITS it, a reader that does not recognise one ignores it, and the set
    -- grows with every feature that records something. A CHECK here would mean a
    -- migration per new event type, which is how a timeline stops being written.
    type           text        NOT NULL CHECK (length(type) BETWEEN 1 AND 64),

    -- Whatever the event carries. Untyped on purpose: the shape belongs to the
    -- event type, which the database does not interpret.
    payload        jsonb       NOT NULL DEFAULT '{}'::jsonb,

    -- Nullable: some events have no human actor -- a retention purge, an
    -- expiry, anything a job emits.
    actor_user_id  uuid        REFERENCES users (id),

    occurred_at    timestamptz NOT NULL DEFAULT now(),

    -- D5, and RESTRICT is the load-bearing half.
    --
    -- 00009 gave `form_submissions.application_id` ON DELETE CASCADE on purpose:
    -- there the child is the PERSONAL DATA and §5.4's retention purge is
    -- precisely the answers going away. That choice is only safe while the
    -- EVIDENCE lives somewhere that does not cascade -- and this is that
    -- somewhere. With both cascading, purging a rejected solicitud would erase
    -- the record that it ever existed: who reviewed it, when it was rejected,
    -- what was decided. The trail would vanish through a door nobody was
    -- watching, exactly as `pet_status_history` would have in T-01-020.
    --
    -- The consequence is intended: an application with a timeline cannot be hard
    -- deleted. §5.4's purge removes the personal data; the fact that a case
    -- existed is not personal data and is what an audit is made of.
    CONSTRAINT application_events_application_fkey
        FOREIGN KEY (application_id, shelter_id)
            REFERENCES adoption_applications (id, shelter_id)
        ON DELETE RESTRICT
);

-- The timeline read: one application's events, oldest first.
CREATE INDEX application_events_application_occurred_idx
    ON application_events (shelter_id, application_id, occurred_at);

ALTER TABLE application_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE application_events FORCE  ROW LEVEL SECURITY;

-- Layer 1, the policies: TWO per-command policies instead of one FOR ALL. The
-- ABSENCE of an UPDATE or DELETE policy is the point -- under FORCE, a command
-- with no permissive policy matches zero rows, for the owner too.
--
-- Each clause is written out explicitly. A `FOR ALL` policy infers WITH CHECK
-- from USING; split like this, that inference is gone, and a policy relying on
-- it would silently allow any INSERT.
CREATE POLICY tenant_read ON application_events FOR SELECT TO app_tenant
    USING (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid);

CREATE POLICY tenant_append ON application_events FOR INSERT TO app_tenant
    WITH CHECK (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid);

-- Layer 2, the grants. REVOKE first and from PUBLIC too: a privilege that was
-- never granted still reads as deliberate here, and PUBLIC is the default
-- grantee people forget.
REVOKE UPDATE, DELETE, TRUNCATE ON application_events FROM app_tenant, PUBLIC;
GRANT  SELECT, INSERT            ON application_events TO   app_tenant;

-- Layer 3, the trigger -- not redundant with the two above.
--
-- Neither the grant nor the missing policy survives a role with BYPASSRLS, and
-- on Neon that is not hypothetical: `neon_superuser` carries it. RLS is bypassed
-- by such roles; TRIGGERS ARE NOT. This is the only layer that holds under a
-- mis-provisioned role.
--
-- It also turns a SILENT zero-row result -- which application code readily
-- misreads as success -- into a loud error. The default SQLSTATE of a plpgsql
-- RAISE is P0001, deliberately distinct from the grant's 42501 so a test can
-- tell which layer fired.
CREATE FUNCTION application_events_is_append_only() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION
        'application_events is append-only: % is refused. The timeline of an adoption '
        'case is what an audit reads; append a correcting event instead', TG_OP;
END;
$$;

CREATE TRIGGER application_events_no_rewrite
    BEFORE UPDATE OR DELETE ON application_events
    FOR EACH ROW EXECUTE FUNCTION application_events_is_append_only();

-- Layer 4, separately, because TRUNCATE is the one write no row-level policy can
-- ever see: it removes every row without visiting any, so USING and WITH CHECK
-- are never consulted and a FOR EACH ROW trigger never fires.
CREATE TRIGGER application_events_no_truncate
    BEFORE TRUNCATE ON application_events
    FOR EACH STATEMENT EXECUTE FUNCTION application_events_is_append_only();

-- ---------------------------------------------------------------------------
-- application_notes -- working memory, deliberately mutable.
-- ---------------------------------------------------------------------------

CREATE TABLE application_notes (
    id               uuid        NOT NULL PRIMARY KEY,
    shelter_id       uuid        NOT NULL REFERENCES shelters (id),
    application_id   uuid        NOT NULL,

    -- Nullable for the same reason as the event's actor: a note can be written
    -- by a job -- a summary, an import -- rather than by a person.
    author_user_id   uuid        REFERENCES users (id),

    body             text        NOT NULL,

    -- A CLOSED set, unlike the event type above, and the reason is the one that
    -- decides every closed union in this schema: a later phase BRANCHES on this
    -- value to decide who may read the note. A third value reaches a switch with
    -- no arm for it, and whatever the fallback does decides whether an adopter
    -- sees what the shelter wrote about them.
    --
    -- This phase only STORES it. Enforcing the visibility is a later phase.
    visibility       text        NOT NULL DEFAULT 'internal'
                                 CHECK (visibility IN ('internal', 'shared')),

    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),

    -- Same RESTRICT, same reason -- with one addition specific to notes. A note
    -- is where a shelter records WHY it decided something, so a purge that took
    -- the notes with it would remove the reasoning behind a rejection while
    -- leaving the rejection itself.
    CONSTRAINT application_notes_application_fkey
        FOREIGN KEY (application_id, shelter_id)
            REFERENCES adoption_applications (id, shelter_id)
        ON DELETE RESTRICT
);

CREATE INDEX application_notes_application_created_idx
    ON application_notes (shelter_id, application_id, created_at DESC);

ALTER TABLE application_notes ENABLE ROW LEVEL SECURITY;
ALTER TABLE application_notes FORCE  ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON application_notes FOR ALL TO app_tenant
    USING      (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid)
    WITH CHECK (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid);

-- Full DML: editing and deleting a note is ordinary work, not history being
-- rewritten. TRUNCATE is still revoked -- no row-level mechanism sees it coming,
-- and nothing in this product ever needs to empty a table in one statement.
REVOKE TRUNCATE                       ON application_notes FROM app_tenant, PUBLIC;
GRANT  SELECT, INSERT, UPDATE, DELETE ON application_notes TO   app_tenant;

-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin

DROP TABLE application_notes;
DROP TABLE application_events;
DROP FUNCTION application_events_is_append_only();

-- +goose StatementEnd
