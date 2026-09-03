// Package dbtest provides the PostgreSQL harness every integration test in
// this phase builds on.
//
// It is a normal package rather than a _test one on purpose: the RLS suite
// lives in a separate external test package and imports these helpers.
//
// # Why this package decides between skipping and failing
//
// The isolation tests are this phase's actual deliverable. A suite that
// quietly skips itself proves nothing while looking green, which is the exact
// failure mode row-level security is being introduced to eliminate. So the
// three ways this harness can decline to run are deliberately not the same:
//
//   - `-short` SKIPS. An explicit request for the fast path, chosen per run.
//   - MASCOTAPP_SKIP_DOCKER_TESTS=1 SKIPS, but only off CI. It is the escape
//     hatch for a developer whose Docker daemon is down, and GuardEscapeHatch
//     refuses it in CI so it can never buy a green build.
//   - An unreachable daemon FAILS. Nobody asked for the fast path here;
//     something is broken, and a skip would hide it.
package dbtest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver, for goose
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/gentleman/mascotapp/apps/api/internal/db"
)

const (
	// image must track docker-compose.yml. The whole point of running
	// migrations against a container is that it behaves like the local
	// database, and a version drift silently removes that guarantee.
	image = "postgres:17-alpine"

	ownerRole = "mascotapp"
	ownerPass = "mascotapp"
	database  = "mascotapp"

	// skipEnvVar is the developer escape hatch. Never honoured in CI.
	skipEnvVar = "MASCOTAPP_SKIP_DOCKER_TESTS"
	ciEnvVar   = "CI"

	// tenantRole and publicRole are created by migration 00001. They are named
	// here, not configured: the migration is the single source of truth, and a
	// mismatch should fail loudly rather than silently connect as someone else.
	tenantRole = "app_tenant"
	publicRole = "app_public"
	authRole   = "app_auth"

	// Test-only credentials for a throwaway container. Passwords are never in
	// the migration set (D8), so the harness sets them the same way production
	// does, through db.SetRolePassword.
	tenantPass = "tenant-test-password"
	publicPass = "public-test-password"
	authPass   = "auth-test-password"
)

// ErrEscapeHatchInCI is returned by GuardEscapeHatch when the Docker escape
// hatch is enabled in CI.
var ErrEscapeHatchInCI = errors.New(
	skipEnvVar + " must never be set in CI: it would let the isolation suite " +
		"skip itself and report a green build that proves nothing",
)

// Env is the live database handed to an integration test.
type Env struct {
	// OwnerPool connects as the owning role. It is for migrations and fixture
	// setup only. Isolation assertions MUST NOT use it — the owner is exactly
	// the role that would make every policy look like it works.
	OwnerPool *pgxpool.Pool

	// OwnerDSN is the connection string behind OwnerPool, for the goose runner
	// and for building further pools against the same container.
	OwnerDSN string

	// OwnerRole is the name the owner connects as. The guard needs it: a role
	// with clean attributes that still owns the objects is not bound by their
	// policies.
	OwnerRole string

	// TenantPool connects as app_tenant. Every A/B isolation assertion runs
	// through this pool, and never through OwnerPool.
	TenantPool *pgxpool.Pool

	// PublicPool connects as app_public, read-only, for the public catalog.
	PublicPool *pgxpool.Pool

	// AuthPool connects as app_auth, the identity door added in Phase 02. It is
	// a THIRD pool rather than a second GUC on TenantPool: app.user_id and
	// app.shelter_id must never be live on one connection (P2-D1).
	AuthPool *pgxpool.Pool

	// ShelterA and ShelterB are the two tenants every A/B assertion is written
	// against. They are generated per harness rather than fixed so a test can
	// never accidentally depend on a literal id, and they are always distinct:
	// comparing a tenant with itself proves nothing.
	ShelterA uuid.UUID
	ShelterB uuid.UUID

	// guarded records which roles passed the bypass check during setup, so a
	// test can prove the harness ran it rather than trusting that it did.
	guarded map[string]RoleCapabilities
}

// GuardedRoles reports the roles this harness verified cannot bypass row-level
// security. It exists so the setup-time guarantee is observable: without it,
// deleting the guard from the harness breaks no test.
func (e *Env) GuardedRoles() map[string]RoleCapabilities {
	out := make(map[string]RoleCapabilities, len(e.guarded))
	for role, caps := range e.guarded {
		out[role] = caps
	}

	return out
}

