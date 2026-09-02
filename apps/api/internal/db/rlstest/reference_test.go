package rlstest_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gentleman/mascotapp/apps/api/internal/db"
	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
	"github.com/gentleman/mascotapp/apps/api/internal/db/rlstest"
)

// referenceTables are the global ones: no shelter_id, not in the tenant set,
// readable by both application roles and writable by neither.
var referenceTables = []string{"species", "breeds"}

// The DoD of T-01-018, asserted rather than assumed: reference data carries no
// shelter_id.
//
// It is worth a test because the column is how every other table in this schema
// is protected, so adding it here is the natural mistake — and it would be worse
// than useless. A `shelter_id` on `breeds` would make the tenant policy template
// applicable, someone would apply it, and the catalog would show each shelter
// only the breeds it had happened to create.
func TestReferenceData_CarriesNoShelterID(t *testing.T) {
	env := dbtest.Postgres(t)

	for _, table := range referenceTables {
		t.Run(table, func(t *testing.T) {
			var present bool
			err := env.OwnerPool.QueryRow(context.Background(), `
				SELECT EXISTS (
					SELECT 1 FROM pg_attribute
					WHERE attrelid = ('public.' || $1)::regclass
					  AND attname = 'shelter_id'
					  AND NOT attisdropped
				)`, table).Scan(&present)
			if err != nil {
				t.Fatalf("reading %s's columns: %v", table, err)
			}
			if present {
				t.Errorf("%s carries a shelter_id. It is global reference data: a breed "+
					"does not belong to a shelter, and scoping it would show each shelter "+
					"only the breeds it created", table)
			}

			for _, declared := range rlstest.Schema.Tenant {
				if declared == table {
					t.Errorf("%s is declared in the tenant set. It has no shelter_id, so "+
						"the tenant policy template cannot apply to it and its A/B case "+
						"could never be written", table)
				}
			}
		})
	}
}

// Both roles read the SAME rows. That is the property "global" actually means,
// and it is not implied by either role merely being able to read.
func TestReferenceData_BothRolesSeeTheSameRows(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	for _, table := range referenceTables {
		t.Run(table, func(t *testing.T) {
			// As the owner, which bypasses nothing here but is the reference
			// point: whatever is on disk is what both roles must see.
			var onDisk int
			if err := env.OwnerPool.QueryRow(ctx,
				`SELECT count(*) FROM `+table).Scan(&onDisk); err != nil {
				t.Fatalf("counting %s as the owner: %v", table, err)
			}
			if onDisk == 0 {
				t.Fatalf("%s is empty, so every count below would agree for the wrong "+
					"reason", table)
			}

			// app_tenant, under a scope, and under a DIFFERENT scope: the rows
			// must not move with the tenant.
			for _, shelter := range []uuid.UUID{env.ShelterA, env.ShelterB} {
				var seen int
				err := db.WithTenant(ctx, env.TenantPool, shelter,
					func(ctx context.Context, tx pgx.Tx) error {
						return tx.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&seen)
					})
				if err != nil {
					t.Fatalf("reading %s as app_tenant: %v", table, err)
				}
				if seen != onDisk {
					t.Errorf("app_tenant scoped to %s sees %d rows of %s, the owner sees %d. "+
						"Reference data must not vary by tenant", shelter, seen, table, onDisk)
				}
			}

			// app_public, through the read-only public path.
			var public int
			err := db.WithPublic(ctx, env.PublicPool,
				func(ctx context.Context, tx pgx.Tx) error {
					return tx.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&public)
				})
			if err != nil {
				t.Fatalf("reading %s as app_public: %v", table, err)
			}
			if public != onDisk {
				t.Errorf("app_public sees %d rows of %s, the owner sees %d. The public "+
					"catalog needs the same breed list the shelters use", public, table, onDisk)
			}
		})
	}
}

