# Authorization and RBAC

## Purpose

The path from a verified token claim to `WithTenant`, the domain permission
matrix by `membership.role`, and the column-level grants that give B1 a
database backstop, not just a domain check.

## Requirements

### Requirement: Tenant scope derives only from the verified token claim

`WithTenant` MUST be invoked with `shelter_id` sourced exclusively from the
verified access token's claim. A `shelter_id` present in a URL path, query,
or header MUST NOT itself set tenant scope; if a request also names a
`shelter_id` in the path and it does not match the claim, the request MUST be
refused before the handler runs.

RLS: this is the sole legitimate input to `app.shelter_id` (ADR-0002/0008);
`WithTenant` continues to refuse `uuid.Nil`.

#### Scenario: A path shelter_id matching the claim proceeds

- GIVEN a valid token with claim `shelter_id = A`
- WHEN a request to a path scoped to shelter A is made
- THEN the request proceeds, scoped to A via `WithTenant`

#### Scenario: A forged shelter_id in the path is refused

- GIVEN the same valid token with claim `shelter_id = A`
- WHEN a request to a path scoped to shelter B is made
- THEN the request is refused with 403 before `WithTenant` or any handler logic runs

#### Scenario: A missing or invalid claim is refused before scope comparison

- GIVEN a request with no token, or a token with no valid `shelter_id` claim
- WHEN any tenant-scoped endpoint is called
- THEN the request is refused and `WithTenant` is never invoked

### Requirement: The domain permission matrix authorizes by role, deny by default

Every privileged action MUST be checked against a role × permission matrix
before it executes. A role/permission combination with no explicit grant in
the matrix MUST be denied, not allowed by omission.

#### Scenario: A permitted role completes the action

- GIVEN a role that the matrix maps to permission P
- WHEN a member with that role performs the action requiring P
- THEN the action is authorized and proceeds

#### Scenario: A role without the permission is denied

- GIVEN a role the matrix does NOT map to permission P
- WHEN a member with that role attempts the same action
- THEN the action is denied

#### Scenario: Every role × permission cell has an explicit outcome

- GIVEN the full set of declared roles and the full set of declared permissions
- WHEN every role × permission pair is enumerated against the matrix
- THEN each pair resolves to an explicit allow or deny, with no pair defaulting to allow by being unmapped

### Requirement: Column-level grants close the self-escalation gap (B1)

The system MUST deny UPDATE, at the database privilege level and independent
of any domain check, on `shelters.status`, `shelters.verified_at`,
`shelters.verified_by`, `shelters.storage_quota_bytes`,
`shelters.storage_bytes_used`, and `memberships.role`, even for a session
correctly scoped to its own shelter. A bypassed or absent domain check MUST
NOT be able to self-verify a shelter, self-promote a membership to `owner`,
raise its own quota, or rewrite the counter that quota is checked against.

`storage_bytes_used` is in the protected set for the same reason
`storage_quota_bytes` is: a tenant that can write the counter defeats the
quota check exactly as thoroughly as one that can write the limit.
`verified_at` / `verified_by` are the second and third columns of the
self-verification hole that `status` opens.

RLS: this is a GRANT-level (privilege) restriction, layered on top of the
unchanged row-level policy that already scopes `shelters` and `memberships`
to the tenant's own rows (see the tenant-isolation delta for the exact
catalog assertion).

#### Scenario: Direct SQL against a protected column fails independent of the domain layer

- GIVEN `app_tenant` is scoped to shelter A via a valid `WithTenant` transaction
- WHEN an `UPDATE` targets `shelters.status`, `shelters.verified_at`, `shelters.verified_by`, `shelters.storage_quota_bytes`, `shelters.storage_bytes_used`, or `memberships.role` on A's own rows
- THEN each statement fails with SQLSTATE `42501`
- AND no row's protected column value changes

#### Scenario: Non-protected columns on the same tables remain writable

- GIVEN the same tenant scope A
- WHEN an `UPDATE` targets a column outside the protected set (e.g. `shelters.name`)
- THEN it succeeds
