-- Dynamic forms, part two: the recorded answers.
--
-- A submission is BOUND TO A VERSION, not to a template, and that binding is the
-- reason 00007 froze published versions. The pair is what keeps historical data
-- readable: the answers say WHAT was answered, the version says WHAT WAS ASKED,
-- and neither means anything without the other.
--
-- This table holds regulated personal data. §5.4 puts the sensitive half in
-- `answers_encrypted` under application-level AES-256-GCM, which is PHASE 07 --
-- what lands here is the column and nothing that pretends to be encryption.

-- +goose Up

-- +goose StatementBegin

-- The referenced key of the composite foreign key below (D5).
--
-- It is declared HERE rather than in 00007 for the ordinary reason a migration
-- is append-only once applied: 00007 has run. It is also where the need appears
-- -- `form_templates` got its own key in 00007 because its child was in the same
-- migration, and this one's child is here.
ALTER TABLE form_template_versions
    ADD CONSTRAINT form_template_versions_id_shelter_key UNIQUE (id, shelter_id);

CREATE TABLE form_submissions (
    id                   uuid        NOT NULL PRIMARY KEY,
    shelter_id           uuid        NOT NULL REFERENCES shelters (id),
    template_version_id  uuid        NOT NULL,

    -- No foreign key yet: `adoption_applications` lands at T-01-027 and a
    -- reference cannot name a table that does not exist. When it does, this
    -- needs the COMPOSITE key to (id, shelter_id) like every other reference in
    -- this schema. TestFormSubmissions_GetsItsApplicationKeyWhenApplicationsLand
    -- asserts the equivalence in BOTH directions so the deferral cannot survive
    -- by being forgotten, the way media's public policy did through two
    -- migrations.
    application_id       uuid,

    -- Nullable: an adopter fills a form through a magic link and §4.1 gives that
    -- user no password. An anonymous submission is still a submission.
    submitted_by_user_id uuid        REFERENCES users (id),

    -- The answer document, keyed by `field.id`. Those ids are immutable for the
    -- life of a template (§4.4 rule 2), which is what lets an answer recorded
    -- against v1 still resolve after v2 drops the field.
    answers              jsonb       NOT NULL DEFAULT '{}'::jsonb,

    -- Phase 07 fills this. It is declared now so the shape of the record does
    -- not change under a table that already holds data.
    answers_encrypted    bytea,

    submitted_at         timestamptz NOT NULL DEFAULT now(),

    -- Hashed with a salt, never the address (§5.4). The column name is the
    -- contract: a plain `ip` here would be personal data by another name.
    ip_hash              text,

    -- D5. A plain `template_version_id REFERENCES form_template_versions (id)`
    -- would resolve another shelter's version on this tenant's behalf, because
    -- referential integrity checks ALWAYS BYPASS ROW SECURITY.
    --
    -- RESTRICT: a version with recorded answers is a version something still
    -- renders against. A published version already cannot be deleted at all
    -- (00007's trigger); this closes the remaining door, which is a DRAFT
    -- version that somehow collected submissions.
    CONSTRAINT form_submissions_version_fkey
        FOREIGN KEY (template_version_id, shelter_id)
            REFERENCES form_template_versions (id, shelter_id)
        ON DELETE RESTRICT
);

-- §4.6. GIN, not btree, and that is the whole point: a btree index on a jsonb
-- column is accepted by PostgreSQL, reads as an index in the catalog, and serves
-- no containment query at all. The adopter-side search is `answers @> ...`.
CREATE INDEX form_submissions_answers_idx
    ON form_submissions USING GIN (answers);

-- The shelter-side listing: a refuge reads its own submissions newest first.
CREATE INDEX form_submissions_shelter_submitted_idx
    ON form_submissions (shelter_id, submitted_at DESC);

ALTER TABLE form_submissions ENABLE ROW LEVEL SECURITY;
ALTER TABLE form_submissions FORCE  ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON form_submissions FOR ALL TO app_tenant
    USING      (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid)
    WITH CHECK (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid);

-- DELETE is granted deliberately: §5.4 requires a retention policy -- rejected
-- applications are purged after N months -- and a subject can ask for their data
-- to be removed. A table nobody can delete from cannot honour either.
--
-- TRUNCATE is not, for the same reason it is revoked everywhere else in this
-- schema: no row-level mechanism can see it coming.
REVOKE TRUNCATE                       ON form_submissions FROM app_tenant, PUBLIC;
GRANT  SELECT, INSERT, UPDATE, DELETE ON form_submissions TO   app_tenant;

-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin

DROP TABLE form_submissions;
ALTER TABLE form_template_versions
    DROP CONSTRAINT form_template_versions_id_shelter_key;

-- +goose StatementEnd