// Read-only, for both roles, on all three write commands.
//
// Two independent layers refuse each of these — no policy for the command, and
// no grant — and the test does not care which one fired, only that something
// did and that it was a privilege refusal rather than, say, a constraint.
func TestReferenceData_NeitherRoleCanWrite(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	// One real species id, so an UPDATE or DELETE has something to match and a
	// refusal cannot be "no rows".
	var speciesID uuid.UUID
	if err := env.OwnerPool.QueryRow(ctx,
		`SELECT id FROM species WHERE code = 'dog'`).Scan(&speciesID); err != nil {
		t.Fatalf("reading a seeded species: %v", err)
	}

	statements := map[string][]struct {
		name string
		sql  string
		args []any
	}{
		"species": {
			{"insert", `INSERT INTO species (id, code, name) VALUES ($1, 'ferret', 'Hurón')`,
				[]any{uuid.New()}},
			{"update", `UPDATE species SET name = 'Cambiado' WHERE id = $1`, []any{speciesID}},
			{"delete", `DELETE FROM species WHERE id = $1`, []any{speciesID}},
		},
		"breeds": {
			{"insert", `INSERT INTO breeds (id, species_id, name) VALUES ($1, $2, 'Inventado')`,
				[]any{uuid.New(), speciesID}},
			{"update", `UPDATE breeds SET name = 'Cambiado' WHERE species_id = $1`,
				[]any{speciesID}},
			{"delete", `DELETE FROM breeds WHERE species_id = $1`, []any{speciesID}},
		},
	}

	for _, table := range referenceTables {
		for _, stmt := range statements[table] {
			t.Run(table+"/"+stmt.name+"/app_tenant", func(t *testing.T) {
				err := db.WithTenant(ctx, env.TenantPool, env.ShelterA,
					func(ctx context.Context, tx pgx.Tx) error {
						_, err := tx.Exec(ctx, stmt.sql, stmt.args...)

						return err
					})
				requireInsufficientPrivilege(t, err,
					"app_tenant wrote to "+table+", which is global reference data one "+
						"shelter must not be able to edit for everyone else")
			})

			t.Run(table+"/"+stmt.name+"/app_public", func(t *testing.T) {
				err := writeAsPublic(ctx, env.PublicPool, stmt.sql, stmt.args...)
				if err == nil {
					t.Fatalf("app_public wrote to %s. It is the anonymous catalog role and "+
						"holds SELECT and nothing else", table)
				}
				// WithPublic opens the transaction READ ONLY, so the refusal may
				// come from the transaction (25006) before the grant (42501) is
				// ever consulted. Either is correct; the point is that it is
				// refused, and by the access layer rather than by data.
				var pgErr *pgconn.PgError
				if !errors.As(err, &pgErr) ||
					(pgErr.Code != sqlstateInsufficientPrivilege && pgErr.Code != "25006") {
					t.Fatalf("the write failed, but not with a privilege or read-only "+
						"refusal: %v", err)
				}
			})
		}
	}
}

func writeAsPublic(ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) error {
	return db.WithPublic(ctx, pool, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, sql, args...)

		return err
	})
}

// Idempotent seeds, asserted against the real seed SQL.
//
// The spec's own scenario for this — apply, reverse, apply again, count — CANNOT
// FAIL: `down-to 0` drops the tables, so the second apply always seeds a clean
// slate no matter how the INSERT is written. It is satisfied by the round-trip
// suite already, and it proves nothing about idempotency.
//
// The failure that actually exists is replaying the seed against a database that
// already holds the rows: a second deployment path, a hand-run migration, a
// restored dump. So this reads the statements between the markers in the
// migration and executes them a second time against the live schema. It fails if
// the file's markers go missing, because a test that silently found nothing to
// run would be the same kind of lie.
func TestReferenceData_SeedsAreIdempotentOnReplay(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	seeds := seedStatements(t)

	before := map[string]int{}
	for _, table := range referenceTables {
		var n int
		if err := env.OwnerPool.QueryRow(ctx,
			`SELECT count(*) FROM `+table).Scan(&n); err != nil {
			t.Fatalf("counting %s: %v", table, err)
		}
		if n == 0 {
			t.Fatalf("%s was never seeded, so replaying the seed proves nothing", table)
		}
		before[table] = n
	}

	if _, err := env.OwnerPool.Exec(ctx, seeds); err != nil {
		t.Fatalf("replaying the seed statements failed. They must be a no-op against a "+
			"database that already holds the rows: %v", err)
	}

	for _, table := range referenceTables {
		var after int
		if err := env.OwnerPool.QueryRow(ctx,
			`SELECT count(*) FROM `+table).Scan(&after); err != nil {
			t.Fatalf("counting %s: %v", table, err)
		}
		if after != before[table] {
			t.Errorf("%s went from %d rows to %d on a seed replay. The ON CONFLICT target "+
				"is not the key the seed actually collides on", table, before[table], after)
		}
	}
}

// seedStatements returns the SQL between the seed markers in the migration.
func seedStatements(t *testing.T) string {
	t.Helper()

	const (
		begin = "-- seeds:begin"
		end   = "-- seeds:end"
	)

	path := filepath.Join(migrationsDir(t), "00004_reference_data.sql")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the migration: %v", err)
	}

	text := string(body)
	from := strings.Index(text, begin)
	to := strings.Index(text, end)
	if from < 0 || to < 0 || to <= from {
		t.Fatalf("the %q / %q markers are missing from 00004_reference_data.sql. They are "+
			"what lets this test execute the REAL seed rather than a copy of it that drifts",
			begin, end)
	}

	seeds := strings.TrimSpace(text[from+len(begin) : to])
	if !strings.Contains(seeds, "INSERT INTO species") ||
		!strings.Contains(seeds, "INSERT INTO breeds") {
		t.Fatalf("the seed block does not insert both tables, so this test would replay "+
			"only part of it:\n%s", seeds)
	}

	return seeds
}

// migrationsDir walks up to the module root and returns the migrations
// directory, so the test does not depend on how deep the package sits.
func migrationsDir(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("locating the working directory: %v", err)
	}
	for {
		candidate := filepath.Join(dir, "internal", "db", "migrations")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find internal/db/migrations above the working directory")
		}
		dir = parent
	}
}
