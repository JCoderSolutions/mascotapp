package dbtest_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
)

// TestMain enforces the rule that keeps the Docker escape hatch honest: it may
// be used on a developer machine, never in CI. A suite that can silently skip
// itself in CI is a suite that rots, and this phase's whole deliverable is the
// proof those tests produce.
func TestMain(m *testing.M) {
	if err := dbtest.GuardEscapeHatch(os.Getenv); err != nil {
		//nolint:forbidigo // TestMain has no *testing.T to report through.
		println("dbtest:", err.Error())
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// TestGuardEscapeHatch is a pure unit test: no Docker, no database, no process
// environment mutated. GuardEscapeHatch takes its lookup function so the four
// combinations can be asserted in parallel.
func TestGuardEscapeHatch(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		env     map[string]string
		wantErr bool
	}{
		{
			name:    "no escape hatch, not CI",
			env:     map[string]string{},
			wantErr: false,
		},
		{
			name:    "no escape hatch, in CI",
			env:     map[string]string{"CI": "true"},
			wantErr: false,
		},
		{
			name:    "escape hatch on a developer machine is allowed",
			env:     map[string]string{"MASCOTAPP_SKIP_DOCKER_TESTS": "1"},
			wantErr: false,
		},
		{
			// The one combination that must fail. Otherwise a green CI run
			// would prove nothing at all.
			name: "escape hatch in CI is refused",
			env: map[string]string{
				"MASCOTAPP_SKIP_DOCKER_TESTS": "1",
				"CI":                          "true",
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := dbtest.GuardEscapeHatch(func(key string) string {
				return tc.env[key]
			})

			if tc.wantErr && err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tc.wantErr && !errors.Is(err, dbtest.ErrEscapeHatchInCI) {
				t.Fatalf("expected ErrEscapeHatchInCI, got %v", err)
			}
		})
	}
}

// TestSkipReason pins the complete set of conditions under which the harness
// declines to start a container.
//
// The assertion that earns its keep is the LAST one: an unreachable Docker
// daemon is not a skip. That path cannot be exercised on a developer host —
// testcontainers resolves Docker Desktop's named pipe directly and ignores both
// DOCKER_HOST and DOCKER_CONTEXT (verified 2026-08-29: both were pointed at
// dead endpoints and the container started anyway). So the guarantee is pinned
// structurally instead: if SkipReason is the only skip branch in Postgres, and
// SkipReason says no for every environment except these two, then a startup
// failure has nowhere to go but t.Fatalf.
func TestSkipReason(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		short    bool
		env      map[string]string
		wantSkip bool
	}{
		{
			name:     "-short skips",
			short:    true,
			env:      map[string]string{},
			wantSkip: true,
		},
		{
			name:     "the escape hatch skips",
			short:    false,
			env:      map[string]string{"MASCOTAPP_SKIP_DOCKER_TESTS": "1"},
			wantSkip: true,
		},
		{
			name:     "an ordinary run does not skip",
			short:    false,
			env:      map[string]string{},
			wantSkip: false,
		},
		{
			// The one that matters. Nothing about a broken or absent daemon
			// reaches this decision, so a startup failure cannot become a skip.
			name:  "no daemon-related variable buys a skip",
			short: false,
			env: map[string]string{
				"DOCKER_HOST":                  "tcp://127.0.0.1:1",
				"DOCKER_CONTEXT":               "does-not-exist",
				"TESTCONTAINERS_RYUK_DISABLED": "true",
				"CI":                           "true",
			},
			wantSkip: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			reason, skip := dbtest.SkipReason(tc.short, func(key string) string {
				return tc.env[key]
			})

			if skip != tc.wantSkip {
				t.Fatalf("skip = %v, want %v (reason %q)", skip, tc.wantSkip, reason)
			}
			if skip && reason == "" {
				t.Fatal("skipped with an empty reason: the log would not say why")
			}
			if !skip && reason != "" {
				t.Fatalf("not skipping but returned reason %q", reason)
			}
		})
	}
}

// TestPostgres_RunsPostgres17 is the spike itself: it proves testcontainers-go
// starts a real PostgreSQL 17 under CGO_ENABLED=0 on this host. Every later
// integration task in this phase depends on this working.
func TestPostgres_RunsPostgres17(t *testing.T) {
	env := dbtest.Postgres(t)

	var versionNum int
	err := env.OwnerPool.QueryRow(
		context.Background(),
		"SELECT current_setting('server_version_num')::int",
	).Scan(&versionNum)
	if err != nil {
		t.Fatalf("querying server version: %v", err)
	}

	// 170000 <= v < 180000. Asserting the major version rather than an exact
	// build keeps this from breaking on a patch bump of the image.
	if versionNum < 170000 || versionNum >= 180000 {
		t.Fatalf("expected PostgreSQL 17, got server_version_num=%d", versionNum)
	}
}

// TestPostgres_OwnerCanCreateExtensionCitext proves the one extension this
// schema depends on is present in the compose image, which is the assumption
// design decision D7 rests on after the user restored §4's citext.
func TestPostgres_OwnerCanCreateExtensionCitext(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	if _, err := env.OwnerPool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS citext"); err != nil {
		t.Fatalf("creating citext: %v", err)
	}

	var version string
	err := env.OwnerPool.QueryRow(
		ctx,
		"SELECT extversion FROM pg_extension WHERE extname = 'citext'",
	).Scan(&version)
	if err != nil {
		t.Fatalf("reading citext version: %v", err)
	}
	if version == "" {
		t.Fatal("citext reported an empty version")
	}
}
