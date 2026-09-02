# Delta for Tenant Isolation

## MODIFIED Requirements

### Requirement: Tenant table set

The schema SHALL define exactly one canonical **tenant table set**. A table is in
the set when each of its rows belongs to exactly one shelter.

| Set | Tables |
|---|---|
| Tenant tables (15) | `shelters` (scoped by `id`), `memberships`, `media`, `pets`, `pet_media`, `pet_health_records`, `pet_status_history`, `form_templates`, `form_template_versions`, `form_submissions`, `adoption_applications`, `application_events`, `application_notes`, `documents`, `audit_log` |
| Tenant child tables (9) | `pet_media`, `pet_health_records`, `pet_status_history`, `form_template_versions`, `form_submissions`, `adoption_applications`, `application_events`, `application_notes`, `documents` |
| Non-tenant model tables (5) | `users`, `refresh_tokens`, `totp_recovery_codes`, `species`, `breeds` |
| Infrastructure exemption (1) | `goose_db_version` |

All 20 model tables — tenant and non-tenant alike — MUST have `relrowsecurity` AND
`relforcerowsecurity` true, and MUST have at least one policy. There is no longer a
declared no-policy exception: `refresh_tokens` gains a policy in this phase (see
below), and the new `totp_recovery_codes` table gets one from creation. The three
sets MUST be exhaustive over schema `public`.

(Previously: 19 model tables, 4 non-tenant model tables, and `refresh_tokens` was
the one declared no-policy exception. This delta adds `totp_recovery_codes` to the
non-tenant set and removes the no-policy exception now that `refresh_tokens` is
policy-protected.)

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
- AND the policy count is at least 1 for every table, with no exception
- AND a table missing any of these fails the test

### Requirement: The connecting role cannot bypass RLS

The role the application and the test suite connect as MUST have `rolsuper = false`
AND `rolbypassrls = false` in `pg_roles`, and MUST NOT be the object-owner role.
Roles SHALL be created by migration in SQL only; a role provisioned outside the
migration may carry `BYPASSRLS` and would make every policy a no-op in production
while the suite stayed green. This now includes the new `app_auth` role.

(Previously: the guard checked `app_tenant` and `app_public` only. `app_auth`
joins the same guard, unchanged in mechanism.)

#### Scenario: Role guard runs before any isolation case

- GIVEN the integration harness has connected as `app_tenant`
- WHEN `SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user` is executed
- THEN both columns are false
- AND `current_user` is not the owner role
- AND the same assertion passes for `app_public` and `app_auth`

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

#### Scenario: refresh_tokens is reachable only through the auth role

- GIVEN `refresh_tokens` has RLS enabled with a policy and a grant to `app_auth` only
- WHEN `app_auth` performs the rotation lookup by `token_hash` and the follow-up write
- THEN both the read and the write succeed
- AND the same read and write attempted as `app_tenant`, even under a valid tenant scope, fail with SQLSTATE `42501`
- AND the same read and write attempted as `app_public` fail with SQLSTATE `42501`

(Previously: "refresh_tokens is default-deny in this phase" — refused to every
role, with no grant and no policy at all. This scenario replaces that one: the
table is now reachable through `app_auth`, and still refused to every other role.)

## ADDED Requirements

### Requirement: Table-level grants on shelters and memberships exclude privilege-sensitive columns

The `app_tenant` grant on `shelters` MUST NOT include UPDATE privilege on
`status`, `verified_at`, `verified_by`, `storage_quota_bytes`, or
`storage_bytes_used`. The `app_tenant` grant on `memberships`
MUST NOT include UPDATE privilege on `role`. This is a database-privilege
boundary, independent of and prior to any row-level policy or domain check
(see the authorization-rbac capability for the corresponding behavioral
scenario over SQL execution).

RLS: this narrows the GRANT, not the row-level policy; the existing policies
on `shelters` and `memberships` continue to scope rows to the tenant unchanged.

#### Scenario: Protected columns are absent from app_tenant's UPDATE privileges

- GIVEN the migrations have been applied
- WHEN `information_schema.column_privileges` is read for `app_tenant` on `shelters` and `memberships`
- THEN no UPDATE privilege row exists for `shelters.status`, `shelters.verified_at`, `shelters.verified_by`, `shelters.storage_quota_bytes`, `shelters.storage_bytes_used`, or `memberships.role`
- AND an UPDATE privilege row exists for at least one other column on each table
