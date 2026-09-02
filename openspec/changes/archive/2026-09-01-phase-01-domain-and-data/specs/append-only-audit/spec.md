# Delta for append-only-audit

## ADDED Requirements

### Requirement: `application_events` and `audit_log` are append-only

Rows in `application_events` and `audit_log` MAY be inserted and read, and MUST NOT
be updated, deleted, or truncated by any application role. This is enforced by
three independent layers, each covering a different actor:

| Layer | Stops |
|---|---|
| Revoked `UPDATE`/`DELETE`/`TRUNCATE` grants | the ordinary tenant role |
| Absence of any `UPDATE`/`DELETE` policy, under `FORCE ROW LEVEL SECURITY` | a restored grant, and the table owner |
| `BEFORE UPDATE OR DELETE` row trigger and `BEFORE TRUNCATE` statement trigger | a role carrying `BYPASSRLS`, which row security does not stop |

The trigger layer is also what turns a silent zero-row result — which calling code
readily misreads as success — into a loud error.

#### Scenario: Update is refused

- GIVEN an `application_events` row exists under tenant scope A
- WHEN it is updated as `app_tenant` under tenant scope A
- THEN the statement fails rather than reporting zero rows affected
- AND the row is unchanged

#### Scenario: Delete is refused

- GIVEN an `audit_log` row exists under tenant scope A
- WHEN it is deleted as `app_tenant` under tenant scope A
- THEN the statement fails and the row remains

#### Scenario: Truncate is refused

- GIVEN either append-only table holds rows
- WHEN `TRUNCATE` is attempted as `app_tenant`
- THEN the statement fails and the row count is unchanged

#### Scenario: No mutating policy exists

- GIVEN the migration set has been applied
- WHEN `pg_policies` is read for both tables
- THEN a `SELECT` policy and an `INSERT` policy exist
- AND no policy covers `UPDATE`, `DELETE`, or `ALL`

#### Scenario: The trigger layer is present

- GIVEN the migration set has been applied
- WHEN triggers on both tables are read from the catalog
- THEN a `BEFORE UPDATE OR DELETE` row trigger and a `BEFORE TRUNCATE` statement trigger exist on each

#### Scenario: Append still works

- GIVEN tenant scope A
- WHEN a row is inserted into each append-only table
- THEN it succeeds and is readable under tenant scope A

### Requirement: Append-only tables are still tenant-isolated

Both tables SHALL carry `shelter_id` and satisfy the standard isolation rule.
Because the `FOR ALL` policy is split into per-command policies here, each policy
SHALL state its `USING` or `WITH CHECK` clause explicitly rather than relying on
inference.

#### Scenario: Cross-tenant read and insert are refused

- GIVEN an event row owned by shelter A
- WHEN it is read under tenant scope B
- THEN 0 rows are returned
- AND an insert carrying A's `shelter_id` under tenant scope B fails with SQLSTATE `42501`

#### Scenario: An event cannot be attached to another tenant's application

- GIVEN application X owned by shelter A
- WHEN under tenant scope B an `application_events` row referencing X is inserted with B's `shelter_id`
- THEN the statement fails with SQLSTATE `23503`

### Requirement: The audit log is queryable by shelter and time

`audit_log` SHALL use a monotonically increasing `BIGSERIAL` identifier, carry
`occurred_at`, and be indexed on `(shelter_id, occurred_at DESC)`. The sequence
backing the identifier SHALL be granted to the tenant role, otherwise inserts fail.

#### Scenario: The index exists

- GIVEN the migration set has been applied
- WHEN indexes on `audit_log` are read from the catalog
- THEN an index on `(shelter_id, occurred_at DESC)` exists

#### Scenario: The tenant role can advance the sequence

- GIVEN tenant scope A
- WHEN a row is inserted into `audit_log` without supplying an identifier
- THEN it succeeds and receives an identifier greater than any previously issued
