package db_test

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver

	"github.com/gentleman/mascotapp/apps/api/internal/db"
	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
)

// openSQL gives goose a database/sql handle over the same container the rest of
// the harness uses. goose speaks database/sql; everything else here speaks pgx
// directly.
func openSQL(t *testing.T, dsn string) *sql.DB {
	t.Helper()

	handle, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("opening a database/sql handle: %v", err)
	}
	t.Cleanup(func() { _ = handle.Close() })

	return handle
}

// The unit tests in migrate_test.go only inspect the text of the migration set.
// This one proves the SQL actually executes, which no amount of string checking
// can.
func TestMigrations_ApplyToARealDatabase(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()
	handle := openSQL(t, env.OwnerDSN)

	if err := db.Up(ctx, handle); err != nil {
		t.Fatalf("goose up: %v", err)
	}

	version, err := db.Version(ctx, handle)
	if err != nil {
		t.Fatalf("reading schema version: %v", err)
	}
	if version < 1 {
		t.Fatalf("schema version = %d after migrating, want at least 1", version)
	}
}

// Replaying a migration must be a no-op. CREATE ROLE is cluster-wide, so
// without the pg_roles guard this fails the second time a second database in
// the same cluster migrates.
func TestMigrations_AreIdempotentOnReplay(t *testing.T) {
	// Destroys the schema, so it cannot share the package container.
	env := dbtest.PostgresIsolated(t)
	ctx := context.Background()
	handle := openSQL(t, env.OwnerDSN)

	if err := db.Up(ctx, handle); err != nil {
		t.Fatalf("first goose up: %v", err)
	}
	if err := db.DownTo(ctx, handle, 0); err != nil {
		t.Fatalf("goose down-to 0: %v", err)
	}
	if err := db.Up(ctx, handle); err != nil {
		t.Fatalf("second goose up after a full rollback: %v", err)
	}
}

// The single most important assertion in this file. Neon's neon_superuser
// carries BYPASSRLS and is granted automatically to any role created through
// the Console, CLI or API. A role with either attribute makes every policy in
// this schema a silent no-op.
func TestMigrations_CreateRolesThatCannotBypassRLS(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()
	handle := openSQL(t, env.OwnerDSN)

	if err := db.Up(ctx, handle); err != nil {
		t.Fatalf("goose up: %v", err)
	}

	for _, role := range []string{"app_tenant", "app_public"} {
		t.Run(role, func(t *testing.T) {
			var super, bypass, canLogin bool
			err := env.OwnerPool.QueryRow(ctx,
				`SELECT rolsuper, rolbypassrls, rolcanlogin FROM pg_roles WHERE rolname = $1`,
				role,
			).Scan(&super, &bypass, &canLogin)
			if err != nil {
				t.Fatalf("reading %s from pg_roles: %v", role, err)
			}

			if super {
				t.Errorf("%s is SUPERUSER: RLS does not apply to it at all", role)
			}
			if bypass {
				t.Errorf("%s has BYPASSRLS: every policy in this schema would be a no-op", role)
			}
			if !canLogin {
				t.Errorf("%s cannot log in, so the application could never connect as it", role)
			}
		})
	}
}

// D7 rests on citext being installable in both target environments. The unit
// tests cannot see whether the statement succeeded.
func TestMigrations_InstallCitextAndNothingElse(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()
	handle := openSQL(t, env.OwnerDSN)

	if err := db.Up(ctx, handle); err != nil {
		t.Fatalf("goose up: %v", err)
	}

	rows, err := env.OwnerPool.Query(ctx,
		`SELECT extname FROM pg_extension WHERE extname <> 'plpgsql' ORDER BY extname`)
	if err != nil {
		t.Fatalf("listing extensions: %v", err)
	}
	defer rows.Close()

	var installed []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scanning extension name: %v", err)
		}
		installed = append(installed, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating extensions: %v", err)
	}

	if len(installed) != 1 || installed[0] != "citext" {
		t.Fatalf("installed extensions = %v, want exactly [citext]", installed)
	}
}

