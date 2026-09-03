package dbtest

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/gentleman/mascotapp/apps/api/internal/db"
)

// SQLSTATEs the runner asserts on, named because the numbers alone say nothing
// at a call site.
const (
	// sqlstateInsufficientPrivilege is what a WITH CHECK violation raises. It is
	// what tenant B gets for trying to write a row stamped with tenant A's id.
	sqlstateInsufficientPrivilege = "42501"
)

// RowKey identifies one row by the columns that make it unique.
//
// It is a map rather than a single id because not every tenant table has a
// single-column identity: §4.3 gives `pet_media` the columns
// `(pet_id, media_id, position, is_primary)` and no `id`, so its identity is a
// pair. A runner that assumed `WHERE id = $1` would need a special case for it,
// and special cases in a security harness are where coverage goes to die.
type RowKey map[string]any

// where renders the key as a predicate with positional parameters, in a stable
// column order so the generated SQL is deterministic.
func (k RowKey) where(startAt int) (string, []any) {
	cols := make([]string, 0, len(k))
	for col := range k {
		cols = append(cols, col)
	}
	sort.Strings(cols)

	predicates := make([]string, 0, len(cols))
	args := make([]any, 0, len(cols))
	for i, col := range cols {
		predicates = append(predicates, fmt.Sprintf("%s = $%d", col, startAt+i))
		args = append(args, k[col])
	}

	return strings.Join(predicates, " AND "), args
}

// TenantTable is one table's entry in the A/B isolation suite.
//
// Adding a table to the suite is one of these. That is deliberate: ADR-0002
// makes an A/B test the completion rule for every table carrying `shelter_id`,
// and a rule that is expensive to follow is a rule that gets skipped.
type TenantTable struct {
	// Name is the table, used in generated SQL and in failure messages.
	Name string

	// TenantColumn is the column the table's policy scopes on. It defaults to
	// `shelter_id`, which is every table in this schema except `shelters`,
	// whose rows ARE the tenants and which is therefore scoped by its own `id`
	// (design §The RLS policy template).
	//
	// The runner needs it for the no-op UPDATE below. A hardcoded `shelter_id`
	// would fail on `shelters` with `column does not exist` (42703) — an error
	// that looks like a broken test rather than the missing case it is.
	TenantColumn string

	// Insert creates exactly one row owned by shelterID and returns the key
	// that identifies it. It runs inside a transaction whose tenant scope is
	// already set, and is called both to seed tenant A's row and to attempt the
	// cross-tenant write that must be refused.
	Insert func(ctx context.Context, tx pgx.Tx, shelterID uuid.UUID) (RowKey, error)

	// Fixture runs as the OWNER before the case, for rows the case depends on
	// but does not assert about — a parent a foreign key points at, say.
	//
	// It is separate from Insert because the two run as different roles for
	// different reasons: Insert must run as app_tenant or it proves nothing,
	// while a fixture that had to run as app_tenant could only ever create rows
	// the policy already allows, which is not always what a foreign key needs.
	//
	// It takes the whole Env rather than just the owner pool because the rows a
	// fixture creates are usually the two tenants themselves.
	//
	// It must be idempotent: the container is shared across the package, and a
	// case may be re-run alone with `-run`.
	Fixture func(ctx context.Context, env *Env) error

	// AppendOnly marks a table that app_tenant may read and insert into but
	// never update, delete from or truncate — `pet_status_history` today,
	// `application_events` and `audit_log` at T-01-029 and T-01-032.
	//
	// It does not weaken the completion rule, it STRENGTHENS it. On an ordinary
	// table the write probes assert "the statement reached no rows"; on one of
	// these they assert "the statement was REFUSED", which is a strictly
	// stronger claim. Skipping the probes instead would have been the easy
	// route and the wrong one: an append-only table whose grants were quietly
	// restored would then pass by not being looked at.
	AppendOnly bool

	// TouchColumn is the column the no-op UPDATE probe self-assigns. It
	// defaults to the tenant column, which is what every case used until
	// `00015_column_grants`.
	//
	// Why it had to become configurable, found at T-02-006. The probe writes
	// `SET <col> = <col>` purely to see whether the POLICY lets the statement
	// reach another tenant's row. That only measures the policy while the role
	// actually HOLDS the privilege on that column: once `00015` narrowed
	// `shelters` and `memberships` to column grants, `shelters.id` and
	// `memberships.shelter_id` stopped being updatable and the probe came back
	// 42501 — a privilege refusal standing in for a policy that was never
	// consulted.
	//
	// Granting UPDATE on those columns to fix it would be absurd: an updatable
	// `shelters.id` re-tenants a shelter and an updatable
	// `memberships.shelter_id` moves a member into somebody else's. The probe is
	// what had to move, onto a column the tenant is genuinely allowed to write,
	// so the grant steps aside and the policy answers.
	//
	// That makes the probe STRICTLY STRONGER than before: it can no longer be
	// satisfied by a privilege refusal that proves nothing about isolation.
	TouchColumn string

	// NoDeleteGrant marks a table app_tenant may update but never delete from.
	//
	// It is the narrow half of AppendOnly, and it exists because `00015` created
	// a state the flag could not describe: `shelters` and `memberships` are
	// updatable through a column grant and have DELETE revoked outright. Marking
	// them AppendOnly would have been wrong in the direction that matters — it
	// would stop the UPDATE probe from ever exercising the policy on the two
	// core tenancy tables, which is precisely the isolation §10 calls blocking.
	NoDeleteGrant bool
}

