# Tenant Isolation

### Requirement: Tenant table set

The schema SHALL define exactly one canonical **tenant table set**. A table is in
the set when each of its rows belongs to exactly one shelter.

| Set | Tables |
|---|---|
| Tenant tables (15) | `shelters` (scoped by `id`), `memberships`, `media`, `pets`, `pet_media`, `pet_health_records`, `pet_status_history`, `form_templates`, `form_template_versions`, `form_submissions`, `adoption_applications`, `application_events`, `application_notes`, `documents`, `audit_log` |
| Tenant child tables (9) | `pet_media`, `pet_health_records`, `pet_status_history`, `form_template_versions`, `form_submissions`, `adoption_applications`, `application_events`, `application_notes`, `documents` |
| Non-tenant model tables (4) | `users`, `refresh_tokens`, `species`, `breeds` |
| Infrastructure exemption (1) | `goose_db_version` |

All 19 model tables — tenant and non-tenant alike — MUST have `relrowsecurity` AND
`relforcerowsecurity` true. All of them MUST have at least one policy, with a single
declared exception: `refresh_tokens` is default-deny in this phase and carries none.
The three sets MUST be exhaustive over schema `public`.

Enabling row level security on the non-tenant tables is not ceremony: `users` is
protected only by a policy, and a policy on a table without RLS enabled is inert,
which would expose every user row to every tenant while the policy looked correct.

#### Scenario: Catalog meta-test rejects an unclassified table

- GIVEN the migrations have been applied to a fresh database
- WHEN every relation of kind `r` in schema `public` is read from `pg_class`
- THEN each one is in exactly one of the three sets
- AND a relation in none of them fails the test naming the offending table

#### Scenario: Catalog meta-test rejects an unprotected table

- GIVEN a table named in the tenant set or the non-tenant model set
- WHEN `relrowsecurity`, `relforcerowsecurity` and its `pg_policies` rows are read
- THEN both flags are true
- AND the policy count is at least 1 for every table except `refresh_tokens`
- AND a table missing any of these fails the test

### Requirement: Cross-tenant access is impossible for every tenant table

For EVERY table in the tenant table set, a session scoped to shelter B MUST NOT be
able to read, update, or delete a row owned by shelter A, and MUST NOT be able to
insert a row carrying A's `shelter_id`. This requirement is parameterised over the
tenant table set: adding a table to the set adds its case. This is the ADR-0002
completion rule — a table without a passing case is not done.

#### Scenario: A/B isolation, applied per tenant table

- GIVEN row R exists in table T owned by shelter A, inserted under tenant scope A as `app_tenant`
- WHEN the same role runs `SELECT`, `UPDATE` and `DELETE` against R under tenant scope B
- THEN `SELECT` returns 0 rows, `UPDATE` affects 0 rows, and `DELETE` affects 0 rows
- AND an `INSERT` into T carrying A's `shelter_id` fails with SQLSTATE `42501`
- AND R is still present and byte-identical when re-read under tenant scope A

### Requirement: Cross-tenant child rows are rejected by the database

For EVERY tenant child table, an insert referencing a parent row owned by another
shelter MUST fail with a foreign key violation, SQLSTATE `23503`. PostgreSQL
referential integrity checks always bypass row security, so a single-column foreign
key would let such an insert succeed; the constraint — not the policy — is what
closes the orphan path.

#### Scenario: Child insert referencing another tenant's parent

- GIVEN parent row P in table `pets` owned by shelter A
- WHEN under tenant scope B a `pet_media` row is inserted with `pet_id = P.id` and B's own `shelter_id`
- THEN the statement fails with SQLSTATE `23503`
- AND the same failure occurs for a `pet_id` that exists in no shelter at all

#### Scenario: Every child table is covered

- GIVEN the tenant child table set
- WHEN the orphan case is run for each of the nine tables against its declared parent
- THEN every case fails with `23503`

### Requirement: Tenant scope is transaction-local

Tenant scope SHALL be established only inside a transaction, by
`SELECT set_config('app.shelter_id', $1, true)`. `SET LOCAL` and any session-level
or pool-level binding are prohibited. A query issued outside a tenant transaction
MUST see zero rows, never another shelter's rows.

#### Scenario: Scope does not survive the transaction

- GIVEN a tenant transaction for shelter A has committed on a pooled connection
- WHEN `current_setting('app.shelter_id', true)` is read on that same connection outside any transaction
- THEN the result is NULL or empty
- AND the same holds after a transaction that rolled back

#### Scenario: Unscoped query fails closed

- GIVEN rows exist for shelters A and B in a tenant table
- WHEN that table is queried as `app_tenant` with no tenant scope set
- THEN 0 rows are returned rather than any shelter's rows

#### Scenario: Tenant wrapper rejects an absent tenant

- GIVEN a caller invokes the tenant wrapper with the nil UUID
- WHEN the call is made
- THEN it returns a no-tenant error and no transaction is opened

#### Scenario: Tenant wrapper never commits after a failure

- GIVEN a caller's function returns an error, or panics, inside a tenant transaction
- WHEN the wrapper unwinds
- THEN the transaction is rolled back, no work is committed, and a panic is re-raised after rollback

### Requirement: The connecting role cannot bypass RLS

The role the application and the test suite connect as MUST have `rolsuper = false`
AND `rolbypassrls = false` in `pg_roles`, and MUST NOT be the object-owner role.
Roles SHALL be created by migration in SQL only; a role provisioned outside the
migration may carry `BYPASSRLS` and would make every policy a no-op in production
while the suite stayed green.

#### Scenario: Role guard runs before any isolation case

- GIVEN the integration harness has connected as `app_tenant`
- WHEN `SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user` is executed
- THEN both columns are false
- AND `current_user` is not the owner role
- AND the same assertion passes for `app_public`

#### Scenario: A bypassing role fails the guard

- GIVEN the harness is pointed at a role carrying `BYPASSRLS` or `SUPERUSER`
- WHEN the guard runs
- THEN the suite fails at setup and no isolation case is reported as passing

### Requirement: Grants are explicit and default-deny

Privileges SHALL be granted per table. `GRANT ... ON ALL TABLES` and
`ALTER DEFAULT PRIVILEGES` MUST NOT be used, so a new table is unreachable by
`app_tenant` until a migration grants it deliberately.

#### Scenario: A table with no explicit grant is unreachable

- GIVEN a table exists with RLS enabled and a policy but no grant to `app_tenant`
- WHEN `app_tenant` selects from it under a valid tenant scope
- THEN the statement fails with SQLSTATE `42501`

#### Scenario: refresh_tokens is default-deny in this phase

- GIVEN `refresh_tokens` has RLS enabled with no policy and no grant to `app_tenant`
- WHEN `app_tenant` reads or writes it
- THEN the statement is refused
