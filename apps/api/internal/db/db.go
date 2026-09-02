package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NeonAutoSuspendAfter is how long Neon's free plan waits before suspending the
// compute. It is a documented external constraint, not a knob: the free plan
// does not allow disabling Scale to Zero.
//
// It matters here because compute time is the scarcest resource in this stack.
// The free plan grants 100 CU-hours a month — about 3.3 hours a day — so a pool
// that keeps the database awake spends the entire monthly budget on an idle
// database within a couple of days.
const NeonAutoSuspendAfter = 5 * time.Minute

const (
	// maxConns is per process. Cloud Run runs several instances against one
	// small Neon compute, so the ceiling that matters is instances x maxConns,
	// not this number alone.
	maxConns = 4

	// maxConnIdleTime must stay below NeonAutoSuspendAfter so we close an idle
	// connection before Neon closes it from its side. pgxpool's default is 30
	// minutes, six times too long, which would leave sockets in the pool that
	// Neon has already dropped.
	//
	// This does NOT keep the compute awake: the pool never pings. pgxpool's
	// background health check only inspects local state — idle duration and
	// expiry — and destroys connections locally, with no network round trip.
	// (Verified in pgxpool's checkConnsHealth, v5.10.0.) A pool that pings on an
	// interval is exactly what Neon's own guides warn defeats autosuspend.
	maxConnIdleTime = 2 * time.Minute

	// maxConnLifetime recycles connections well inside Neon's own recycling, so
	// a long-lived socket never becomes the thing that breaks.
	maxConnLifetime = 30 * time.Minute
)

// PoolConfig builds the tenant pool's configuration from a connection string.
//
// It is exported, and separate from NewPool, because these values are a
// cost-and-correctness contract with Neon rather than tuning preferences, and a
// contract nobody can assert is one that quietly drifts. Everything decided here
// is unit-testable without a database.
func PoolConfig(dsn string) (*pgxpool.Config, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("db: a connection string is required")
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("db: parsing connection string: %w", err)
	}

	cfg.MaxConns = maxConns
	cfg.MaxConnIdleTime = maxConnIdleTime
	cfg.MaxConnLifetime = maxConnLifetime

	// Set explicitly even though both already default to zero. A future change
	// that raises either one to "warm up" the pool would keep the compute from
	// ever suspending, and the tests next to these lines say so out loud.
	cfg.MinConns = 0
	cfg.MinIdleConns = 0

	// The tenant scope is deliberately NOT set in AfterConnect or BeforeAcquire.
	// pgxpool reuses physical connections, so anything set at connection scope
	// outlives the request that set it and the next acquirer inherits it. Scope
	// belongs inside the transaction — see WithTenant.

	return cfg, nil
}

// PublicPoolConfig builds the read-only public catalog pool's configuration.
//
// It asks the server to default every transaction on the connection to
// read-only. WithPublic already opens BEGIN READ ONLY; this is the second layer,
// and the one that still holds if a caller reaches the pool without the wrapper.
// Neither replaces the app_public role's grants, which are the actual authority.
func PublicPoolConfig(dsn string) (*pgxpool.Config, error) {
	cfg, err := PoolConfig(dsn)
	if err != nil {
		return nil, err
	}

	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"

	return cfg, nil
}

// NewPool opens the tenant pool. Reaching it directly is not how tenant data is
// read: use WithTenant, which is the only path that sets the scope every policy
// in this schema depends on.
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := PoolConfig(dsn)
	if err != nil {
		return nil, err
	}

	return newPool(ctx, cfg)
}

// NewPublicPool opens the read-only pool used by the public catalog, connecting
// as app_public. It is a separate pool and a separate role on purpose: the
// public site must not be one forgotten WHERE away from tenant data.
func NewPublicPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := PublicPoolConfig(dsn)
	if err != nil {
		return nil, err
	}

	return newPool(ctx, cfg)
}

func newPool(ctx context.Context, cfg *pgxpool.Config) (*pgxpool.Pool, error) {
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("db: creating pool: %w", err)
	}

	// NewWithConfig is lazy, so an unreachable or misconfigured database would
	// otherwise surface at the first request instead of at startup.
	if err := pool.Ping(ctx); err != nil {
		pool.Close()

		return nil, fmt.Errorf("db: connecting to %s: %w", cfg.ConnConfig.Database, err)
	}

	return pool, nil
}