// tenantColumn is TenantColumn with its default applied.
func (tt TenantTable) tenantColumn() string {
	if tt.TenantColumn == "" {
		return "shelter_id"
	}

	return tt.TenantColumn
}

// touchColumn is TouchColumn with its default applied.
func (tt TenantTable) touchColumn() string {
	if tt.TouchColumn == "" {
		return tt.tenantColumn()
	}

	return tt.TouchColumn
}

// deleteIsRefused reports whether a DELETE by app_tenant must come back 42501
// rather than reach zero rows. AppendOnly implies it; NoDeleteGrant is the case
// where UPDATE survives and only DELETE is gone.
func (tt TenantTable) deleteIsRefused() bool {
	return tt.AppendOnly || tt.NoDeleteGrant
}

// Check runs the ADR-0002 completion rule against one table and reports what
// failed, if anything.
//
// It returns an error rather than taking a *testing.T so that the runner itself
// can be tested — specifically, so a test can prove it FAILS on a table whose
// isolation is broken. Every table in this phase is declared safe on this
// runner's word, and a runner nobody has watched fail is an assumption wearing
// a test's clothes.
//
// The sequence, all of it as app_tenant and never as the owner:
//
//  1. As A, insert row R.
//  2. As B, R is invisible to SELECT, and UPDATE and DELETE affect no rows.
//  3. As B, inserting a row stamped with A's shelter_id is refused (42501).
//  4. As A, R is still there.
//
// Step 4 matters more than it looks: without it, a policy that hid the row from
// everyone — including its owner — would pass steps 1 to 3 perfectly.
func (tt TenantTable) Check(ctx context.Context, env *Env) error {
	if tt.Name == "" || tt.Insert == nil {
		return errors.New("dbtest: a TenantTable needs a Name and an Insert")
	}

	if tt.Fixture != nil {
		if err := tt.Fixture(ctx, env); err != nil {
			return fmt.Errorf("%s: preparing the rows this case depends on: %w", tt.Name, err)
		}
	}

	var key RowKey
	err := db.WithTenant(ctx, env.TenantPool, env.ShelterA,
		func(ctx context.Context, tx pgx.Tx) error {
			var err error
			key, err = tt.Insert(ctx, tx, env.ShelterA)

			return err
		})
	if err != nil {
		return fmt.Errorf("%s: seeding tenant A's row: %w", tt.Name, err)
	}
	if len(key) == 0 {
		return fmt.Errorf("%s: Insert returned an empty RowKey, so nothing can be asserted",
			tt.Name)
	}

	if err := tt.checkTenantBIsBlind(ctx, env, key); err != nil {
		return err
	}

	if err := tt.checkTenantBCannotWipeTheTable(ctx, env); err != nil {
		return err
	}

	if err := tt.checkTenantBCannotForgeOwnership(ctx, env); err != nil {
		return err
	}

	return tt.checkTenantAStillSeesItsRow(ctx, env, key)
}

func (tt TenantTable) checkTenantBIsBlind(ctx context.Context, env *Env, key RowKey) error {
	predicate, args := key.where(1)

	return db.WithTenant(ctx, env.TenantPool, env.ShelterB,
		func(ctx context.Context, tx pgx.Tx) error {
			var visible int
			err := tx.QueryRow(ctx,
				fmt.Sprintf(`SELECT count(*) FROM %s WHERE %s`, tt.Name, predicate),
				args...).Scan(&visible)
			if err != nil {
				return fmt.Errorf("%s: tenant B reading tenant A's row: %w", tt.Name, err)
			}
			if visible != 0 {
				return fmt.Errorf("%s: tenant B can READ %d of tenant A's rows", tt.Name, visible)
			}

			// A no-op assignment: the point is whether the policy lets the
			// statement reach the row at all, not what it would write. The
			// column has to be one app_tenant may actually write, or a
			// privilege refusal answers before the policy is ever consulted —
			// see TouchColumn.
			if err := tt.probeWrite(ctx, tx, "WRITE", tt.AppendOnly,
				fmt.Sprintf(`UPDATE %s SET %s = %s WHERE %s`,
					tt.Name, tt.touchColumn(), tt.touchColumn(), predicate),
				args); err != nil {
				return err
			}

			return tt.probeWrite(ctx, tx, "DELETE", tt.deleteIsRefused(),
				fmt.Sprintf(`DELETE FROM %s WHERE %s`, tt.Name, predicate), args)
		})
}

