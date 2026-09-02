-- Pets' three children, and the first append-only table in this schema.
--
-- All three follow D5: a denormalised `shelter_id` AND a COMPOSITE foreign key
-- to `pets (id, shelter_id)`. The denormalised column alone is worse than
-- nothing here. Referential integrity checks ALWAYS bypass row security, so with
-- a plain `pet_id REFERENCES pets (id)` tenant B could insert a row carrying B's
-- own shelter_id and A's pet_id: the foreign key check passes because it never
-- consults a policy, and the WITH CHECK passes because the shelter_id really is
-- B's. B has just attached a row to another tenant's animal. The composite key
-- makes that unrepresentable rather than merely forbidden, because
-- (A's pet, B's shelter) is not a row in `pets`.
--
-- It also closes an existence oracle: a single-column key answers "does this
-- uuid exist in some other tenant?" through the difference between success and
-- violation. The composite key returns the same 23503 for a foreign id and for
-- an id that never existed.

-- +goose Up

-- +goose StatementBegin

-- ---------------------------------------------------------------------------
-- pet_media -- the join row, and the only table here whose identity is a PAIR.
-- §4.3 gives it no `id`, and a pure join row needs no separate one: the pet and
-- the media ARE the fact being recorded.
-- ---------------------------------------------------------------------------

CREATE TABLE pet_media (
    pet_id     uuid        NOT NULL,
    media_id   uuid        NOT NULL,
    shelter_id uuid        NOT NULL REFERENCES shelters (id),

    -- Gallery order, chosen by the shelter. Not unique: reordering a gallery by
    -- rewriting positions one row at a time would collide halfway through.
    position   integer     NOT NULL DEFAULT 0,
    is_primary boolean     NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (pet_id, media_id),

    CONSTRAINT pet_media_pet_fkey
        FOREIGN KEY (pet_id, shelter_id) REFERENCES pets (id, shelter_id)
        ON DELETE CASCADE,

    -- The media side needs the same treatment for the same reason: without it a
    -- tenant could attach another shelter's photo to its own pet.
    CONSTRAINT pet_media_media_fkey
        FOREIGN KEY (media_id, shelter_id) REFERENCES media (id, shelter_id)
        ON DELETE CASCADE
);

-- One cover photo per pet -- keyed on (shelter_id, pet_id) and deliberately NOT
-- on pet_id alone.
--
-- `UNIQUE (pet_id) WHERE is_primary` is the natural spelling and it is a
-- cross-tenant existence oracle, the same shape as a global UNIQUE on
-- pets.microchip_id (T-01-019). Uniqueness is checked before any policy, so
-- tenant B could name tenant A's pet and read the answer off the SQLSTATE:
-- 23505 means that pet already has a cover photo, 23503 means it does not exist
-- or is not B's. Two different answers to a question B has no right to ask.
--
-- Adding shelter_id keeps the guarantee identical inside a tenant -- a pet
-- belongs to exactly one shelter, so per-shelter uniqueness over its
-- attachments IS per-pet uniqueness -- and collapses the two SQLSTATEs into one
-- for everybody else.
CREATE UNIQUE INDEX pet_media_one_primary_idx
    ON pet_media (shelter_id, pet_id)
    WHERE is_primary;

CREATE INDEX pet_media_pet_position_idx ON pet_media (shelter_id, pet_id, position);

ALTER TABLE pet_media ENABLE ROW LEVEL SECURITY;
ALTER TABLE pet_media FORCE  ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON pet_media FOR ALL TO app_tenant
    USING      (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid)
    WITH CHECK (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid);

-- The public half of the chain. It says only "there is a pet" -- and under
-- app_public that nested lookup is itself filtered by `public_catalog` on
-- `pets`, so it resolves to "there is a PUBLIC pet" without repeating what
-- public means. Same mechanism as D6's users-through-memberships.
CREATE POLICY public_catalog ON pet_media FOR SELECT TO app_public
    USING (EXISTS (SELECT 1 FROM pets p WHERE p.id = pet_media.pet_id));

GRANT SELECT, INSERT, UPDATE, DELETE ON pet_media TO app_tenant;
GRANT SELECT                        ON pet_media TO app_public;