// GuardEscapeHatch reports whether the Docker escape hatch is set in CI, which
// is refused. It takes its environment lookup so callers can test all four
// combinations in parallel without mutating the process environment.
func GuardEscapeHatch(getenv func(string) string) error {
	if getenv(skipEnvVar) != "" && getenv(ciEnvVar) != "" {
		return ErrEscapeHatchInCI
	}

	return nil
}

// SkipReason returns why this harness should decline to start a container, and
// whether there is such a reason at all.
//
// This is the COMPLETE set of skip conditions, and it is exported so a test can
// prove that. The property that matters to this phase is negative — *no third
// way to skip exists* — and a negative is not provable by running the suite in a
// working environment.
//
// It matters because "the Docker daemon is unreachable" must FAIL rather than
// skip, and that path cannot be exercised on a developer host: testcontainers
// resolves Docker Desktop's named pipe directly and ignores DOCKER_HOST and
// DOCKER_CONTEXT, so the unreachable case cannot be simulated without stopping
// the daemon. Rather than claim an untested guarantee, the decision is pulled
// out here where it can be asserted exhaustively, leaving Postgres with exactly
// two branches: skip when this says so, fail otherwise.
func SkipReason(short bool, getenv func(string) string) (string, bool) {
	if short {
		return "-short was requested and this test needs a container", true
	}
	if getenv(skipEnvVar) != "" {
		return skipEnvVar + " is set", true
	}

	return "", false
}

var (
	sharedOnce sync.Once
	sharedEnv  *Env
	sharedErr  error
)

// Postgres returns the package's shared, already-migrated database.
//
// ONE container per test package, started on first use. Before this was shared,
// every call started its own, which meant roughly fourteen containers at once
// under `go test ./...` — slow, and guessed at the time to be behind a transient
// failure seen on 2026-08-29. `design.md` specified per-package from the start;
// this brings the harness back in line with it.
//
// **That guess was wrong, and the transient is now diagnosed.** It is not
// container count, it is testcontainers' provider detection losing a race when
// several test binaries open Docker Desktop's named pipe at the same instant.
// The symptom is a whole package failing at 0.00s with `rootless Docker is not
// supported on Windows, failed to create Docker provider`. Measured 2026-08-30:
// 6 of 17 parallel runs, 0 of 12 with `go test -p 1`, which is what `make
// test-api` now passes. Sharing the container is still right; it just was not
// the cure.
//
// The container is reaped by testcontainers' Ryuk sidecar when the test process
// exits. It is deliberately NOT bound to the first test that asks for it: that
// test's cleanup would tear it down while its neighbours were still using it.
//
// Tests sharing this must not destroy the schema. If yours does, use
// PostgresIsolated.
//
// It skips on -short and on the developer escape hatch. It FAILS on anything
// else, an unreachable Docker daemon included — see the package comment.
func Postgres(t *testing.T) *Env {
	t.Helper()

	// The only two branches here. Everything after this point that goes wrong
	// is a failure, never a skip.
	if reason, skip := SkipReason(testing.Short(), os.Getenv); skip {
		t.Skip("skipping: " + reason)
	}

	sharedOnce.Do(func() {
		sharedEnv, _, sharedErr = newEnv(context.Background())
	})
	if sharedErr != nil {
		t.Fatalf("%v\n\n%s", sharedErr, failureAdvice())
	}

	return sharedEnv
}

// PostgresIsolated starts a container for this test alone and tears it down when
// the test finishes.
//
// It exists for tests that destroy the schema: a `goose down-to 0` against the
// shared container would pull the floor out from under every test beside it.
// Prefer Postgres everywhere else — an isolated container costs a couple of
// seconds, and multiplied across a suite it is exactly what this harness moved
// away from.
func PostgresIsolated(t *testing.T) *Env {
	t.Helper()

	if reason, skip := SkipReason(testing.Short(), os.Getenv); skip {
		t.Skip("skipping: " + reason)
	}

	env, cleanup, err := newEnv(context.Background())
	if cleanup != nil {
		t.Cleanup(cleanup)
	}
	if err != nil {
		t.Fatalf("%v\n\n%s", err, failureAdvice())
	}

	return env
}

func failureAdvice() string {
	return fmt.Sprintf("This is a failure, not a skip. If your Docker daemon is simply not "+
		"running, start it (`make dev`) and re-run. To skip these tests locally, set %s=1 "+
		"— it is refused in CI.", skipEnvVar)
}

