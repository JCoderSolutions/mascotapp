package db_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver

	"github.com/gentleman/mascotapp/apps/api/internal/db"
	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
	"github.com/gentleman/mascotapp/apps/api/internal/db/rlstest"
)

// The migration round-trip, driven by the embedded set rather than by a
// hand-written list, so it covers every migration this phase adds without
// anyone remembering to extend it. That is T-01-011's definition of done: the
// check runs after every new migration lands, not once.
//
// What the tests in migrate_integration_test.go prove is that
// `up → down-to 0 → up` works end to end. What they cannot see is the schema
// BETWEEN those endpoints — the same blind spot mutation testing found in
// T-01-007, one level up. With one migration the two are the same journey. With
// twelve, a Down that leaves a table without its policy, or drops a parent and
// orphans a child, is invisible to a round-trip that only inspects where it
// started and where it stopped.

// downState is what a rollback must never destroy, read from the server rather
// than assumed from the migration text.
type downState struct {
	TenantRole bool
	PublicRole bool
	AuthRole   bool
	Citext     bool
}

var (
	// errRoleDropped means a Down removed an application role. D1b forbids it.
	errRoleDropped = errors.New("a rollback dropped an application role")

	// errCitextDropped means a Down removed the extension, which CASCADEs.
	errCitextDropped = errors.New("a rollback dropped the citext extension")
)

// checkDownState is pure so every combination is asserted exhaustively rather
// than inferred from whichever migration happens to exist on the day.
func checkDownState(state downState, at int64) error {
	var problems []error

	for _, role := range []struct {
		name    string
		present bool
	}{
		{"app_tenant", state.TenantRole},
		{"app_public", state.PublicRole},
		{"app_auth", state.AuthRole},
	} {
		if !role.present {
			problems = append(problems, fmt.Errorf(
				"%w: rolling back to version %d dropped %s. CREATE ROLE is cluster-wide, "+
					"so a Down that drops it fails when the role owns objects and breaks "+
					"any other database in the same cluster using it",
				errRoleDropped, at, role.name))
		}
	}

	if !state.Citext {
		problems = append(problems, fmt.Errorf(
			"%w: rolling back to version %d dropped citext, which CASCADEs to users.email "+
				"and destroys the column a rollback is meant to leave recoverable",
			errCitextDropped, at))
	}

	return errors.Join(problems...)
}

