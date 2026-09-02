-- Reference data: species and breeds. Global, read-only, and the same rows for
-- every shelter and for the public catalog.
--
-- These two tables are the deliberate exception to this schema's shape. They
-- carry NO shelter_id and are NOT in the tenant set, because a breed does not
-- belong to a shelter. What they still carry is RLS -- ENABLE and FORCE, like
-- everything else -- because a policy on a table whose row security is off is
-- inert, and "global" must mean "readable by both roles", never "unprotected".
--
-- The protection here is read-only rather than isolation, in two layers that
-- fail independently: a FOR SELECT policy and no other, and a SELECT grant and
-- no other. Either alone refuses a write; both are stated so that losing one is
-- not enough to open the table.
--
-- Identifiers are literal, not generated. D3 says application identifiers are
-- UUID v7 made in Go before the insert; a seed is not an application identifier.
-- Fixing them here means `pets.species_id` resolves to the same row in every
-- environment, which is what makes a fixture, a test and a client-side mapping
-- portable.

-- +goose Up

-- +goose StatementBegin

CREATE TABLE species (
    id   uuid NOT NULL PRIMARY KEY,

    -- The stable machine handle, in English because it is an identifier that
    -- code branches on -- `pets` filtering, the catalog's facets, the i18n key.
    code text NOT NULL UNIQUE CHECK (code = lower(code)),

    -- Display text, in es-MX because that is v1's locale (§11.1). When a second
    -- locale arrives, `code` is the join key a translation table hangs off, and
    -- this column becomes the fallback rather than the source.
    name text NOT NULL
);

CREATE TABLE breeds (
    id         uuid NOT NULL PRIMARY KEY,
    species_id uuid NOT NULL REFERENCES species (id),
    name       text NOT NULL,

    -- Two shelters must not be able to disagree about whether "Pastor Aleman"
    -- and "Pastor Alemán" are one breed, and the seed's replay guard below
    -- needs a key to conflict on.
    UNIQUE (species_id, name)
);

-- Every pet-catalog query filters by species first, so the child lookup is worth
-- an index even at this size -- and the size is the point: it will not stay at
-- thirty rows once shelters ask for the breeds they actually take in.
CREATE INDEX breeds_species_idx ON breeds (species_id);

ALTER TABLE species ENABLE ROW LEVEL SECURITY;
ALTER TABLE species FORCE  ROW LEVEL SECURITY;

ALTER TABLE breeds  ENABLE ROW LEVEL SECURITY;
ALTER TABLE breeds  FORCE  ROW LEVEL SECURITY;

-- USING (true) here is not the tautology it would be on a tenant table. It says
-- the deliberate thing: every row, to both application roles, for SELECT and for
-- nothing else. There is no INSERT, UPDATE or DELETE policy, so those commands
-- match no policy and are denied even before the missing grant is consulted.
CREATE POLICY reference_readable ON species FOR SELECT TO app_tenant, app_public
    USING (true);

CREATE POLICY reference_readable ON breeds  FOR SELECT TO app_tenant, app_public
    USING (true);

GRANT SELECT ON species TO app_tenant;
GRANT SELECT ON species TO app_public;
GRANT SELECT ON breeds  TO app_tenant;
GRANT SELECT ON breeds  TO app_public;

-- ---------------------------------------------------------------------------
-- Seeds.
--
-- ON CONFLICT DO NOTHING on the natural key, not on the primary key: replaying
-- this block against a database that already holds the rows must be a no-op, and
-- the identifiers are fixed so it would conflict either way. The natural key is
-- the honest one -- it is what makes a SECOND seed written later, with different
-- ids for the same breed, fail to duplicate it.
--
-- The markers below are read by TestReferenceData_SeedsAreIdempotent, which
-- re-executes exactly these statements against the live schema. Do not remove
-- them, and keep every seed statement between them.
-- ---------------------------------------------------------------------------

-- seeds:begin
INSERT INTO species (id, code, name) VALUES
    ('0192f000-0000-7000-8000-000000000001', 'dog',   'Perro'),
    ('0192f000-0000-7000-8000-000000000002', 'cat',   'Gato'),
    ('0192f000-0000-7000-8000-000000000003', 'other', 'Otro')
ON CONFLICT (code) DO NOTHING;