// newEnv starts a container, migrates it, and returns pools for the owner and
// for both application roles, each guarded.
//
// It reports an error instead of taking a *testing.T because the shared
// container outlives any single test: there is no test to fail against at the
// moment it is built.
func newEnv(ctx context.Context) (*Env, func(), error) {
	var closers []func()
	cleanup := func() {
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
	}

	container, err := postgres.Run(ctx, image,
		postgres.WithDatabase(database),
		postgres.WithUsername(ownerRole),
		postgres.WithPassword(ownerPass),
		postgres.BasicWaitStrategies(),
	)
	// Registered before the error check: Run can return a partially started
	// container alongside one, and leaking it would strand a container.
	if container != nil {
		closers = append(closers, func() { _ = testcontainers.TerminateContainer(container) })
	}
	if err != nil {
		return nil, cleanup, fmt.Errorf("starting %s: %w", image, err)
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return nil, cleanup, fmt.Errorf("building connection string: %w", err)
	}

	pool, err := openPool(ctx, dsn)
	if err != nil {
		return nil, cleanup, fmt.Errorf("owner pool: %w", err)
	}
	closers = append(closers, pool.Close)

	env := &Env{
		OwnerPool: pool,
		OwnerDSN:  dsn,
		OwnerRole: ownerRole,
		ShelterA:  uuid.New(),
		ShelterB:  uuid.New(),
		guarded:   map[string]RoleCapabilities{},
	}

	if err := migrate(ctx, env); err != nil {
		return nil, cleanup, err
	}

	for _, spec := range []struct {
		role, password string
		assign         func(*pgxpool.Pool)
	}{
		{tenantRole, tenantPass, func(p *pgxpool.Pool) { env.TenantPool = p }},
		{publicRole, publicPass, func(p *pgxpool.Pool) { env.PublicPool = p }},
		{authRole, authPass, func(p *pgxpool.Pool) { env.AuthPool = p }},
	} {
		appPool, err := openAppPool(ctx, env, spec.role, spec.password)
		if err != nil {
			return nil, cleanup, err
		}
		closers = append(closers, appPool.Close)

		caps, err := guardRole(ctx, spec.role, appPool, env.OwnerRole)
		if err != nil {
			return nil, cleanup, err
		}
		env.guarded[spec.role] = caps
		spec.assign(appPool)
	}

	return env, cleanup, nil
}

func openPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("creating pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()

		return nil, fmt.Errorf("connecting: %w", err)
	}

	return pool, nil
}

// migrate brings the container's schema up to date and sets both role
// passwords. It runs as the owner because it creates roles, grants privileges
// and installs an extension, none of which a deliberately unprivileged role can
// do.
func migrate(ctx context.Context, env *Env) error {
	handle, err := sql.Open("pgx", env.OwnerDSN)
	if err != nil {
		return fmt.Errorf("opening a database/sql handle for goose: %w", err)
	}
	defer func() { _ = handle.Close() }()

	if err := db.Up(ctx, handle); err != nil {
		return fmt.Errorf("applying migrations: %w", err)
	}

	// Passwords deliberately do not live in the migration set (D8), so the
	// harness sets them through exactly the path production uses.
	for role, password := range map[string]string{
		tenantRole: tenantPass,
		publicRole: publicPass,
		authRole:   authPass,
	} {
		if err := db.SetRolePassword(ctx, handle, role, password); err != nil {
			return fmt.Errorf("setting the password for %s: %w", role, err)
		}
	}

	return nil
}

// openAppPool builds a pool connected AS one of the application roles.
func openAppPool(ctx context.Context, env *Env, role, password string) (*pgxpool.Pool, error) {
	dsn, err := withCredentials(env.OwnerDSN, role, password)
	if err != nil {
		return nil, fmt.Errorf("building the %s connection string: %w", role, err)
	}

	pool, err := openPool(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("%s pool: %w", role, err)
	}

	return pool, nil
}

// withCredentials swaps the user and password of a connection string, keeping
// host, port, database and every parameter the container chose.
func withCredentials(dsn, user, password string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("parsing connection string: %w", err)
	}
	parsed.User = url.UserPassword(user, password)

	return parsed.String(), nil
}

// MustExec runs a statement as the owner and fails the test if it errors. It
// is for fixture setup, never for an assertion — an assertion needs to inspect
// the error, and this deliberately does not hand it back.
func MustExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()

	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", truncate(sql), err)
	}
}

func truncate(s string) string {
	const limit = 80
	if len(s) <= limit {
		return s
	}

	return fmt.Sprintf("%s...", s[:limit])
}
