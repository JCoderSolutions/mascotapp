-- Media: the tenant-scoped record of every object stored in R2.
--
-- The bytes never live here. This table holds the metadata and the storage key;
-- the object itself is uploaded straight to R2 with a pre-signed URL and never
-- passes through the API (§5.5). What this row is, then, is the tenant's claim
-- on an object, and the isolation below is what stops one shelter claiming
-- another's.
--
-- The policy is the template verbatim, with NO substitution -- media is the
-- first table in this schema where `shelter_id` means exactly what it says.

-- +goose Up

-- +goose StatementBegin

CREATE TABLE media (
    id              uuid        NOT NULL PRIMARY KEY,
    shelter_id      uuid        NOT NULL REFERENCES shelters (id),

    -- A closed union, enforced by the database rather than by whoever writes
    -- the insert. §4.2 gives exactly these two.
    kind            text        NOT NULL CHECK (kind IN ('image', 'document')),

    -- The R2 object key. UNIQUE because it IS the object's identity: two rows
    -- pointing at one object would make deleting either one corrupt the other.
    storage_key     text        NOT NULL UNIQUE,

    mime            text        NOT NULL,
    bytes           bigint      NOT NULL,
    width           integer,
    height          integer,
    checksum_sha256 bytea,

    -- {thumb, card, full} -- the derived AVIF/WebP variants. JSONB rather than
    -- three columns because the variant set is a pipeline decision Phase 04
    -- owns and will change; nothing filters on it.
    variants        jsonb       NOT NULL DEFAULT '{}'::jsonb,

    -- WCAG 2.2 AA (AD-4). Nullable because it is written after upload, often by
    -- a different person than the one who uploaded.
    alt_text        text,
    uploaded_by     uuid        REFERENCES users (id),
    created_at      timestamptz NOT NULL DEFAULT now(),

    -- Logical deletion. The row survives so the R2 object can be reaped later
    -- and so a pet's history does not develop holes.
    deleted_at      timestamptz,

    -- D5's composite key. `documents` (T-01-031) needs it for its composite
    -- foreign key on (media_id, shelter_id), and shelters' own logo and cover
    -- keys below need it TODAY. Every parent in a composite tenant foreign key
    -- declares that key in its own migration, so nothing reaches back to ALTER
    -- a table several migrations upstream.
    UNIQUE (id, shelter_id)
);

-- ---------------------------------------------------------------------------
-- Row-level security. The template from 00002, unchanged.
--
-- nullif(current_setting(..., true), '') -- BOTH halves, and the nullif is not
-- decoration. A GUC reverted at COMMIT or ROLLBACK goes back to the EMPTY
-- STRING, not to unset, so on any pooled connection that has already served a
-- scoped request the bare form raises 22P02 instead of returning zero rows.
-- Found by T-01-015; the design's own policy template still shows the bare form
-- and is stale.
-- ---------------------------------------------------------------------------

ALTER TABLE media ENABLE ROW LEVEL SECURITY;
ALTER TABLE media FORCE  ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON media FOR ALL TO app_tenant
    USING      (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid)
    WITH CHECK (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON media TO app_tenant;

-- NO app_public policy here, deliberately. Design §Public surface lists media as
-- public, but a media row is public only THROUGH a published pet, and `pets` is
-- migration 00005. A policy cannot reference a table that does not exist, so the
-- public policy lands there (T-01-019) alongside the table it depends on.

-- ---------------------------------------------------------------------------
-- shelters' deferred foreign keys, promised by 00002 and paid here.
--
-- COMPOSITE, not single-column, and that is the whole point (D5). Referential
-- integrity checks ALWAYS bypass row security: with a plain
-- `REFERENCES media (id)`, shelter B could set its logo to a media row owned by
-- shelter A -- a row B cannot see, cannot read, and would still be publishing.
-- Pointing at `media (id, shelter_id)` and carrying shelters' own `id` as the
-- second column makes that unrepresentable rather than merely forbidden.
--
-- The pair is (logo_media_id, id) because for `shelters` the tenant column IS
-- `id`. MATCH SIMPLE -- the default -- means the constraint is satisfied
-- whenever any column of the key is NULL, which is exactly right here: a shelter
-- with no logo has NULL in logo_media_id and is checked against nothing.
-- ---------------------------------------------------------------------------

ALTER TABLE shelters
    ADD CONSTRAINT shelters_logo_media_fkey
        FOREIGN KEY (logo_media_id, id) REFERENCES media (id, shelter_id),
    ADD CONSTRAINT shelters_cover_media_fkey
        FOREIGN KEY (cover_media_id, id) REFERENCES media (id, shelter_id);

-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin

-- shelters references media, so its constraints come off first. Without this the
-- DROP fails with 2BP01, the same dependency shape the stepwise round-trip test
-- caught in 00002 -- and for the same reason it is spelled out by name rather
-- than swept up by CASCADE, whose blast radius is whatever the schema happens to
-- contain on the day.
ALTER TABLE shelters
    DROP CONSTRAINT shelters_logo_media_fkey,
    DROP CONSTRAINT shelters_cover_media_fkey;

DROP TABLE media;

-- +goose StatementEnd
