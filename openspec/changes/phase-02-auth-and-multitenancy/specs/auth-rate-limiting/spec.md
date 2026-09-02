# Auth Rate Limiting

## Purpose

In-memory, per-instance limiting on login and magic-link, over the existing
`ClientIPFromContext`. Stated as a partial mitigation, not a distributed one
(decision 3) — Phase 11 owns a limiter that survives a cold start.

## Requirements

### Requirement: Login and magic-link are rate-limited per client IP, in-memory and per instance

The system MUST enforce a maximum request count per client IP, resolved via
`ClientIPFromContext`, within a bounded time window, on the login and
magic-link endpoints. The limiter's state MUST be held in process memory
only: it MUST NOT coordinate across instances and MUST reset when the
process restarts. This stops clumsy, single-instance brute force; it does
not stop a distributed or patient attacker, and MUST NOT be documented or
implemented as if it did.

#### Scenario: Requests within the limit succeed

- GIVEN a client IP with fresh limiter state
- WHEN it makes N requests to login within the window, N below the threshold
- THEN every request reaches the normal auth handling, unaffected by the limiter

#### Scenario: A request past the limit is refused

- GIVEN the same client IP has just reached the threshold within the window
- WHEN one more request arrives before the window resets
- THEN the request is refused by the limiter, with a response distinct from a normal auth failure

#### Scenario: Limiter state does not survive a process restart

- GIVEN a client IP was refused for exceeding the threshold
- WHEN the process restarts and that IP makes a request immediately after
- THEN the request is processed normally, because the in-memory counter reset

### Requirement: Repeated failed logins against one account are throttled independent of client IP

The system MUST also track failed login attempts per account (by email),
independent of the per-IP limiter, so an attacker rotating source IPs
against a single account is still slowed.

#### Scenario: A different IP is still throttled once the account-level threshold is reached

- GIVEN account X has just reached its per-account failed-attempt threshold within the window, attempted from several distinct IPs
- WHEN another login attempt for X arrives from an IP that has never been used against X before
- THEN the request is refused by the account-level limiter, even though that IP's own per-IP limiter is fresh
