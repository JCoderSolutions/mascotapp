-- Pets: the catalog core, and the first table with two policies for two roles.
--
-- The filter attributes are TYPED COLUMNS, never JSON (AD-3). size,
-- energy_level and good_with_* are the axis of adopter search, and a catalog
-- that cannot index its own filters is a catalog that stops working at fifty
-- animals. JSONB is for the shape nobody queries by; this is not that.
--
-- Two permissive policies live here and they OR together, which is safe only
-- because each is scoped TO a different role: tenant_isolation to app_tenant,
-- public_catalog to app_public. Role scoping is what keeps the union from
-- widening either one.

-- +goose Up

-- +goose StatementBegin

CREATE TABLE pets (
    id                   uuid        NOT NULL PRIMARY KEY,
    shelter_id           uuid        NOT NULL REFERENCES shelters (id),

    -- The code a shelter puts on a kennel card and an adopter quotes on the
    -- phone. Unique per shelter, not globally: two shelters both numbering from
    -- 001 is normal, and forcing them into one namespace would leak the size of
    -- the platform to every tenant.
    public_code          text        NOT NULL,

    name                 text        NOT NULL,
    species_id           uuid        NOT NULL REFERENCES species (id),

    -- NULL is the common case, not the exception: most animals a shelter takes
    -- in have no known breed, which is what breed_note is for.
    breed_id             uuid        REFERENCES breeds (id),
    breed_note           text,

    sex                  text        NOT NULL DEFAULT 'unknown'
                                     CHECK (sex IN ('male', 'female', 'unknown')),
    size                 text        NOT NULL
                                     CHECK (size IN ('xs', 's', 'm', 'l', 'xl')),

    birth_date_estimate  date,
    age_precision        text        NOT NULL DEFAULT 'estimated'
                                     CHECK (age_precision IN ('exact', 'month', 'year',
                                                              'estimated')),
    weight_kg            numeric(5, 2),

    -- The lifecycle (LT-5). A pet leaves the catalog by changing status, never
    -- by disappearing: pet_status_history (00006) keeps the trail.
    status               text        NOT NULL DEFAULT 'draft'
                                     CHECK (status IN ('draft', 'available', 'reserved',
                                                       'in_process', 'adopted',
                                                       'unavailable', 'deceased')),
    status_changed_at    timestamptz NOT NULL DEFAULT now(),

    -- Nullable booleans on purpose: NULL means "not known", which for an animal
    -- that arrived last night is the truth. NOT NULL DEFAULT false would record
    -- "not vaccinated" for every intake and no adopter could tell the two apart.
    sterilized           boolean,
    vaccinated           boolean,
    dewormed             boolean,

    microchip_id         text,

    energy_level         text        NOT NULL DEFAULT 'medium'
                                     CHECK (energy_level IN ('low', 'medium', 'high')),

    -- Three-state, exactly as §4.3 writes them. "unknown" is a real answer for a
    -- dog nobody has tested around cats, and collapsing it into "no" costs
    -- adoptions.
    good_with_kids       text        NOT NULL DEFAULT 'unknown'
                                     CHECK (good_with_kids IN ('yes', 'no', 'unknown')),
    good_with_dogs       text        NOT NULL DEFAULT 'unknown'
                                     CHECK (good_with_dogs IN ('yes', 'no', 'unknown')),
    good_with_cats       text        NOT NULL DEFAULT 'unknown'
                                     CHECK (good_with_cats IN ('yes', 'no', 'unknown')),

    special_needs        boolean     NOT NULL DEFAULT false,
    special_needs_note   text,

    story                text,
    description          text,

    intake_date          date,
    intake_reason        text,

    adoption_fee_cents   integer     NOT NULL DEFAULT 0 CHECK (adoption_fee_cents >= 0),
    adoption_fee_currency text       NOT NULL DEFAULT 'MXN'
                                     CHECK (length(adoption_fee_currency) = 3),

    -- The publication gate. NULL means never published; the public policy below
    -- reads it, and clearing it retracts a pet without deleting it.
    published_at         timestamptz,

    created_by           uuid        REFERENCES users (id),
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    deleted_at           timestamptz,

    UNIQUE (shelter_id, public_code),

    -- Per shelter, and deliberately NOT global. A microchip number IS globally
    -- unique in the world, so a global constraint here would be the "correct"
    -- modelling -- and it would turn every INSERT into an existence oracle:
    -- tenant A types a chip number, gets 23505, and has learned that some other
    -- shelter holds that animal. Uniqueness violations are reported before any
    -- policy is consulted, exactly like the forged-insert path T-01-013 proved.
    -- Cross-shelter duplicate detection is a job for a moderation view under the
    -- owner role, not for a constraint every tenant can probe.
    UNIQUE (shelter_id, microchip_id),

    -- D5's composite key, for pet_media, pet_health_records and
    -- pet_status_history in 00006. Declared here, in the parent's own migration.
    UNIQUE (id, shelter_id)
);

-- §4.6, verbatim. The first serves the shelter's own dashboard; the second is
-- the adopter's filter, partial because the public catalog only ever reads
-- available pets and a partial index is a fraction of the size.
CREATE INDEX pets_shelter_status_published_idx
    ON pets (shelter_id, status, published_at DESC);

CREATE INDEX pets_public_filter_idx
    ON pets (species_id, size, energy_level)
    WHERE status = 'available';

ALTER TABLE pets ENABLE ROW LEVEL SECURITY;
ALTER TABLE pets FORCE  ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON pets FOR ALL TO app_tenant
    USING      (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid)
    WITH CHECK (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid);

-- The public catalog. Three conditions, and every one of them is a way a pet
-- leaves the catalog without being destroyed: it is no longer available, it was
-- retracted, or it was logically deleted.
--
-- No tenant scope is read here at all. app_public never sets app.shelter_id, so
-- this policy spans shelters by construction -- which is the product
-- requirement, not an oversight.
CREATE POLICY public_catalog ON pets FOR SELECT TO app_public
    USING (status = 'available'
           AND published_at IS NOT NULL
           AND deleted_at IS NULL);

GRANT SELECT, INSERT, UPDATE, DELETE ON pets TO app_tenant;
GRANT SELECT                        ON pets TO app_public;

-- ---------------------------------------------------------------------------
-- `media`'s public policy is NOT here, and this is the second time it has been
-- scheduled against a dependency it does not have.
--
-- Design §Public surface put it in 00003. T-01-017 moved it to 00005 on the
-- grounds that it depends on `pets`. It does not: the spec's own scenario is
-- "media M is ATTACHED to a draft pet", and attachment is `pet_media`, which is
-- migration 00006. A policy cannot reference a table that does not exist, so it
-- lands there, with the table that makes the join expressible.
--
-- Until then `media` has no app_public policy and no app_public grant, so the
-- public role sees no media at all. That is the safe direction to be wrong in,
-- and TestMedia_GetsItsPublicPolicyWhenPetMediaLands fails the day 00006 lands
-- without it.
-- ---------------------------------------------------------------------------

-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin

-- Policies, indexes and grants go with the table. Nothing references pets yet --
-- its children arrive in 00006 -- so there is no constraint to drop first.
DROP TABLE pets;

-- +goose StatementEnd