// readDownState asks the server what survived.
func readDownState(ctx context.Context, pool *pgxpool.Pool) (downState, error) {
	var state downState
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM pg_roles     WHERE rolname = 'app_tenant'),
		       EXISTS (SELECT 1 FROM pg_roles     WHERE rolname = 'app_public'),
		       EXISTS (SELECT 1 FROM pg_roles     WHERE rolname = 'app_auth'),
		       EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'citext')`,
	).Scan(&state.TenantRole, &state.PublicRole, &state.AuthRole, &state.Citext)
	if err != nil {
		return downState{}, fmt.Errorf("reading the post-rollback state: %w", err)
	}

	return state, nil
}

// checkCatalogAt holds the T-01-010 rules that must be true at EVERY version,
// not only at HEAD, and reports how many model tables it actually inspected.
//
// CheckPending is deliberately not among them. Its job is to prove the pending
// ledger matches a fully migrated schema; halfway down the chain a declared
// table is absent for a legitimate reason, and asserting otherwise would fail
// for being correct.
//
// What remains is the pair that must hold whatever has been rolled back: every
// table that still exists is classified, and every table that still exists is
// still protected. A Down that drops a policy but leaves its table behind is
// caught here and nowhere else in the suite.
//
// It returns an error rather than taking a *testing.T so that a test can prove
// it FAILS on a broken schema — the same reason TenantTable.Check does.
func checkCatalogAt(ctx context.Context, pool *pgxpool.Pool, at int64) (int, error) {
	tables, err := rlstest.ReadTables(ctx, pool)
	if err != nil {
		return 0, fmt.Errorf("reading the catalog at version %d: %w", at, err)
	}

	names := make([]string, 0, len(tables))
	for _, table := range tables {
		names = append(names, table.Name)
	}

	var problems []error
	if err := rlstest.Schema.ClassifyAll(names); err != nil {
		problems = append(problems, fmt.Errorf(
			"at version %d a relation escapes classification: %w", at, err))
	}

	model := 0
	for _, table := range tables {
		// CheckProtectionAt, not CheckProtection: a table like refresh_tokens
		// can legitimately have RLS on with zero policies at an intermediate
		// version, if its first policy belongs to a migration later than `at`
		// (rlstest.Schema.PolicyLandsAt). That is a real, applied historical
		// state the walk passes through, not a rollback bug.
		if err := rlstest.Schema.CheckProtectionAt(table, at); err != nil {
			problems = append(problems, fmt.Errorf(
				"at version %d a table is left unprotected by a rollback: %w", at, err))
		}
		if rlstest.Schema.IsModelTable(table.Name) {
			model++
		}
	}

	return model, errors.Join(problems...)
}

// checkPolicyLandsAtIsHonest binds rlstest.Schema.PolicyLandsAt to the schema
// the walk is actually looking at.
//
// PolicyLandsAt is an exemption, and an exemption nobody checks against
// reality drifts silently in the permissive direction -- the exact failure the
// catalog exists to prevent, and the reason its other lists are derived rather
// than hand-written.
//
// A version declared too LOW is already caught, by CheckProtectionAt demanding
// a policy that has not landed yet. Too HIGH is the dangerous direction and is
// caught only here: the exemption would cover versions at which the policy
// already exists, silencing the walk over real, checkable states.
//
// This lives in the WALK and not in checkCatalogAt on purpose. checkCatalogAt
// takes `at` as a caller-supplied label and is called by unit tests against a
// fully migrated database, where "version 1" describes nothing about the schema
// in front of it. Only here has the database really been rolled back to `at`,
// so only here does comparing the two mean anything.
func checkPolicyLandsAtIsHonest(ctx context.Context, pool *pgxpool.Pool, at int64) error {
	tables, err := rlstest.ReadTables(ctx, pool)
	if err != nil {
		return fmt.Errorf("reading the catalog at version %d: %w", at, err)
	}

	var problems []error
	for _, table := range tables {
		landsAt, tracked := rlstest.Schema.PolicyLandsAt[table.Name]
		if !tracked || at >= landsAt || table.PolicyCount == 0 {
			continue
		}
		problems = append(problems, fmt.Errorf(
			"at version %d %q already carries %d policy/policies, but PolicyLandsAt "+
				"declares its first one lands at %d. The declaration is too high, so the "+
				"exemption is covering versions this walk should be checking",
			at, table.Name, table.PolicyCount, landsAt))
	}

	return errors.Join(problems...)
}

// appliedVersions reads what goose actually recorded, which is not the same
// question as what the embedded set contains.
func appliedVersions(t *testing.T, pool *pgxpool.Pool) []int64 {
	t.Helper()

	rows, err := pool.Query(context.Background(),
		`SELECT version_id FROM goose_db_version WHERE is_applied AND version_id > 0
		 ORDER BY version_id`)
	if err != nil {
		t.Fatalf("reading goose_db_version: %v", err)
	}
	defer rows.Close()

	var versions []int64
	for rows.Next() {
		var version int64
		if err := rows.Scan(&version); err != nil {
			t.Fatalf("scanning a recorded version: %v", err)
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating recorded versions: %v", err)
	}

	return versions
}

// embeddedVersions is what shipped in the binary, in order.
func embeddedVersions(t *testing.T) []int64 {
	t.Helper()

	migrations := loadMigrations(t)
	versions := make([]int64, 0, len(migrations))
	for _, m := range migrations {
		versions = append(versions, int64(m.version))
	}

	return versions
}

// migrator is the slice of goose the stepwise walk needs. It exists so the walk
// can be driven by a fake: with one migration in the set the real walk has one
// step and nothing to detect, and mutation testing on 2026-08-30 confirmed that
// every assertion inside it could be deleted with the suite still green.
type migrator interface {
	Up(ctx context.Context) error
	DownTo(ctx context.Context, version int64) error
	Version(ctx context.Context) (int64, error)
}

// stepwiseWalk rolls back one migration at a time, calls inspect at every
// version the schema really occupies, and then climbs all the way back.
//
// One migration at a time is the whole point. `down-to 0` in a single hop only
// ever shows the bottom of the chain, and a production rollback stops at the
// intermediate versions — which is where a Down that drops a policy but keeps
// its table leaves the schema open.
func stepwiseWalk(
	ctx context.Context, m migrator, versions []int64, inspect func(at int64) error,
) error {
	if len(versions) == 0 {
		return errors.New("the migration set is empty, so the walk would assert nothing")
	}
	head := versions[len(versions)-1]

	at, err := m.Version(ctx)
	if err != nil {
		return err
	}
	if at != head {
		return fmt.Errorf("the schema starts at version %d, want %d (the highest embedded "+
			"migration). A migration that ships but never applies is a table that silently "+
			"does not exist in production", at, head)
	}

	for i := len(versions) - 1; i >= 0; i-- {
		target := int64(0)
		if i > 0 {
			target = versions[i-1]
		}

		if err := m.DownTo(ctx, target); err != nil {
			return fmt.Errorf("rolling back to %d: %w", target, err)
		}
		if at, err = m.Version(ctx); err != nil {
			return err
		}
		if at != target {
			return fmt.Errorf("after rolling back from %d the schema reports version %d, "+
				"want %d", versions[i], at, target)
		}
		if err := inspect(target); err != nil {
			return err
		}
	}

	// The climb back is what makes the descent a rollback rather than a
	// one-way trip.
	if err := m.Up(ctx); err != nil {
		return fmt.Errorf("goose up after a stepwise rollback: %w", err)
	}
	if at, err = m.Version(ctx); err != nil {
		return err
	}
	if at != head {
		return fmt.Errorf("after climbing back the schema is at version %d, want %d", at, head)
	}

	return inspect(head)
}

// gooseMigrator binds the walk to the real embedded migration set.
type gooseMigrator struct{ handle *sql.DB }

func (g gooseMigrator) Up(ctx context.Context) error { return db.Up(ctx, g.handle) }
func (g gooseMigrator) DownTo(ctx context.Context, v int64) error {
	return db.DownTo(ctx, g.handle, v)
}
func (g gooseMigrator) Version(ctx context.Context) (int64, error) {
	return db.Version(ctx, g.handle)
}

// The stepwise round-trip: down one migration at a time, checking the schema at
// every stop, then all the way back up.
func TestMigrations_EveryStepDownLeavesAConsistentSchema(t *testing.T) {
	// Destroys the schema, so it cannot share the package container.
	env := dbtest.PostgresIsolated(t)
	ctx := context.Background()
	handle := openSQL(t, env.OwnerDSN)

	if err := db.Up(ctx, handle); err != nil {
		t.Fatalf("goose up: %v", err)
	}

	checkedTables := 0
	inspect := func(at int64) error {
		state, err := readDownState(ctx, env.OwnerPool)
		if err != nil {
			return err
		}
		// At version 0 this overlaps TestMigrations_DownKeepsRolesAndExtension
		// on purpose: here it names the exact version whose Down broke the
		// invariant, instead of reporting that something in the chain did.
		if err := checkDownState(state, at); err != nil {
			return err
		}

		model, err := checkCatalogAt(ctx, env.OwnerPool, at)
		checkedTables += model
		if err != nil {
			return err
		}

		return checkPolicyLandsAtIsHonest(ctx, env.OwnerPool, at)
	}

	versions := embeddedVersions(t)
	if err := stepwiseWalk(ctx, gooseMigrator{handle}, versions, inspect); err != nil {
		t.Fatalf("the migration set does not survive a stepwise rollback: %v", err)
	}

	// Said out loud, because with one migration this walk has one step and
	// inspects no model table at all. A suite that reports its own coverage
	// cannot be mistaken for one that proved more than it did.
	t.Logf("walked %d migration(s); inspected %d model table(s) across the intermediate "+
		"versions", len(versions), checkedTables)
}

// errRecordedSetMismatch means goose applied something other than exactly the
// embedded set, in order.
var errRecordedSetMismatch = errors.New("the database does not record the embedded migration set")

// checkRecordedSet is pure, because with one migration in the set the two lists
// are trivially equal and the comparison proves nothing about itself.
//
// The gap it closes is real and quiet: the unit tests assert that migration
// FILENAMES ascend from one with no gaps, but nothing asserted that goose
// applied all of them, in that order, and nothing else. A migration that is
// embedded but skipped, or a row recorded for a version no file supplies,
// leaves the unit tests green and the database wrong.
func checkRecordedSet(recorded, embedded []int64) error {
	if len(recorded) != len(embedded) {
		return fmt.Errorf("%w: goose recorded %d applied migrations, the binary embeds %d\n"+
			"  recorded: %v\n  embedded: %v",
			errRecordedSetMismatch, len(recorded), len(embedded), recorded, embedded)
	}
	for i := range embedded {
		if recorded[i] != embedded[i] {
			return fmt.Errorf("%w: at position %d goose recorded version %d, the binary "+
				"embeds version %d\n  recorded: %v\n  embedded: %v",
				errRecordedSetMismatch, i, recorded[i], embedded[i], recorded, embedded)
		}
	}

	return nil
}

func TestCheckRecordedSet(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		recorded []int64
		embedded []int64
		wantErr  bool
		why      string
	}{
		{
			name:     "the database records exactly what shipped",
			recorded: []int64{1, 2, 3},
			embedded: []int64{1, 2, 3},
		},
		{
			name:     "a migration shipped but was never applied",
			recorded: []int64{1, 2},
			embedded: []int64{1, 2, 3},
			wantErr:  true,
			why:      "a table that silently does not exist in production",
		},
		{
			name:     "a version was applied that no file supplies",
			recorded: []int64{1, 2, 3},
			embedded: []int64{1, 2},
			wantErr:  true,
			why:      "the database carries schema no migration in this repository describes",
		},
		{
			name:     "the versions were applied out of order",
			recorded: []int64{1, 3, 2},
			embedded: []int64{1, 2, 3},
			wantErr:  true,
			why:      "a Down would then unwind in an order the Ups never ran in",
		},
		{
			name:     "nothing was applied at all",
			recorded: nil,
			embedded: []int64{1},
			wantErr:  true,
			why:      "goose up succeeded and did nothing",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := checkRecordedSet(tc.recorded, tc.embedded)
			if tc.wantErr {
				if !errors.Is(err, errRecordedSetMismatch) {
					t.Fatalf("accepted a mismatch that means %s: %v", tc.why, err)
				}

				return
			}
			if err != nil {
				t.Fatalf("rejected a database that matches the embedded set: %v", err)
			}
		})
	}
}

// The integration half: what goose actually recorded against what shipped.
func TestMigrations_DatabaseRecordsExactlyTheEmbeddedSet(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()
	handle := openSQL(t, env.OwnerDSN)

	if err := db.Up(ctx, handle); err != nil {
		t.Fatalf("goose up: %v", err)
	}

	if err := checkRecordedSet(appliedVersions(t, env.OwnerPool), embeddedVersions(t)); err != nil {
		t.Fatalf("%v", err)
	}
}

// checkDownState is pure, so every way a rollback can destroy something is
// asserted here rather than waiting for a migration that does it.
func TestCheckDownState(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		state   downState
		wantErr error
		wants   string
		why     string
	}{
		{
			name:  "everything survived",
			state: downState{TenantRole: true, PublicRole: true, AuthRole: true, Citext: true},
		},
		{
			name:    "app_tenant was dropped",
			state:   downState{PublicRole: true, AuthRole: true, Citext: true},
			wantErr: errRoleDropped,
			wants:   "app_tenant",
			why:     "the role the whole isolation model connects as",
		},
		{
			name:    "app_public was dropped",
			state:   downState{TenantRole: true, AuthRole: true, Citext: true},
			wantErr: errRoleDropped,
			wants:   "app_public",
			why:     "the public catalog would have no role to read as",
		},
		{
			name:    "app_auth was dropped",
			state:   downState{TenantRole: true, PublicRole: true, Citext: true},
			wantErr: errRoleDropped,
			wants:   "app_auth",
			why:     "the auth door (P2-D1) would have no role to read or write through",
		},
		{
			name:    "citext was dropped",
			state:   downState{TenantRole: true, PublicRole: true, AuthRole: true},
			wantErr: errCitextDropped,
			wants:   "CASCADEs",
			why:     "dropping it takes users.email with it, so the rollback is not one",
		},
		{
			name:    "everything was dropped",
			state:   downState{},
			wantErr: errRoleDropped,
			wants:   "app_tenant",
			why:     "the loudest possible Down",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := checkDownState(tc.state, 3)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("refused a clean rollback: %v", err)
				}

				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v, want %v — %s", err, tc.wantErr, tc.why)
			}
			if !strings.Contains(err.Error(), tc.wants) {
				t.Fatalf("the failure does not mention %q, so nobody could act on it: %v",
					tc.wants, err)
			}
			if !strings.Contains(err.Error(), "version 3") {
				t.Errorf("the failure does not name the version whose Down broke it: %v", err)
			}
		})
	}
}

// checkCatalogAt is the half of the stepwise check that no migration exercises
// yet, so it is proven against schemas broken the two ways a Down can break
// one. Every intermediate version in this phase will be declared sound on this
// function's word, and a check nobody has watched fail is an assumption wearing
// a test's clothes.
func TestCheckCatalogAt_FailsOnABrokenIntermediateSchema(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	t.Run("a table left behind with no policy", func(t *testing.T) {
		// The shape of a Down that drops a policy and forgets its table: the
		// table is DECLARED, so the exhaustiveness rule stays green, and it sits
		// open at that intermediate version.
		//
		// The name is taken from the pending ledger rather than written here.
		// It used to be a literal `pets`, chosen because pets was declared and
		// not yet created — and the day T-01-019 created it, this test failed
		// with "relation already exists". Eleven declared tables are still
		// pending, so a second literal would only postpone the same failure
		// eleven times.
		probe := firstPendingTable(t)
		dbtest.MustExec(t, env.OwnerPool,
			fmt.Sprintf(`CREATE TABLE %s (id uuid PRIMARY KEY)`, probe))
		t.Cleanup(func() {
			_, _ = env.OwnerPool.Exec(ctx, fmt.Sprintf(`DROP TABLE IF EXISTS %s`, probe))
		})

		_, err := checkCatalogAt(ctx, env.OwnerPool, 5)
		if !errors.Is(err, rlstest.ErrRLSNotEnabled) {
			t.Fatalf("a declared table with no RLS passed the intermediate check: %v", err)
		}
		if !strings.Contains(err.Error(), "left unprotected by a rollback") {
			t.Errorf("the protection rule is not the one that fired: %v", err)
		}
		if !strings.Contains(err.Error(), "version 5") {
			t.Errorf("the failure does not name the version: %v", err)
		}
	})

	t.Run("a relation a rollback orphaned", func(t *testing.T) {
		dbtest.MustExec(t, env.OwnerPool, `CREATE TABLE leftover_from_a_down (id uuid PRIMARY KEY)`)
		t.Cleanup(func() {
			_, _ = env.OwnerPool.Exec(ctx, `DROP TABLE IF EXISTS leftover_from_a_down`)
		})

		_, err := checkCatalogAt(ctx, env.OwnerPool, 7)
		if !errors.Is(err, rlstest.ErrUnclassifiedRelation) {
			t.Fatalf("a relation nobody declared passed the intermediate check: %v", err)
		}
		if !strings.Contains(err.Error(), "leftover_from_a_down") {
			t.Errorf("the failure does not name the relation: %v", err)
		}
		// Both rules answer an unclassified relation with the same sentinel, so
		// the sentinel alone cannot say which one fired. Mutation testing caught
		// exactly that: disabling the exhaustiveness rule left this green
		// because the protection rule covered for it.
		if !strings.Contains(err.Error(), "escapes classification") {
			t.Errorf("the exhaustiveness rule is not the one that fired, so it could be "+
				"deleted with this test still passing: %v", err)
		}
	})

	t.Run("a clean intermediate schema passes", func(t *testing.T) {
		if _, err := checkCatalogAt(ctx, env.OwnerPool, 1); err != nil {
			t.Fatalf("rejected the schema this phase actually produces: %v", err)
		}
	})
}

// firstPendingTable returns a table the declaration knows about and the schema
// does not have yet, which is exactly the shape this probe needs: classified, so
// the exhaustiveness rule stays green, and absent, so creating it is possible.
//
// Deterministic rather than arbitrary — the ledger is sorted — so a failure names
// the same table on every run.
func firstPendingTable(t *testing.T) string {
	t.Helper()

	pending := make([]string, 0, len(rlstest.Schema.Pending))
	for name := range rlstest.Schema.Pending {
		pending = append(pending, name)
	}
	if len(pending) == 0 {
		t.Skip("every declared table has landed, so a declared-but-absent table — the " +
			"exact shape this probe needs — no longer exists. Delete this subtest when " +
			"the phase closes rather than inventing a name the declaration does not know.")
	}
	sort.Strings(pending)

	return pending[0]
}

// fakeMigrator records what the walk asked for, so the walk's own behaviour can
// be asserted instead of inferred from a one-migration set that has nothing to
// go wrong.
type fakeMigrator struct {
	at int64

	// downTo is every rollback target the walk requested, in order.
	downTo []int64
	// ups counts the climbs back.
	ups int

	// stickAt makes DownTo silently refuse to move below this version, which is
	// what a Down that does not undo its Up looks like from the outside.
	stickAt *int64
	// failUp makes the climb back fail.
	failUp bool
	// upTo makes the climb back land somewhere other than HEAD, which is what
	// a migration whose Up silently no-ops looks like from the outside.
	upTo *int64
}

func (f *fakeMigrator) Up(_ context.Context) error {
	f.ups++
	if f.failUp {
		return errors.New("synthetic: goose up refused")
	}
	f.at = 3
	if f.upTo != nil {
		f.at = *f.upTo
	}

	return nil
}

func (f *fakeMigrator) DownTo(_ context.Context, version int64) error {
	f.downTo = append(f.downTo, version)
	if f.stickAt != nil && version < *f.stickAt {
		return nil // recorded the request, did not move
	}
	f.at = version

	return nil
}

func (f *fakeMigrator) Version(_ context.Context) (int64, error) { return f.at, nil }

// The walk is the logic T-01-011 delivers, and with a single migration in the
// embedded set it has exactly one step. These assertions are what keep it
// honest until the other eleven migrations arrive.
func TestStepwiseWalk(t *testing.T) {
	t.Parallel()

	versions := []int64{1, 2, 3}

	t.Run("descends one migration at a time and climbs back", func(t *testing.T) {
		t.Parallel()

		fake := &fakeMigrator{at: 3}
		var inspected []int64

		err := stepwiseWalk(context.Background(), fake, versions, func(at int64) error {
			inspected = append(inspected, at)

			return nil
		})
		if err != nil {
			t.Fatalf("rejected a clean walk: %v", err)
		}

		// One hop per migration, never a single jump to zero: the intermediate
		// versions are the ones a production rollback actually stops at.
		want := []int64{2, 1, 0}
		if fmt.Sprint(fake.downTo) != fmt.Sprint(want) {
			t.Errorf("rolled back to %v, want %v", fake.downTo, want)
		}
		// Every stop is inspected, and so is HEAD after the climb.
		if got := fmt.Sprint(inspected); got != fmt.Sprint([]int64{2, 1, 0, 3}) {
			t.Errorf("inspected %v, want [2 1 0 3]", inspected)
		}
		if fake.ups != 1 {
			t.Errorf("climbed back %d times, want 1", fake.ups)
		}
	})

	t.Run("a Down that does not move is a failure", func(t *testing.T) {
		t.Parallel()

		floor := int64(2)
		fake := &fakeMigrator{at: 3, stickAt: &floor}

		err := stepwiseWalk(context.Background(), fake, versions, func(int64) error { return nil })
		if err == nil {
			t.Fatal("accepted a rollback that reported success without moving")
		}
		if !strings.Contains(err.Error(), "reports version") {
			t.Fatalf("the failure does not say the schema stayed put: %v", err)
		}
	})

	t.Run("an inspection failure stops the walk", func(t *testing.T) {
		t.Parallel()

		fake := &fakeMigrator{at: 3}
		sentinel := errors.New("synthetic: the schema is broken at this version")

		err := stepwiseWalk(context.Background(), fake, versions, func(at int64) error {
			if at == 1 {
				return sentinel
			}

			return nil
		})
		if !errors.Is(err, sentinel) {
			t.Fatalf("got %v, want the inspection's own error", err)
		}
		// It must stop there rather than carry on to the bottom: continuing
		// would report the last failure instead of the first.
		if fmt.Sprint(fake.downTo) != fmt.Sprint([]int64{2, 1}) {
			t.Errorf("kept walking after a broken version: %v", fake.downTo)
		}
	})

	t.Run("a failed climb back is a failure", func(t *testing.T) {
		t.Parallel()

		fake := &fakeMigrator{at: 3, failUp: true}

		err := stepwiseWalk(context.Background(), fake, versions, func(int64) error { return nil })
		if err == nil {
			t.Fatal("accepted a descent that could not be climbed back, which is not a rollback")
		}
	})

	t.Run("a climb back that stops short of HEAD is a failure", func(t *testing.T) {
		t.Parallel()

		short := int64(2)
		fake := &fakeMigrator{at: 3, upTo: &short}

		err := stepwiseWalk(context.Background(), fake, versions, func(int64) error { return nil })
		if err == nil {
			t.Fatal("accepted a descent whose climb back stopped short. The schema would " +
				"be left mid-rollback with the walk reporting success")
		}
		if !strings.Contains(err.Error(), "climbing back") {
			t.Fatalf("the failure does not say the climb back fell short: %v", err)
		}
	})

	t.Run("a schema not at HEAD is refused before anything is rolled back", func(t *testing.T) {
		t.Parallel()

		fake := &fakeMigrator{at: 2} // a migration shipped but never applied

		err := stepwiseWalk(context.Background(), fake, versions, func(int64) error { return nil })
		if err == nil {
			t.Fatal("walked a schema that was never fully migrated")
		}
		if len(fake.downTo) != 0 {
			t.Errorf("started rolling back an already-incomplete schema: %v", fake.downTo)
		}
	})

	t.Run("an empty migration set is refused rather than walked vacuously", func(t *testing.T) {
		t.Parallel()

		fake := &fakeMigrator{}
		if err := stepwiseWalk(context.Background(), fake, nil,
			func(int64) error { return nil }); err == nil {
			t.Fatal("an empty set produced a green walk that asserted nothing")
		}
	})
}