-- ---------------------------------------------------------------------------
-- pet_health_records -- the animal's medical file. Tenant-only: the public
-- catalog shows the animal, not its case file.
-- ---------------------------------------------------------------------------

CREATE TABLE pet_health_records (
    id                uuid        NOT NULL PRIMARY KEY,
    shelter_id        uuid        NOT NULL REFERENCES shelters (id),
    pet_id            uuid        NOT NULL,

    type              text        NOT NULL
                                  CHECK (type IN ('vaccine', 'deworming', 'surgery',
                                                  'treatment', 'checkup')),
    occurred_on       date        NOT NULL,
    description       text,
    vet_name          text,

    -- The scan of the vaccination card or the surgery report. COMPOSITE for the
    -- same D5 reason as the parent key, and this is the reference that is easy
    -- to miss: the key to `pets` is the one the design writes out, so a
    -- single-column `REFERENCES media (id)` here would look finished while
    -- letting a tenant link another shelter's document. MATCH SIMPLE -- the
    -- default -- means a NULL document is checked against nothing.
    document_media_id uuid,

    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT pet_health_records_pet_fkey
        FOREIGN KEY (pet_id, shelter_id) REFERENCES pets (id, shelter_id)
        ON DELETE CASCADE,

    CONSTRAINT pet_health_records_document_fkey
        FOREIGN KEY (document_media_id, shelter_id) REFERENCES media (id, shelter_id)
);

CREATE INDEX pet_health_records_pet_occurred_idx
    ON pet_health_records (shelter_id, pet_id, occurred_on DESC);

ALTER TABLE pet_health_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE pet_health_records FORCE  ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON pet_health_records FOR ALL TO app_tenant
    USING      (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid)
    WITH CHECK (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid);

-- Full write access on purpose: a vet's typo is corrected in place. These are
-- records, not history, and the distinction is exactly what the next table is.
GRANT SELECT, INSERT, UPDATE, DELETE ON pet_health_records TO app_tenant;

-- ---------------------------------------------------------------------------
-- pet_status_history -- APPEND-ONLY. The trail of how an animal moved through
-- the shelter, and the first immutable table in this schema.
--
-- LT-5 makes it immutable from day one: a pet leaves the catalog by CHANGING
-- STATUS, never by disappearing, and a trail its own author can rewrite records
-- nothing. Four layers, each stopping a different actor.
-- ---------------------------------------------------------------------------

CREATE TABLE pet_status_history (
    id            uuid        NOT NULL PRIMARY KEY,
    shelter_id    uuid        NOT NULL REFERENCES shelters (id),
    pet_id        uuid        NOT NULL,

    -- Nullable: the first row of a pet's life has no previous status.
    from_status   text        CHECK (from_status IN ('draft', 'available', 'reserved',
                                                     'in_process', 'adopted',
                                                     'unavailable', 'deceased')),
    to_status     text        NOT NULL
                              CHECK (to_status IN ('draft', 'available', 'reserved',
                                                   'in_process', 'adopted',
                                                   'unavailable', 'deceased')),
    reason        text,
    actor_user_id uuid        REFERENCES users (id),
    occurred_at   timestamptz NOT NULL DEFAULT now(),

    -- RESTRICT, NOT CASCADE, and this is the difference between append-only and
    -- theatre. app_tenant holds DELETE on `pets`; with a cascading key a shelter
    -- would erase an animal's whole trail by deleting the animal -- never
    -- touching this table, never tripping the trigger below, straight through
    -- the one door nobody was watching.
    --
    -- It is not a workaround bolted on the side either. LT-5's own rule is that
    -- a pet leaves by changing status, so a pet with a recorded history is a pet
    -- that can only be soft-deleted, which is what the product wanted anyway.
    CONSTRAINT pet_status_history_pet_fkey
        FOREIGN KEY (pet_id, shelter_id) REFERENCES pets (id, shelter_id)
        ON DELETE RESTRICT
);

CREATE INDEX pet_status_history_pet_occurred_idx
    ON pet_status_history (shelter_id, pet_id, occurred_at DESC);

