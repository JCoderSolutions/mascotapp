# Delta for dynamic-forms-data

## ADDED Requirements

### Requirement: A published form template version is immutable

Once a `form_template_versions` row has a non-null `published_at`, its `definition`
and `version` MUST NOT change, and the row MUST NOT be deleted. Editing a published
form SHALL create `version + 1`. Without this, historical submissions become
unreadable.

#### Scenario: Editing a published definition is refused

- GIVEN a template version with `published_at` set, under tenant scope A
- WHEN its `definition` is updated
- THEN the statement fails with an explicit error naming the immutability rule
- AND the stored `definition` is unchanged

#### Scenario: Deleting a published version is refused

- GIVEN a template version with `published_at` set
- WHEN it is deleted
- THEN the statement fails and the row remains

#### Scenario: A draft version is still editable

- GIVEN a template version with `published_at` null
- WHEN its `definition` is updated
- THEN the update succeeds

#### Scenario: Publishing creates a new version rather than mutating

- GIVEN template T has a published version 1
- WHEN a changed definition is published for T
- THEN a new row exists with `version = 2`
- AND version 1 is byte-identical to before
- AND `UNIQUE (shelter_id, template_id, version)` refuses a second row with `version = 2`

> **Amended at T-01-026 (verification pass).** This scenario originally wrote
> `UNIQUE (template_id, version)`, and T-01-023 deviated from it deliberately. The reason was
> verified against PostgreSQL 17 rather than assumed: **the unique index is checked BEFORE the
> foreign key** — referential checks run as AFTER triggers, the index insert happens on the heap
> write — so a row that is both a duplicate AND names a foreign parent reports `23505`, while one
> that only names a foreign parent reports `23503`. A version key spanning shelters is therefore
> a **cross-tenant existence oracle even with the composite foreign key in place**, because the
> key never gets to answer: tenant B names A's template and reads A's form history off the
> SQLSTATE. Adding `shelter_id` keeps the guarantee identical inside a tenant — a template
> belongs to exactly one shelter, so per-shelter uniqueness over its versions IS per-template
> uniqueness — and collapses the two answers into one for everybody else. The mutant that
> restores the literal key above **dies**, so this is evidence rather than preference.

### Requirement: Field identifiers are stable for the lifetime of a template

Removing a field in a later version MUST NOT invalidate or orphan answers recorded
against an earlier version.

#### Scenario: Answers survive a field removal

- GIVEN a submission against version 1 contains an answer keyed `has_other_pets`
- WHEN version 2 is published without that field
- THEN the submission still resolves that answer when rendered against version 1

> **Moved to Phase 07 at T-01-026 (verification pass).** This requirement originally opened with
> *"A `field.id` SHALL be unique within its template and SHALL NOT be reused or reassigned"* and
> carried a second scenario, *Duplicate field identifiers are rejected*, written as `WHEN it is
> validated`. Both are application-layer validation, not schema, and this phase delivers no
> validator: the shared closed union (`packages/form-schema/`, plan §9) and the form engine are
> Phase 07 work. A `SHALL` nobody enforces is worse than one written down where it will be, so
> the clause moved with its scenario.
>
> **The half this phase DOES deliver stays above**, because it is the half the database can
> enforce: a removed field cannot orphan an earlier answer, since the submission is bound to the
> version it was filled with and published versions are frozen. Note that it holds regardless of
> the moved clause — even a definition with duplicate ids resolves against its own version.
>
> A jsonb `CHECK` would have covered the moved scenario in the database, and it was rejected as
> the wrong layer **as the only layer**: a `CHECK`'s error cannot *"name the duplicated
> identifier"*, which is what the scenario asks for. The database is the last line of defence
> here, not the first.

### Requirement: Templates, versions and submissions are tenant-scoped

`form_templates`, `form_template_versions` and `form_submissions` SHALL each carry
`shelter_id`. `form_template_versions` SHALL reference its template by
`(template_id, shelter_id)`, and `form_submissions` SHALL reference its version by
`(template_version_id, shelter_id)`. `form_templates` SHALL enforce
`UNIQUE (shelter_id, key)`. `form_submissions.answers` SHALL be indexed with GIN.

> **Amended at T-01-026 (verification pass).** The submission's composite reference was missing
> from this requirement, which is precisely the hole T-01-025's mutation round found: reducing it
> to `REFERENCES form_template_versions (id)` broke no test. Referential integrity checks
> **always bypass row security**, so the single-column form resolves another shelter's version on
> this tenant's behalf, and nothing that READS can notice — the submission carries its own
> shelter's `shelter_id` while pointing at a form that shelter cannot see.

#### Scenario: The three tables satisfy the isolation rule

- GIVEN each of `form_templates`, `form_template_versions` and `form_submissions`
- WHEN the A/B isolation case is run against it
- THEN it passes as specified in the tenant-isolation capability

#### Scenario: A version cannot be attached to another tenant's template

- GIVEN template T owned by shelter A
- WHEN under tenant scope B a `form_template_versions` row referencing T is inserted with B's `shelter_id`
- THEN the statement fails with SQLSTATE `23503`

#### Scenario: A submission cannot be recorded against another tenant's version

- GIVEN published version V owned by shelter A
- WHEN under tenant scope B a `form_submissions` row referencing V is inserted with B's own `shelter_id`
- THEN the statement fails with SQLSTATE `23503`
- AND the SQLSTATE **and constraint name** are identical to those returned for a
  `template_version_id` that exists in no shelter at all, so the refusal is not an oracle

#### Scenario: Template keys are unique per shelter, not globally

- GIVEN shelter A has a template with key `adoption`
- WHEN shelter B creates a template with key `adoption`
- THEN it succeeds
- AND a second `adoption` template within shelter A fails with a unique violation

#### Scenario: Answers are searchable

- GIVEN the migration set has been applied
- WHEN indexes on `form_submissions` are read from the catalog
- THEN a GIN index on `answers` exists
