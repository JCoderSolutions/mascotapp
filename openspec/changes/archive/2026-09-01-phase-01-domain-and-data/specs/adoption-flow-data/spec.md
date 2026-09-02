# Delta for adoption-flow-data

## ADDED Requirements

### Requirement: Adoption applications and their children are tenant-scoped

`adoption_applications`, `application_notes` and `documents` SHALL each carry
`shelter_id` and satisfy the standard isolation rule. `adoption_applications` SHALL
expose `UNIQUE (id, shelter_id)` so its children can reference it compositely, and
SHALL reference `pets` by `(pet_id, shelter_id)`.

#### Scenario: Each table satisfies the isolation rule

- GIVEN each of `adoption_applications`, `application_notes` and `documents`
- WHEN the A/B isolation case is run against it
- THEN it passes as specified in the tenant-isolation capability

#### Scenario: An application cannot reference another tenant's pet

- GIVEN pet P owned by shelter A
- WHEN under tenant scope B an `adoption_applications` row referencing P is inserted with B's `shelter_id`
- THEN the statement fails with SQLSTATE `23503`

#### Scenario: A note cannot be attached to another tenant's application

- GIVEN application X owned by shelter A
- WHEN under tenant scope B an `application_notes` row referencing X is inserted with B's `shelter_id`
- THEN the statement fails with SQLSTATE `23503`

#### Scenario: Application listing is indexed

- GIVEN the migration set has been applied
- WHEN indexes on `adoption_applications` are read from the catalog
- THEN an index on `(shelter_id, status, created_at DESC)` exists

### Requirement: Application status is a closed set with a recorded transition time

`adoption_applications.status` SHALL be constrained to the declared set — `draft`,
`submitted`, `in_review`, `interview_scheduled`, `home_visit_scheduled`,
`approved`, `rejected`, `withdrawn`, `contract_signed`, `delivered`, `returned` —
and `status_changed_at` SHALL accompany it. Which transitions are legal is decided
in the domain layer, not by the database, and is out of scope for this phase.

#### Scenario: An unknown status is rejected

- GIVEN a valid application row
- WHEN `status` is set to a value outside the declared set
- THEN the statement fails with a check or enum violation

### Requirement: Note visibility is explicit

`application_notes.visibility` SHALL be constrained to `internal` or `shared`. This
phase stores the value; enforcing who may read which visibility belongs to a later
phase.

#### Scenario: An unknown visibility is rejected

- GIVEN a valid note row
- WHEN `visibility` is set to a value outside the declared set
- THEN the statement fails

### Requirement: Documents reference a media object and may stand alone

`documents` SHALL carry `shelter_id`, a required `media_id`, a nullable
`application_id`, and a `type` constrained to `adoption_contract`, `receipt`,
`health_certificate` or `custom`.

#### Scenario: A document without an application is valid

- GIVEN tenant scope A
- WHEN a `documents` row is inserted with `application_id` null
- THEN it succeeds

#### Scenario: A document cannot point at another tenant's media

- GIVEN media M owned by shelter A
- WHEN under tenant scope B a `documents` row referencing M is inserted with B's `shelter_id`
- THEN the statement fails with SQLSTATE `23503`
