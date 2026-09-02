# Pet Catalog Data

### Requirement: Reference data is global and read-only to tenants

`species` and `breeds` SHALL be global reference data with no `shelter_id`. Tenant
sessions SHALL have `SELECT` only. The migration set SHALL seed both, and the seed
SHALL be idempotent under replay.

#### Scenario: Tenants can read but not write reference data

- GIVEN the migration set has been applied
- WHEN `app_tenant` selects from `species` under any tenant scope
- THEN rows are returned regardless of which shelter is scoped
- AND an `INSERT`, `UPDATE` or `DELETE` on `species` or `breeds` is refused

#### Scenario: Seeds do not duplicate on replay

- GIVEN the set has been applied, reversed and applied again
- WHEN `species` and `breeds` are counted
- THEN each holds exactly the seeded number of rows

### Requirement: Pets carry typed filter attributes, not JSON

`pets` SHALL store `species_id`, `size`, `energy_level`, `good_with_kids`,
`good_with_dogs`, `good_with_cats` and `status` as typed, constrained columns —
never inside a JSON document — because they are the axis of adopter search.

#### Scenario: An out-of-domain value is rejected

- GIVEN a valid pet row under tenant scope A
- WHEN `status` is set to a value outside the declared status set
- THEN the statement fails with a check or enum violation

#### Scenario: Filter indexes exist

- GIVEN the migration set has been applied
- WHEN indexes on `pets` are read from the catalog
- THEN an index on `(shelter_id, status, published_at DESC)` exists
- AND a partial index on `(species_id, size, energy_level)` restricted to available pets exists

### Requirement: Pet child rows cannot be attached across tenants

`pet_media`, `pet_health_records` and `pet_status_history` SHALL each carry
`shelter_id` and reference their parent by the pair `(pet_id, shelter_id)`, which
requires `pets` to expose `UNIQUE (id, shelter_id)`. A denormalised `shelter_id`
without that composite reference is insufficient: the referential check bypasses
row security, so a single-column reference would let a tenant attach a row to
another tenant's pet.

#### Scenario: Composite uniqueness exists on the parent

- GIVEN the migration set has been applied
- WHEN constraints on `pets` are read from the catalog
- THEN a unique constraint over `(id, shelter_id)` exists

#### Scenario: Cross-tenant attachment is refused

- GIVEN pet P owned by shelter A
- WHEN under tenant scope B a `pet_media`, `pet_health_records` or `pet_status_history` row is inserted referencing P with B's `shelter_id`
- THEN the statement fails with SQLSTATE `23503`

#### Scenario: Child rows follow the parent's isolation

- GIVEN each of the three pet child tables
- WHEN the A/B isolation case is run against it
- THEN it passes as specified in the tenant-isolation capability

### Requirement: The public catalog is read-only and shows only published pets

The public role SHALL see a pet only when it is `available`, has a non-null
`published_at`, and has a null `deleted_at`. Its access SHALL be `SELECT` only,
through a separate role, a separate pool and a read-only transaction, with no
tenant scope set. Media is public only through a pet that is itself public.

#### Scenario: Public sees only the published subset

- GIVEN shelter A has one available published pet and one draft pet, and shelter B has one available published pet
- WHEN `pets` is read as `app_public`
- THEN both published pets are returned across shelters
- AND the draft pet is not returned

#### Scenario: Unpublishing removes a pet from the public catalog

- GIVEN a pet visible to `app_public`
- WHEN its `deleted_at` is set, or its `published_at` is cleared, or its status leaves `available`
- THEN it is no longer returned to `app_public`

#### Scenario: Public cannot write

- GIVEN a session as `app_public`
- WHEN an `INSERT`, `UPDATE` or `DELETE` is attempted on any table
- THEN the statement is refused

#### Scenario: Public media is reachable only through a public pet

- GIVEN media M is attached only to a draft pet
- WHEN `media` is read as `app_public`
- THEN M is not returned