// The bootstrap is the one place a secret touches the database, and it has
// never run until here.
func TestSetRolePassword_LetsTheRoleConnect(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()
	handle := openSQL(t, env.OwnerDSN)

	if err := db.Up(ctx, handle); err != nil {
		t.Fatalf("goose up: %v", err)
	}

	// A password with a quote and a backslash in it: if anything concatenated
	// instead of binding, this is where it breaks.
	const password = `p'a"ss\word`

	if err := db.SetRolePassword(ctx, handle, "app_tenant", password); err != nil {
		t.Fatalf("setting the password: %v", err)
	}

	var stored string
	err := env.OwnerPool.QueryRow(ctx,
		`SELECT rolpassword FROM pg_authid WHERE rolname = 'app_tenant'`).Scan(&stored)
	if err != nil {
		t.Fatalf("reading the stored credential: %v", err)
	}
	if stored == "" {
		t.Fatal("app_tenant still has no credential")
	}
	if stored == password {
		t.Fatal("the password is stored in the clear")
	}
}

// The transaction-local settings must not survive the bootstrap. If they did,
// the next caller on that pooled connection would inherit a staged password.
func TestSetRolePassword_LeavesNoSettingBehind(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()
	handle := openSQL(t, env.OwnerDSN)

	if err := db.Up(ctx, handle); err != nil {
		t.Fatalf("goose up: %v", err)
	}
	if err := db.SetRolePassword(ctx, handle, "app_public", "another-secret"); err != nil {
		t.Fatalf("setting the password: %v", err)
	}

	// PostgreSQL returns NULL, not an empty string, for a setting that was
	// never set on this session -- which is exactly the state we want to see.
	// Scanning into a pointer keeps "absent" and "present but empty" distinct.
	var staged *string
	err := env.OwnerPool.QueryRow(ctx,
		`SELECT current_setting('app.bootstrap_password', true)`).Scan(&staged)
	if err != nil {
		t.Fatalf("reading the setting back: %v", err)
	}
	if staged != nil && *staged != "" {
		t.Fatalf("the staged password outlived its transaction: %q", *staged)
	}
}

// The asymmetry design decision D1b demands, and the gap mutation testing found
// on 2026-08-29: TestMigrations_AreIdempotentOnReplay passed even with
// `DROP ROLE` added to the Down, because the following Up recreated the roles.
// Nothing looked at the state BETWEEN the two.
//
// It matters because CREATE ROLE is cluster-wide. Dropping a role on the way
// down would fail if it owns objects, and would break a second database in the
// same cluster that is using it. The extension is the same shape of hazard:
// DROP EXTENSION citext cascades to users.email and destroys the column that a
// rollback is supposed to leave recoverable.
func TestMigrations_DownKeepsRolesAndExtension(t *testing.T) {
	// Destroys the schema, so it cannot share the package container.
	env := dbtest.PostgresIsolated(t)
	ctx := context.Background()
	handle := openSQL(t, env.OwnerDSN)

	if err := db.Up(ctx, handle); err != nil {
		t.Fatalf("goose up: %v", err)
	}
	if err := db.DownTo(ctx, handle, 0); err != nil {
		t.Fatalf("goose down-to 0: %v", err)
	}

	for _, role := range []string{"app_tenant", "app_public"} {
		var exists bool
		err := env.OwnerPool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)`, role).Scan(&exists)
		if err != nil {
			t.Fatalf("looking for %s: %v", role, err)
		}
		if !exists {
			t.Errorf("the Down dropped %s. D1b forbids it: CREATE ROLE is cluster-wide, "+
				"so this would fail if the role owns objects and would break another "+
				"database in the same cluster that is using it", role)
		}
	}

	var citext bool
	err := env.OwnerPool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'citext')`).Scan(&citext)
	if err != nil {
		t.Fatalf("looking for citext: %v", err)
	}
	if !citext {
		t.Error("the Down dropped citext, which cascades to users.email and destroys " +
			"the column a rollback is meant to leave recoverable")
	}

	// The grants, unlike the roles, MUST be gone: that is what the Down owns.
	var hasUsage bool
	err = env.OwnerPool.QueryRow(ctx,
		`SELECT has_schema_privilege('app_tenant', 'public', 'USAGE')`).Scan(&hasUsage)
	if err != nil {
		t.Fatalf("checking the schema grant: %v", err)
	}
	if hasUsage {
		t.Error("the Down left app_tenant with USAGE on schema public; revoking grants " +
			"is precisely what it is responsible for")
	}
}