ALTER TABLE pet_status_history ENABLE ROW LEVEL SECURITY;
ALTER TABLE pet_status_history FORCE  ROW LEVEL SECURITY;

-- Layer 1, the policies: TWO per-command policies instead of one FOR ALL. The
-- ABSENCE of an UPDATE or DELETE policy is the point -- under FORCE, a command
-- with no permissive policy matches zero rows, for the owner too. This is why
-- WITH CHECK is always written explicitly in this schema: the inference a
-- FOR ALL policy gets vanishes the moment it is split like this.
CREATE POLICY tenant_read ON pet_status_history FOR SELECT TO app_tenant
    USING (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid);

CREATE POLICY tenant_append ON pet_status_history FOR INSERT TO app_tenant
    WITH CHECK (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid);

-- Layer 2, the grants. REVOKE first and from PUBLIC too: a privilege that was
-- never granted still reads as deliberate here, and PUBLIC is the default
-- grantee people forget.
REVOKE UPDATE, DELETE, TRUNCATE ON pet_status_history FROM app_tenant, PUBLIC;
GRANT  SELECT, INSERT            ON pet_status_history TO   app_tenant;

-- Layer 3, the trigger -- and it is not redundant with the two above.
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
CREATE FUNCTION pet_status_history_is_append_only() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION
        'pet_status_history is append-only: % is refused (LT-5)', TG_OP;
END;
$$;

CREATE TRIGGER pet_status_history_no_rewrite
    BEFORE UPDATE OR DELETE ON pet_status_history
    FOR EACH ROW EXECUTE FUNCTION pet_status_history_is_append_only();

-- Layer 4, separately, because TRUNCATE is the one write no row-level policy can
-- ever see: it removes every row without visiting any, so USING and WITH CHECK
-- are never consulted. Only a statement-level trigger reaches it.
CREATE TRIGGER pet_status_history_no_truncate
    BEFORE TRUNCATE ON pet_status_history
    FOR EACH STATEMENT EXECUTE FUNCTION pet_status_history_is_append_only();

-- ---------------------------------------------------------------------------
-- media's public policy -- deferred through 00003 and 00005, paid HERE.
--
-- Design §Public surface put it in 00003; T-01-017 moved it to 00005 believing
-- it depended on `pets`. It depends on the ATTACHMENT: the spec's scenario is
-- "media M is attached only to a draft pet", and attachment is `pet_media`,
-- which this migration creates. This is its third scheduling and its first
-- correct one.
--
-- It says only "there is an attachment", and lets the chain do the rest:
-- pet_media's own public policy is what narrows that to a public pet, and
-- pets' public_catalog is what defines public. So "a public pet" is defined in
-- exactly ONE place and a photo follows its pet automatically -- retract the
-- pet and the photo goes with it, with nobody touching the media row.
--
-- `deleted_at IS NULL` is the one condition media owns rather than inherits:
-- a soft-deleted photo must leave the catalog even while its pet stays
-- published, and nothing upstream in the chain would ever hide it.
-- ---------------------------------------------------------------------------

CREATE POLICY public_catalog ON media FOR SELECT TO app_public
    USING (deleted_at IS NULL
           AND EXISTS (SELECT 1 FROM pet_media pm WHERE pm.media_id = media.id));

GRANT SELECT ON media TO app_public;

-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin

-- media's public policy is dropped by name, and BEFORE pet_media, for the same
-- 2BP01 dependency the 00002 round-trip found: a policy that reads another table
-- makes PostgreSQL record a dependency on it, and dropping that table fails
-- while the policy stands. By name rather than by CASCADE, whose blast radius is
-- whatever the schema happens to contain on the day.
DROP POLICY public_catalog ON media;
REVOKE SELECT ON media FROM app_public;

-- The triggers go with their table; the function does not, so it is dropped
-- explicitly. A leftover function would make a re-run of Up fail with 42723.
DROP TABLE pet_status_history;
DROP FUNCTION pet_status_history_is_append_only();

DROP TABLE pet_health_records;
DROP TABLE pet_media;

-- +goose StatementEnd
