---
type: convention
score: 3
topic_key: mascotapp/convention/every-rls-migration-extends-two-pinned-inventories
task: T-02-016
status: guardado
observation_id: obs-a46dedb3f04d9188
rationale: "TestTenancyPolicies_ApplyToTheRightRoleAndCommand's own doc comment already states this rule for the policy table and the privilege table it carries, but the doc comment is easy to miss because the catalog meta-test (TestCatalog_EveryRelationIsClassifiedAndProtected) passes without it -- the catalog counts policies, it does not see who they are for. A migration author following only the catalog's green signal ships a table that silently escapes the one test that would catch a policy pointed at the wrong role."
---

# A green catalog does not mean the pinned policy/privilege inventory is current

`rlstest/catalog.go`'s meta-test (`TestCatalog_EveryRelationIsClassifiedAndProtected`) only
asserts that a table has RLS, is FORCEd, and carries **at least one** policy — it cannot see
which role a policy is for, or which columns a grant actually covers.

A second, hand-written test closes that gap: `TestTenancyPolicies_ApplyToTheRightRoleAndCommand`
(`isolation_test.go`) pins the exact policy inventory (table, policy name, command, roles) and
the exact privilege inventory (`has_table_privilege` per role/table/privilege) for the whole
schema, ordered by `COLLATE "C"`. It already carries its own instruction — **"every migration
from here adds its rows to both tables below"** — first paid by `refresh_tokens` in `00013`
(P2-D2), then again by `totp_recovery_codes` in `00014` (P2-D8).

Both times the catalog meta-test was green the whole time; only this second, hardcoded test
went red, and only because it enumerates by hand rather than deriving from the schema. Any
future migration that adds a table with RLS needs a row in **both** lists, or the new table's
access shape is simply never checked against who it is actually granted to.
