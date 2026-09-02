package rlstest_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gentleman/mascotapp/apps/api/internal/db"
	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
)

// appendAudit writes one audit row as the tenant, WITHOUT supplying an
// identifier, and returns the one the sequence issued.
func appendAudit(t *testing.T, env *dbtest.Env, shelter uuid.UUID, action string) int64 {
	t.Helper()

	var id int64
	err := db.WithTenant(context.Background(), env.TenantPool, shelter,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				`INSERT INTO audit_log (shelter_id, action, entity_type, entity_id)
				 VALUES ($1, $2, 'pets', $3)
				 RETURNING id`,
				shelter, action, uuid.New()).Scan(&id)
		})
	if err != nil {
		t.Fatalf("appending an audit row: %v", err)
	}

	return id
}

// The DoD in one sentence: *"forgetting the sequence grant is caught by a test,
// not by production"*.
//
// `BIGSERIAL` is not a type — it is a `bigint` plus a sequence plus a default
// that calls `nextval` on it. A table grant says nothing about that sequence, so
// `GRANT INSERT ON audit_log` alone produces a table the tenant can insert into
// in every test that supplies an id, and cannot insert into at all the moment
// real code omits one. `app_tenant` needs `USAGE` on the sequence, and this is
// the only grant in this schema that is not about a table.
//
// The failure mode is what makes it worth its own case: it is invisible to every
// test that writes explicit ids, and it appears on the first production write.
func TestAuditLog_CanBeAppendedWithoutSupplyingAnIdentifier(t *testing.T) {
	env := dbtest.Postgres(t)

	shelter := freshTenant(t, env)

	first := appendAudit(t, env, shelter, "pet.created")
	second := appendAudit(t, env, shelter, "pet.published")

	// The spec asks for monotonicity, not merely for two successful inserts:
	// *"receives an identifier greater than any previously issued"*. An audit
	// log read in id order has to be read in the order things happened.
	if second <= first {
		t.Errorf("the second audit row got id %d after the first got %d. The log is read "+
			"in id order, so a non-increasing identifier reorders history", second, first)
	}
}

// §4.6, and the query it exists for: *"CREATE INDEX ON audit_log (shelter_id,
// occurred_at DESC)"*. Every audit read is one shelter's activity, newest first,
// and `shelter_id` leads because RLS puts that predicate on every query whether
// the caller wrote it or not.
func TestAuditLog_HasTheShelterTimeIndex(t *testing.T) {
	env := dbtest.Postgres(t)

	rows, err := env.OwnerPool.Query(context.Background(), `
		SELECT indexdef FROM pg_indexes
		WHERE schemaname = 'public' AND tablename = 'audit_log'`)
	if err != nil {
		t.Fatalf("reading audit_log's indexes: %v", err)
	}
	defer rows.Close()

	var defs []string
	for rows.Next() {
		var def string
		if err := rows.Scan(&def); err != nil {
			t.Fatalf("scanning an index: %v", err)
		}
		defs = append(defs, def)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading audit_log's indexes: %v", err)
	}

	if !anyIndexContains(defs, []string{"shelter_id", "occurred_at DESC"}) {
		t.Errorf("the audit index is missing. §4.6 — every audit read is one shelter's "+
			"activity, newest first\ngot:\n  %v", defs)
	}
}

