-- The schema-wide audit trail: who changed what, and what it looked like before.
--
-- Distinct from `application_events` (00010) on purpose, and the difference is
-- WHO the record is for. An application event is DOMAIN history a shelter reads
-- in its own timeline — "assigned to Marta", "home visit scheduled". An audit row
-- is FORENSIC: it exists so a question nobody has asked yet can be answered
-- later, and it records the shape of the row on both sides of the change.
--
-- Both are append-only under the same four layers, and from T-01-032 both are
-- enumerated from `rlstest.Schema.AppendOnly` rather than checked one at a time.

-- +goose Up

-- +goose StatementBegin

CREATE TABLE audit_log (
    -- BIGSERIAL, not uuid, and it is the one identifier in this schema that is
    -- not. §4.5 asks for a MONOTONIC id because the log is read in id order and
    -- that order has to be the order things happened. UUIDv7 is time-ordered by
    -- construction but only to millisecond resolution -- two writes inside the
    -- same millisecond have no defined order -- and this table is written by
    -- triggers and batch jobs that do exactly that.
    --
    -- The cost is the sequence grant at the bottom, which a table grant does not
    -- cover. See the comment there.
    id            bigserial   PRIMARY KEY,

    shelter_id    uuid        NOT NULL REFERENCES shelters (id),

    -- Nullable: a retention purge, an expiry job, a migration. Not everything
    -- that changes a row is a person.
    actor_user_id uuid        REFERENCES users (id),

    -- What was done, in the domain's own words: 'pet.published',
    -- 'application.rejected'. NOT a closed set, for the same reason
    -- `application_events.type` is not: the domain EMITS these and the set grows
    -- with every feature. A CHECK would mean a migration per action, which is
    -- how an audit log stops being written.
    action        text        NOT NULL CHECK (length(action) BETWEEN 1 AND 128),

    -- POLYMORPHIC, and deliberately unkeyed. `entity_type` says which table and
    -- `entity_id` says which row, so no foreign key can express it: a key names
    -- ONE table, so it would either refuse every audit row about anything else
    -- or become a single-column cross-tenant reference (D5).
    --
    -- What contains the hole is that this is a LOG, not a reference. Nothing
    -- resolves `entity_id` to fetch the thing, and the row is tenant-scoped by
    -- its own `shelter_id` -- so a tenant writing another shelter's id in here
    -- learns nothing it did not already type.
    entity_type   text        NOT NULL CHECK (length(entity_type) BETWEEN 1 AND 64),
    entity_id     uuid,

    -- The shape of the row on both sides. Nullable because a creation has no
    -- before and a deletion has no after.
    "before"      jsonb,
    "after"       jsonb,

    -- §5.4: the address is never stored, only a salted hash. The column name is
    -- the contract -- a plain `ip` here would be personal data by another name.
    ip_hash       text,
    user_agent    text,

    occurred_at   timestamptz NOT NULL DEFAULT now()
);

-- §4.6. Every audit read is one shelter's activity, newest first, and under RLS
-- the `shelter_id` predicate is on every query whether the caller wrote it or
-- not -- so it leads.
CREATE INDEX audit_log_shelter_occurred_idx
    ON audit_log (shelter_id, occurred_at DESC);

ALTER TABLE audit_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log FORCE  ROW LEVEL SECURITY;

-- Layer 1: TWO per-command policies. The ABSENCE of an UPDATE or DELETE policy
-- is the enforcement -- under FORCE, a command with no permissive policy matches
-- zero rows, for the owner too. Each clause is written out because a FOR ALL
-- policy's inference of WITH CHECK from USING is gone once it is split.
CREATE POLICY tenant_read ON audit_log FOR SELECT TO app_tenant
    USING (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid);

CREATE POLICY tenant_append ON audit_log FOR INSERT TO app_tenant
    WITH CHECK (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid);

-- Layer 2, the grants.
REVOKE UPDATE, DELETE, TRUNCATE ON audit_log FROM app_tenant, PUBLIC;
GRANT  SELECT, INSERT            ON audit_log TO   app_tenant;

-- Layer 2b, and it is the one this migration exists to not forget.
--
-- `bigserial` is not a type: it is `bigint` plus a SEQUENCE plus a default that
-- calls `nextval` on it. A table grant says nothing about that sequence, so
-- without this line `app_tenant` can insert only when it supplies an id itself
-- -- which every test that writes explicit ids does, and which no real caller
-- does. The failure is invisible in development and arrives on the first
-- production write.
--
-- USAGE, not ALL: `nextval` and `currval` are what an inserter needs. `UPDATE`
-- on a sequence is `setval`, which would let a tenant rewind the counter and
-- make two audit rows share an id.
GRANT USAGE ON SEQUENCE audit_log_id_seq TO app_tenant;

-- Layer 3, the row trigger. Neither the grant nor the missing policy survives a
-- role with BYPASSRLS, and on Neon that is not hypothetical: `neon_superuser`
-- carries it. RLS is bypassed by such roles; TRIGGERS ARE NOT.
--
-- The RAISE's default SQLSTATE is P0001, deliberately distinct from the grant's
-- 42501 so a test can tell which layer fired.
CREATE FUNCTION audit_log_is_append_only() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION
        'audit_log is append-only: % is refused. An audit trail that can be edited '
        'answers no question it was written to answer', TG_OP;
END;
$$;

CREATE TRIGGER audit_log_no_rewrite
    BEFORE UPDATE OR DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_is_append_only();

-- Layer 4, separately, because TRUNCATE removes every row without visiting any:
-- USING and WITH CHECK are never consulted and a FOR EACH ROW trigger never
-- fires. Only a statement-level trigger reaches it.
CREATE TRIGGER audit_log_no_truncate
    BEFORE TRUNCATE ON audit_log
    FOR EACH STATEMENT EXECUTE FUNCTION audit_log_is_append_only();

-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin

DROP TABLE audit_log;
DROP FUNCTION audit_log_is_append_only();

-- +goose StatementEnd
