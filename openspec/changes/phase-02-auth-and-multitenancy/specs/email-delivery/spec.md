# Email Delivery

## Purpose

An email-sending port plus a logging stub adapter, so the auth flow (magic
link, TOTP-related notices) stays end-to-end testable with no external
account, key, or network call (decision 2). Wiring a real provider is
unassigned.

## Requirements

### Requirement: Email is sent through a port with no dependency on a concrete provider

The application/domain layer MUST depend only on an email-sending port
(interface), never on a concrete provider client. The adapter configured in
this phase MUST be a logging stub that records the intended send (recipient
and purpose) and MUST NOT make any outbound network call.

#### Scenario: A magic-link email is recorded by the stub, not sent over the network

- GIVEN the logging stub adapter is configured
- WHEN the application sends a magic-link email through the port
- THEN the stub records the send with the correct recipient
- AND no outbound HTTP call is attempted

#### Scenario: The port accepts a second adapter without changing the caller

- GIVEN a second adapter implementing the same port (e.g. a test double)
- WHEN it is substituted for the logging stub
- THEN the code that calls the port requires no changes to compile or run
