package db_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/gentleman/mascotapp/apps/api/internal/db"
)

const validDSN = "postgres://user:pass@localhost:5432/mascotapp?sslmode=disable"

func TestPoolConfig_RejectsAnUnusableDSN(t *testing.T) {
	t.Parallel()

	for _, dsn := range []string{"", "   ", "not a dsn", "http://example.org"} {
		t.Run(dsn, func(t *testing.T) {
			t.Parallel()

			if _, err := db.PoolConfig(dsn); err == nil {
				t.Fatalf("accepted %q as a connection string", dsn)
			}
		})
	}
}

// The assertions below are not style preferences. Neon's free plan suspends the
// compute after 5 minutes of inactivity and CANNOT have that disabled, and
// compute time is the scarcest resource in this whole stack: 100 CU-hours a
// month, about 3.3 hours a day. A pool that keeps the compute awake burns the
// month's budget in a couple of days.
func TestPoolConfig_CannotKeepNeonsComputeAwake(t *testing.T) {
	t.Parallel()

	cfg, err := db.PoolConfig(validDSN)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// A warm minimum is the single most expensive mistake available here: it
	// pins at least one connection open forever, so the compute never suspends
	// and every one of the month's CU-hours is spent on an idle database.
	if cfg.MinConns != 0 {
		t.Errorf("MinConns = %d, want 0: a warm minimum never lets the compute suspend",
			cfg.MinConns)
	}
	if cfg.MinIdleConns != 0 {
		t.Errorf("MinIdleConns = %d, want 0, for the same reason", cfg.MinIdleConns)
	}

	// We must drop an idle connection before Neon's suspend window closes, not
	// after. pgxpool's default is 30 minutes — six times too long — which would
	// leave dead sockets in the pool for Neon to close from its side.
	if cfg.MaxConnIdleTime >= db.NeonAutoSuspendAfter {
		t.Errorf("MaxConnIdleTime = %v, must be below Neon's %v suspend window",
			cfg.MaxConnIdleTime, db.NeonAutoSuspendAfter)
	}
	if cfg.MaxConnIdleTime <= 0 {
		t.Error("MaxConnIdleTime must be positive; 0 means pgxpool's default applies")
	}

	if cfg.MaxConnLifetime <= 0 {
		t.Error("MaxConnLifetime must be positive")
	}

	// Cloud Run runs several instances, each with its own pool, against one
	// small Neon compute. Per-instance ceilings have to stay modest.
	if cfg.MaxConns <= 0 || cfg.MaxConns > 10 {
		t.Errorf("MaxConns = %d, want a small positive ceiling", cfg.MaxConns)
	}
}

func TestPoolConfig_KeepsTheCallersConnectionTarget(t *testing.T) {
	t.Parallel()

	cfg, err := db.PoolConfig(validDSN)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ConnConfig.Database != "mascotapp" {
		t.Errorf("Database = %q, want the one from the DSN", cfg.ConnConfig.Database)
	}
	if cfg.ConnConfig.User != "user" {
		t.Errorf("User = %q, want the one from the DSN", cfg.ConnConfig.User)
	}
}

// The public catalog role must not be able to write even if a query tries to.
// BEGIN READ ONLY in WithPublic is one layer; asking the server to default every
// transaction on this connection to read-only is a second one that survives a
// caller who forgets the wrapper.
func TestPublicPoolConfig_DefaultsTheSessionToReadOnly(t *testing.T) {
	t.Parallel()

	cfg, err := db.PublicPoolConfig(validDSN)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := cfg.ConnConfig.RuntimeParams["default_transaction_read_only"]
	if got != "on" {
		t.Errorf("default_transaction_read_only = %q, want \"on\"", got)
	}
}

// ---------------------------------------------------------------------------
// WithPublic
// ---------------------------------------------------------------------------

type recordingTxPool struct {
	tx       *recordingTx
	opts     []pgx.TxOptions
	beginErr error
}

func (p *recordingTxPool) BeginTx(_ context.Context, opts pgx.TxOptions) (pgx.Tx, error) {
	p.opts = append(p.opts, opts)
	if p.beginErr != nil {
		return nil, p.beginErr
	}

	return p.tx, nil
}

func newTxPool() *recordingTxPool {
	return &recordingTxPool{tx: &recordingTx{}}
}

func TestWithPublic_OpensReadOnlyAndSetsNoTenantScope(t *testing.T) {
	t.Parallel()

	pool := newTxPool()

	err := db.WithPublic(context.Background(), pool,
		func(context.Context, pgx.Tx) error { return nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pool.opts) != 1 {
		t.Fatalf("opened %d transactions, want 1", len(pool.opts))
	}
	if pool.opts[0].AccessMode != pgx.ReadOnly {
		t.Errorf("AccessMode = %v, want ReadOnly", pool.opts[0].AccessMode)
	}

	// The public catalog is not tenant-scoped: it is filtered by the
	// public_catalog policy on the app_public role. Setting app.shelter_id here
	// would quietly scope the public site to one shelter.
	for _, sql := range pool.tx.execSQL {
		if strings.Contains(sql, "app.shelter_id") {
			t.Fatalf("set a tenant scope on the public connection: %q", sql)
		}
	}
	if pool.tx.commits != 1 {
		t.Errorf("commits = %d, want 1", pool.tx.commits)
	}
}

func TestWithPublic_RollsBackWhenTheCallbackFails(t *testing.T) {
	t.Parallel()

	pool := newTxPool()
	sentinel := errors.New("boom")

	err := db.WithPublic(context.Background(), pool,
		func(context.Context, pgx.Tx) error { return sentinel })

	if !errors.Is(err, sentinel) {
		t.Fatalf("expected the callback's error, got %v", err)
	}
	if pool.tx.rollbacks != 1 {
		t.Errorf("rollbacks = %d, want 1", pool.tx.rollbacks)
	}
	if pool.tx.commits != 0 {
		t.Errorf("commits = %d, want 0", pool.tx.commits)
	}
}

func TestWithPublic_RollsBackAndRePanics(t *testing.T) {
	t.Parallel()

	pool := newTxPool()

	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic was swallowed; it must propagate after the rollback")
			}
		}()

		_ = db.WithPublic(context.Background(), pool,
			func(context.Context, pgx.Tx) error { panic("kaboom") })
	}()

	if pool.tx.rollbacks != 1 {
		t.Errorf("rollbacks = %d after a panic, want 1", pool.tx.rollbacks)
	}
}

// NeonAutoSuspendAfter is a documented external constraint, not a tuning knob.
// If someone raises it to make a test pass, this fails and says why.
func TestNeonAutoSuspendAfter_MatchesTheDocumentedWindow(t *testing.T) {
	t.Parallel()

	if db.NeonAutoSuspendAfter != 5*time.Minute {
		t.Fatalf("NeonAutoSuspendAfter = %v, but Neon's free plan documents 5 minutes "+
			"and does not allow disabling it", db.NeonAutoSuspendAfter)
	}
}