-- "Mestizo" is first on purpose and is not filler: most animals a shelter takes
-- in have no known breed, and `pets.breed_note` exists for the rest of that
-- story. A catalog whose breed filter cannot express "mixed" is a filter that
-- hides the majority of its own rows.
INSERT INTO breeds (id, species_id, name) VALUES
    ('0192f001-0000-7000-8000-000000000001', '0192f000-0000-7000-8000-000000000001', 'Mestizo'),
    ('0192f001-0000-7000-8000-000000000002', '0192f000-0000-7000-8000-000000000001', 'Labrador Retriever'),
    ('0192f001-0000-7000-8000-000000000003', '0192f000-0000-7000-8000-000000000001', 'Golden Retriever'),
    ('0192f001-0000-7000-8000-000000000004', '0192f000-0000-7000-8000-000000000001', 'Pastor Alemán'),
    ('0192f001-0000-7000-8000-000000000005', '0192f000-0000-7000-8000-000000000001', 'Pastor Belga Malinois'),
    ('0192f001-0000-7000-8000-000000000006', '0192f000-0000-7000-8000-000000000001', 'Chihuahua'),
    ('0192f001-0000-7000-8000-000000000007', '0192f000-0000-7000-8000-000000000001', 'Xoloitzcuintle'),
    ('0192f001-0000-7000-8000-000000000008', '0192f000-0000-7000-8000-000000000001', 'Poodle'),
    ('0192f001-0000-7000-8000-000000000009', '0192f000-0000-7000-8000-000000000001', 'Bulldog Francés'),
    ('0192f001-0000-7000-8000-00000000000a', '0192f000-0000-7000-8000-000000000001', 'Bulldog Inglés'),
    ('0192f001-0000-7000-8000-00000000000b', '0192f000-0000-7000-8000-000000000001', 'Pug'),
    ('0192f001-0000-7000-8000-00000000000c', '0192f000-0000-7000-8000-000000000001', 'Beagle'),
    ('0192f001-0000-7000-8000-00000000000d', '0192f000-0000-7000-8000-000000000001', 'Boxer'),
    ('0192f001-0000-7000-8000-00000000000e', '0192f000-0000-7000-8000-000000000001', 'Rottweiler'),
    ('0192f001-0000-7000-8000-00000000000f', '0192f000-0000-7000-8000-000000000001', 'Dóberman'),
    ('0192f001-0000-7000-8000-000000000010', '0192f000-0000-7000-8000-000000000001', 'Schnauzer'),
    ('0192f001-0000-7000-8000-000000000011', '0192f000-0000-7000-8000-000000000001', 'Cocker Spaniel'),
    ('0192f001-0000-7000-8000-000000000012', '0192f000-0000-7000-8000-000000000001', 'Dálmata'),
    ('0192f001-0000-7000-8000-000000000013', '0192f000-0000-7000-8000-000000000001', 'Husky Siberiano'),
    ('0192f001-0000-7000-8000-000000000014', '0192f000-0000-7000-8000-000000000001', 'Border Collie'),
    ('0192f001-0000-7000-8000-000000000015', '0192f000-0000-7000-8000-000000000001', 'Pitbull Terrier'),
    ('0192f001-0000-7000-8000-000000000016', '0192f000-0000-7000-8000-000000000001', 'Salchicha'),

    ('0192f002-0000-7000-8000-000000000001', '0192f000-0000-7000-8000-000000000002', 'Mestizo'),
    ('0192f002-0000-7000-8000-000000000002', '0192f000-0000-7000-8000-000000000002', 'Siamés'),
    ('0192f002-0000-7000-8000-000000000003', '0192f000-0000-7000-8000-000000000002', 'Persa'),
    ('0192f002-0000-7000-8000-000000000004', '0192f000-0000-7000-8000-000000000002', 'Maine Coon'),
    ('0192f002-0000-7000-8000-000000000005', '0192f000-0000-7000-8000-000000000002', 'Angora'),
    ('0192f002-0000-7000-8000-000000000006', '0192f000-0000-7000-8000-000000000002', 'Bengalí'),
    ('0192f002-0000-7000-8000-000000000007', '0192f000-0000-7000-8000-000000000002', 'Esfinge'),
    ('0192f002-0000-7000-8000-000000000008', '0192f000-0000-7000-8000-000000000002', 'Británico de Pelo Corto'),

    ('0192f003-0000-7000-8000-000000000001', '0192f000-0000-7000-8000-000000000003', 'Sin especificar')
ON CONFLICT (species_id, name) DO NOTHING;
-- seeds:end

-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin

-- breeds first: it references species. Policies and grants go with the tables.
DROP TABLE breeds;
DROP TABLE species;

-- +goose StatementEnd