// probeWrite runs one write that must not touch tenant A's row, and reads the
// result against whichever guarantee this table claims.
//
// On an ordinary table the statement is expected to SUCCEED and reach zero rows:
// app_tenant holds the grant, and the policy is the only thing standing between
// it and the row. On an append-only table it is expected to be REFUSED outright,
// because the grant itself was never given — so a zero-row success there would
// mean the grant came back and only the policy is holding the line.
// `refused` is passed per probe rather than read from AppendOnly, because since
// `00015_column_grants` a table can expect a zero-row UPDATE and a refused
// DELETE at the same time.
func (tt TenantTable) probeWrite(
	ctx context.Context, tx pgx.Tx, verb string, refused bool, statement string, args []any,
) error {
	if refused {
		// A refused statement ABORTS its transaction: every command after it
		// comes back 25P02 ("current transaction is aborted") no matter what it
		// is. On an ordinary table the probes succeed and never notice, but here
		// a refusal is the expected outcome, so the second probe would report
		// the abort instead of its own result — and a check that can only ever
		// see 25P02 proves nothing about the statement it was written for.
		//
		// A savepoint per probe contains the abort. It is not tidiness: without
		// it the DELETE probe is dead weight and its property is unasserted.
		sub, err := tx.Begin(ctx)
		if err != nil {
			return fmt.Errorf("%s: opening a savepoint for the %s probe: %w",
				tt.Name, verb, err)
		}
		defer func() { _ = sub.Rollback(ctx) }()

		return tt.expectRefusal(ctx, sub, verb, statement, args)
	}

	tag, err := tx.Exec(ctx, statement, args...)
	if err != nil {
		return fmt.Errorf("%s: tenant B's %s statement against tenant A's row: %w",
			tt.Name, verb, err)
	}
	if tag.RowsAffected() != 0 {
		return fmt.Errorf("%s: tenant B can %s %d of tenant A's rows",
			tt.Name, verb, tag.RowsAffected())
	}

	return nil
}

