# Delta for Data Model Core

## ADDED Requirements

### Requirement: TOTP recovery codes provide single-use account recovery, non-tenant scoped

The schema SHALL provide a `totp_recovery_codes` table referencing
`users(id)`, storing each code as a salted/hashed value — never plaintext —
with a nullable `used_at`. The table SHALL carry no `shelter_id`.

RLS: this table has no `shelter_id` and is not tenant-scoped. RLS MUST still
be enabled with an explicit policy restricting access to the auth-path role,
following the same non-tenant-model treatment as `refresh_tokens` (see the
tenant-isolation delta for the exact catalog and grant assertions).

#### Scenario: Recovery codes are stored hashed, not plaintext

- GIVEN a user enrolls TOTP and receives recovery codes
- WHEN the stored rows are read directly from the table
- THEN no column holds the plaintext code
- AND a submitted code can still be verified by comparing its hash

#### Scenario: A used recovery code is marked and a second redemption is rejected

- GIVEN a recovery code row with `used_at` null
- WHEN the code is redeemed during login
- THEN `used_at` is set to the redemption time
- AND a second redemption attempt of the same code is rejected