// `audit_log` records what happened to ANY entity, so `entity_type` and
// `entity_id` are a polymorphic pair with no foreign key — a reference cannot
// name a different table per row.
//
// That is a real hole and it is the right trade, but it has to be SAID rather
// than discovered: an audit row can name an entity that never existed, or one
// belonging to another shelter. What contains it is that the row is a LOG, not a
// reference: nothing resolves `entity_id` to fetch the thing, and the row is
// still tenant-scoped by its own `shelter_id`, so tenant B writing A's pet id
// into its own log learns nothing it did not already type.
//
// The test is the guard against the alternative going in quietly later: a
// single-column `REFERENCES pets (id)` bolted on for one entity type would be
// the cross-tenant reference D5 exists to prevent, and would only work for pets.
func TestAuditLog_HasNoForeignKeyOnItsPolymorphicEntity(t *testing.T) {
	env := dbtest.Postgres(t)

	var keyed bool
	if err := env.OwnerPool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM pg_constraint c
			WHERE c.conrelid = 'public.audit_log'::regclass
			  AND c.contype = 'f'
			  AND 'entity_id' = ANY (
			      SELECT a.attname FROM unnest(c.conkey) AS k(attnum)
			      JOIN pg_attribute a
			        ON a.attrelid = c.conrelid AND a.attnum = k.attnum)
		)`).Scan(&keyed); err != nil {
		t.Fatalf("reading audit_log's foreign keys: %v", err)
	}

	if keyed {
		t.Error("`audit_log.entity_id` carries a foreign key. It is POLYMORPHIC — the row " +
			"says which table through `entity_type` — so a key can only name one of them, " +
			"which means it either refuses every audit row about anything else or it is a " +
			"single-column cross-tenant reference (D5). If a later phase wants referential " +
			"integrity here, it needs one child table per entity type, not a key bolted on " +
			"this one")
	}
}

// `USAGE` on the sequence, not `ALL`, and mutation is what showed the difference
// was unguarded: swapping one for the other broke no test.
//
// `ALL` on a sequence is `USAGE` plus `SELECT` plus **`UPDATE`**, and `UPDATE` on
// a sequence is `setval`. An inserter needs `nextval`; nothing in this product
// ever needs to move the counter by hand.
//
// What it costs is exactly the property the previous test asserts. A tenant can
// set the sequence high, append a row, set it back low, and append again — and
// now a LATER audit row carries a SMALLER id than an earlier one. The log is
// read in id order because that order is meant to be the order things happened,
// so a tenant that can move the counter can reorder its own history. It can also
// rewind into ids already taken and turn every subsequent append into a primary
// key collision, which is an audit trail that stops recording.
func TestAuditLog_TheTenantCannotMoveTheSequence(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	shelter := freshTenant(t, env)

	// Anti-vacuity: `nextval` DOES work for this role. Without it, a revoked
	// USAGE grant would pass the refusal below while making the table
	// unwritable — the exact failure M1 is about, passing as if it were M2.
	if err := db.WithTenant(ctx, env.TenantPool, shelter,
		func(ctx context.Context, tx pgx.Tx) error {
			var next int64

			return tx.QueryRow(ctx, `SELECT nextval('audit_log_id_seq')`).Scan(&next)
		}); err != nil {
		t.Fatalf("app_tenant cannot call nextval on the audit sequence, so the refusal "+
			"below is about USAGE being missing rather than about UPDATE being present: %v",
			err)
	}

	err := db.WithTenant(ctx, env.TenantPool, shelter,
		func(ctx context.Context, tx pgx.Tx) error {
			var moved int64

			return tx.QueryRow(ctx, `SELECT setval('audit_log_id_seq', 1)`).Scan(&moved)
		})
	if err == nil {
		t.Fatal("app_tenant moved the audit sequence with setval. It can now set the " +
			"counter high, append, set it back low and append again — so a LATER audit row " +
			"carries a SMALLER id than an earlier one, and the log read in id order is no " +
			"longer the order things happened. Rewinding into taken ids also turns every " +
			"later append into a primary key collision, which is an audit trail that " +
			"stops recording. The grant is USAGE, never ALL")
	}
	if code := sqlstateOf(t, err, "calling setval as app_tenant"); code !=
		sqlstateInsufficientPrivilege {
		t.Errorf("setval was refused with %s rather than insufficient privilege (%s), so "+
			"the grant is not what refused it", code, sqlstateInsufficientPrivilege)
	}
}