// expectRefusal runs one statement that must be rejected by the missing grant.
func (tt TenantTable) expectRefusal(
	ctx context.Context, tx pgx.Tx, verb, statement string, args []any,
) error {
	tag, err := tx.Exec(ctx, statement, args...)
	if err == nil {
		return fmt.Errorf("%s is append-only, but tenant B's %s statement was ACCEPTED "+
			"(%d rows). The UPDATE/DELETE grant has been restored, so the only thing left "+
			"refusing a write would be a policy — and there is deliberately no UPDATE or "+
			"DELETE policy on this table to refuse it",
			tt.Name, verb, tag.RowsAffected())
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != sqlstateInsufficientPrivilege {
		return fmt.Errorf("%s: tenant B's %s statement failed, but not with the missing "+
			"grant (%s), so what refused it is unproven: %w",
			tt.Name, verb, sqlstateInsufficientPrivilege, err)
	}

	return nil
}

// errRollback aborts a probe transaction whose work must never be kept.
var errRollback = errors.New("dbtest: rolling back a probe")

// checkTenantBCannotWipeTheTable is the only assertion here that can catch a
// broken DELETE policy, and it took mutation testing to notice that.
//
// The WHERE-qualified DELETE above cannot. PostgreSQL documents that "because
// DELETE commands often need to read data from columns (such as in a WHERE or
// RETURNING clause), SELECT rights are typically required... the appropriate
// SELECT or ALL policies are applied IN ADDITION to the DELETE policies". So
// while the SELECT policy is correct, a completely permissive DELETE policy is
// unreachable through a qualified statement, and the assertion above passes no
// matter how wrong the DELETE policy is.
//
// An unqualified `DELETE FROM t` reads no columns, so the SELECT policy never
// applies and the DELETE policy stands alone. That is also the shape of the real
// attack: tenant B destroys tenant A's rows without ever being able to see them.
//
// The probe always rolls back. A test that proves data can be destroyed must not
// destroy it.
//
// It compares against the rows tenant B can SEE rather than against zero.
// Deleting your own rows is allowed, and `shelters` is the table where that
// stops being hypothetical: tenant B owns a shelters row, because every other
// case's foreign keys need it to exist. Asserting zero there would fail on
// correct behaviour, and the obvious repair -- dropping the case for `shelters`
// -- would drop the only assertion that reaches a DELETE policy at all.
//
// The comparison is sound because checkTenantBIsBlind has already proven the
// SELECT policy correct, so the visible count IS tenant B's own rows. A
// permissive DELETE policy reaches every row in the table and exceeds it.
func (tt TenantTable) checkTenantBCannotWipeTheTable(ctx context.Context, env *Env) error {
	var visible, affected int64

	err := db.WithTenant(ctx, env.TenantPool, env.ShelterB,
		func(ctx context.Context, tx pgx.Tx) error {
			err := tx.QueryRow(ctx,
				fmt.Sprintf(`SELECT count(*) FROM %s`, tt.Name)).Scan(&visible)
			if err != nil {
				return fmt.Errorf("%s: counting tenant B's own rows: %w", tt.Name, err)
			}

			tag, err := tx.Exec(ctx, fmt.Sprintf(`DELETE FROM %s`, tt.Name))

			// Where DELETE was never granted the wipe must be refused outright,
			// not merely reach nothing. Tenant B deleting only its OWN rows is
			// allowed everywhere else in this suite; here it is exactly the
			// thing append-only exists to forbid, so a zero-row success would
			// be the failure, not the pass.
			if tt.deleteIsRefused() {
				if err == nil {
					return fmt.Errorf("%s: an unqualified DELETE by tenant B was ACCEPTED "+
						"(%d rows) on a table app_tenant holds no DELETE grant on",
						tt.Name, tag.RowsAffected())
				}

				var pgErr *pgconn.PgError
				if !errors.As(err, &pgErr) || pgErr.Code != sqlstateInsufficientPrivilege {
					return fmt.Errorf("%s: the unqualified DELETE was refused, but not by "+
						"the missing grant (%s): %w",
						tt.Name, sqlstateInsufficientPrivilege, err)
				}

				return errRollback
			}

			if err != nil {
				return fmt.Errorf("%s: tenant B attempting an unqualified delete: %w",
					tt.Name, err)
			}
			affected = tag.RowsAffected()

			return errRollback
		})
	if err != nil && !errors.Is(err, errRollback) {
		return err
	}

	if tt.deleteIsRefused() {
		return nil
	}

	if affected != visible {
		return fmt.Errorf("%s: an unqualified DELETE by tenant B removed %d rows, but only %d "+
			"are B's own. The DELETE policy is permissive; the SELECT policy was hiding that, "+
			"because a DELETE with no WHERE reads no columns and so never consults it",
			tt.Name, affected, visible)
	}

	return nil
}

// A permissive policy is invisible to the read assertions above: a row B cannot
// see is also a row B cannot forge. This is the write half, and it is what
// separates "the policy filters" from "the policy is correct".
func (tt TenantTable) checkTenantBCannotForgeOwnership(ctx context.Context, env *Env) error {
	err := db.WithTenant(ctx, env.TenantPool, env.ShelterB,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tt.Insert(ctx, tx, env.ShelterA)

			return err
		})

	if err == nil {
		return fmt.Errorf("%s: tenant B inserted a row stamped with tenant A's shelter_id. "+
			"The policy's WITH CHECK is missing or permissive", tt.Name)
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != sqlstateInsufficientPrivilege {
		return fmt.Errorf("%s: tenant B's forged insert failed, but not with a WITH CHECK "+
			"violation (%s). Something other than the policy refused it, so the policy is "+
			"still unproven: %w", tt.Name, sqlstateInsufficientPrivilege, err)
	}

	return nil
}

// Without this, a policy that hid the row from EVERYONE would sail through the
// assertions above. "Nobody can see it" is not isolation, it is a broken table.
func (tt TenantTable) checkTenantAStillSeesItsRow(ctx context.Context, env *Env, key RowKey) error {
	predicate, args := key.where(1)

	return db.WithTenant(ctx, env.TenantPool, env.ShelterA,
		func(ctx context.Context, tx pgx.Tx) error {
			var found int
			err := tx.QueryRow(ctx,
				fmt.Sprintf(`SELECT count(*) FROM %s WHERE %s`, tt.Name, predicate),
				args...).Scan(&found)
			if err != nil {
				return fmt.Errorf("%s: tenant A re-reading its own row: %w", tt.Name, err)
			}
			if found != 1 {
				return fmt.Errorf("%s: tenant A can no longer see its own row (found %d). "+
					"A policy that hides a row from its owner passes every isolation "+
					"assertion and is still wrong", tt.Name, found)
			}

			return nil
		})
}

// RunAB runs the completion rule for each table as its own subtest.
func RunAB(t *testing.T, env *Env, tables ...TenantTable) {
	t.Helper()

	for _, table := range tables {
		t.Run(table.Name, func(t *testing.T) {
			if err := table.Check(context.Background(), env); err != nil {
				t.Fatalf("tenant isolation is not proven: %v", err)
			}
		})
	}
}
