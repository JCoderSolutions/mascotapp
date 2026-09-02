-- The files a shelter keeps: contracts, receipts, health certificates.
--
-- A document is a `media` object with meaning attached. The bytes live in R2
-- (§5.5, Phase 04) and `media` already holds the storage key, checksum and
-- variants; this table says WHAT that file is and WHAT it belongs to.
--
-- Two references, both composite, and only one of them required. That asymmetry
-- is the design: every document is a file, not every document belongs to an
-- adoption case.

-- +goose Up

-- +goose StatementBegin

CREATE TABLE documents (
    id             uuid        NOT NULL PRIMARY KEY,
    shelter_id     uuid        NOT NULL REFERENCES shelters (id),

    -- Required. A document with no file is not a document.
    media_id       uuid        NOT NULL,

    -- NULLABLE, and the spec asks for it explicitly: *"A document without an
    -- application is valid"*. A shelter files a health certificate against an
    -- animal long before anybody applies to adopt it, and a receipt may have
    -- nothing to do with an adoption at all.
    --
    -- The composite key below still applies to the rows that DO carry one,
    -- because PostgreSQL's default MATCH SIMPLE skips the whole check when any
    -- column of the key is NULL. `MATCH FULL` would refuse the standalone case
    -- outright -- same columns, same tables, opposite behaviour.
    application_id uuid,

    -- A CLOSED set: Phase 09 picks a PDF template from this value, so a fifth
    -- type reaches a switch with no arm for it and either produces the wrong
    -- document or none.
    type           text        NOT NULL
                               CHECK (type IN ('adoption_contract', 'receipt',
                                               'health_certificate', 'custom')),

    -- What the generator was fed, for a document this platform produced:
    -- template id, the values substituted, the version of the renderer. Untyped
    -- because the shape belongs to the generator, and null for anything a human
    -- uploaded.
    generated_from jsonb,

    -- §11 assumption 5: signing is TRACEABILITY in v1, not a legally binding
    -- signature -- there is no certified e-signature provider. `signature`
    -- records how it was captured (method, ip_hash, user agent); `signed_at`
    -- records when. Both null until it happens.
    signed_at      timestamptz,
    signature      jsonb,

    created_at     timestamptz NOT NULL DEFAULT now(),

    -- D5. `media`'s UNIQUE (id, shelter_id) has existed since 00003, declared
    -- there for exactly this -- so nothing is ALTERed eight migrations upstream.
    --
    -- RESTRICT: a document whose file was deleted is a row pointing at nothing.
    -- Soft deletion is what `media.deleted_at` is for.
    CONSTRAINT documents_media_fkey
        FOREIGN KEY (media_id, shelter_id) REFERENCES media (id, shelter_id)
        ON DELETE RESTRICT,

    -- The second composite key, and the one this schema has now missed twice --
    -- `pet_media`'s media key in T-01-020, `form_submissions`' application key
    -- in T-01-027. The key D5 writes out gets written; the other one looks
    -- finished beside it.
    --
    -- RESTRICT for the same reason as `application_events` and
    -- `application_notes` (00010): 00009 let `form_submissions` CASCADE because
    -- there the child is the PERSONAL DATA, and that is only defensible while
    -- the EVIDENCE does not. An adoption contract is the record of who took
    -- which animal home. A cascade here would erase it while leaving the pet
    -- marked `adopted`, and the shelter would have no record of who has it.
    CONSTRAINT documents_application_fkey
        FOREIGN KEY (application_id, shelter_id)
            REFERENCES adoption_applications (id, shelter_id)
        ON DELETE RESTRICT
);

-- The shelter-side read: one case's paperwork, newest first. Partial, because
-- the standalone documents are found by other routes and a partial index is a
-- fraction of the size (§4.6's own reasoning for the pets filter index).
CREATE INDEX documents_application_created_idx
    ON documents (shelter_id, application_id, created_at DESC)
    WHERE application_id IS NOT NULL;

ALTER TABLE documents ENABLE ROW LEVEL SECURITY;
ALTER TABLE documents FORCE  ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON documents FOR ALL TO app_tenant
    USING      (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid)
    WITH CHECK (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid);

-- No `app_public` grant: a signed contract carries the adopter's name and
-- address. The public catalog shows animals.
--
-- DELETE is granted: a shelter that filed the wrong certificate has to be able
-- to unfile it, and §5.4's subject-deletion path reaches documents too. UPDATE
-- likewise -- correcting a type or attaching a signature is ordinary work. What
-- must NOT be rewritable is the record of what HAPPENED, and that is
-- `application_events` (00010), not this table.
REVOKE TRUNCATE                       ON documents FROM app_tenant, PUBLIC;
GRANT  SELECT, INSERT, UPDATE, DELETE ON documents TO   app_tenant;

-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin

DROP TABLE documents;

-- +goose StatementEnd
