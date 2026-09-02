# Identity and Session

## Purpose

Registration, login, magic link, TOTP (with recovery codes), JWT issuance, and
refresh-token rotation — the producer of the `shelter_id` claim every tenant
policy depends on, plus the cross-origin session mechanics decision 4 requires.

## Requirements

### Requirement: Password-based registration and login use Argon2id

Passwords MUST be hashed with Argon2id before storage; the system MUST NOT
store a plaintext or reversibly-encrypted password. Login MUST verify the
submitted password against the stored Argon2id hash.

#### Scenario: Login succeeds with the correct password

- GIVEN a registered user with an Argon2id password hash
- WHEN they log in with the correct password
- THEN the credential check passes and issuance proceeds

#### Scenario: Login fails with an incorrect password

- GIVEN the same registered user
- WHEN they log in with any other password
- THEN the credential check fails and no session is issued

### Requirement: Shelter registration is open and lands pending_verification

Registering a new shelter MUST succeed without prior invitation or approval.
The created row MUST have `status = 'pending_verification'`. The registration
path MUST NOT accept a caller-supplied `status`.

#### Scenario: A newly registered shelter is pending_verification

- GIVEN no shelter exists yet for this registration
- WHEN a shelter is registered with valid input
- THEN the shelter is created
- AND its `status` is `pending_verification`, regardless of any `status` field submitted by the caller

### Requirement: Magic-link responses are indistinguishable by account existence

The response to a magic-link request MUST be identical — same HTTP status,
same body shape, and comparable timing — whether or not the address belongs
to an account. An email MUST be sent only when the account exists.

#### Scenario: Response is indistinguishable between an existing and a non-existing address

- GIVEN one address with an account and one with none
- WHEN a magic-link request is made for each
- THEN both responses share the same HTTP status and body shape
- AND both response times fall within the same bounded tolerance
- AND only the existing account's address receives an email

### Requirement: TOTP is mandatory for owner and admin roles

Login for a membership with role `owner` or `admin` MUST NOT issue a session
until a valid TOTP code (or a valid unused recovery code) is presented. Roles
outside `owner`/`admin` MUST NOT be required to present TOTP.

#### Scenario: Owner login without TOTP does not complete

- GIVEN an owner has TOTP enrolled
- WHEN they submit the correct password with no TOTP code
- THEN no session is issued

#### Scenario: Owner login with a valid TOTP code succeeds

- GIVEN the same owner
- WHEN they submit the correct password and a valid current TOTP code
- THEN a session is issued

#### Scenario: A non-privileged role logs in without TOTP

- GIVEN an adopter account with no membership role requiring TOTP
- WHEN they submit the correct password with no TOTP code
- THEN a session is issued

### Requirement: A recovery code can complete login in place of TOTP

Login for `owner`/`admin` MAY be completed with a valid, unused recovery code
instead of a TOTP code.

#### Scenario: Login completes with a valid recovery code instead of TOTP

- GIVEN an owner holds unused recovery codes
- WHEN they log in with the correct password and one valid recovery code
- THEN the login completes and a session is issued

#### Scenario: Login still fails with neither TOTP nor a recovery code

- GIVEN an admin has TOTP enrolled
- WHEN they log in with only the correct password
- THEN no session is issued

### Requirement: JWT access tokens are short-lived and carry the tenant claim

Access tokens MUST expire 15 minutes after issuance and MUST carry a
`shelter_id` claim. A token past its expiry MUST be rejected.

RLS: the `shelter_id` claim on a valid, unexpired token is the sole input to
`WithTenant` (see authorization-rbac); it is not itself an RLS mechanism.

#### Scenario: An expired access token is rejected

- GIVEN an access token issued 16 minutes ago
- WHEN it is presented to an authenticated endpoint
- THEN the request is refused before any handler logic runs

#### Scenario: An unexpired access token is accepted

- GIVEN an access token issued 1 minute ago
- WHEN it is presented to the same endpoint
- THEN the token verification succeeds

### Requirement: Refresh tokens rotate on use and reuse revokes the whole family

Each successful refresh MUST issue a new refresh token and invalidate the one
presented. Presenting a token that has already been rotated (i.e., reused)
MUST revoke every token sharing its `family_id`.

RLS: refresh-token rows are non-tenant and reachable only through the
auth-path role's grant and policy; `app_tenant` remains refused (see the
tenant-isolation delta).

#### Scenario: A valid, not-yet-rotated refresh token rotates successfully

- GIVEN a refresh token that has not been used since issuance
- WHEN it is presented to the refresh endpoint
- THEN a new refresh token is issued in the same `family_id`
- AND the presented token is marked rotated

#### Scenario: Reusing an already-rotated token revokes its entire family

- GIVEN a refresh token that was already rotated once
- WHEN the same (now-stale) token is presented again
- THEN the request is refused
- AND every token in that `family_id`, including the currently valid one, is revoked

#### Scenario: A token from a revoked family is refused after the reuse event

- GIVEN the family was revoked by the prior scenario
- WHEN the token that was valid immediately before the reuse event is presented
- THEN it is refused, proving the revocation was family-wide, not single-token

### Requirement: The refresh cookie and cross-origin requests are protected for separate origins

The refresh token MUST be delivered as an HTTP-only cookie with
`SameSite=None; Secure`. Cross-origin requests MUST be accepted only from an
explicit origin allowlist, and a response granting credentials MUST NOT use a
wildcard origin. State-changing requests MUST additionally require a valid
CSRF token, independent of `SameSite`.

#### Scenario: An allowlisted origin with valid credentials and a valid CSRF token succeeds

- GIVEN a request from an allowlisted origin, with the session cookie and a valid CSRF token
- WHEN a state-changing endpoint is called
- THEN the request succeeds

#### Scenario: A non-allowlisted origin is refused regardless of credentials

- GIVEN a request from an origin not on the allowlist, with a valid session cookie
- WHEN the same endpoint is called
- THEN CORS refuses the request before it reaches the handler

#### Scenario: A missing or invalid CSRF token is refused even from an allowlisted origin

- GIVEN a request from an allowlisted origin with a valid session cookie but no or an invalid CSRF token
- WHEN a state-changing endpoint is called
- THEN the request is refused
