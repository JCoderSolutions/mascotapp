package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
)

// migrationsFS embeds the migration set into the binary.
//
// Embedding rather than reading from disk is what makes the schema a property
// of the build. A Cloud Run container has no repository checkout, so a
// migration that exists only on someone's machine is a migration that never
// runs in production — and the failure would be a missing table, discovered by
// a user.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrationsDir is the path inside migrationsFS. goose is handed the subtree so
// migration filenames resolve at the root, exactly as they would on disk.
const migrationsDir = "migrations"

// Migrations returns the embedded migration set, rooted at the directory that
// holds the .sql files.
//
// It is exported so the invariant tests can assert against what actually
// shipped rather than against the working directory.
func Migrations() fs.FS {
	sub, err := fs.Sub(migrationsFS, migrationsDir)
	if err != nil {
		// Unreachable: migrationsDir is a compile-time constant and //go:embed
		// already failed the build if it did not exist.
		panic(fmt.Sprintf("db: embedded migrations are unreadable: %v", err))
	}

	return sub
}

// newProvider builds a goose provider over the embedded set.
//
// goose also exposes package-level SetBaseFS/SetDialect helpers that mutate
// global state. They are avoided here: two tests migrating two containers in
// parallel would race on that state, and this phase's suite does exactly that.
func newProvider(db *sql.DB) (*goose.Provider, error) {
	provider, err := goose.NewProvider(goose.DialectPostgres, db, Migrations())
	if err != nil {
		return nil, fmt.Errorf("db: preparing migrations: %w", err)
	}

	return provider, nil
}

// Up applies every pending migration.
//
// It must run as the owning role, not as app_tenant: it creates roles, grants
// privileges and installs an extension, none of which a deliberately
// unprivileged role can do.
func Up(ctx context.Context, db *sql.DB) error {
	provider, err := newProvider(db)
	if err != nil {
		return err
	}

	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("db: applying migrations: %w", err)
	}

	return nil
}

// DownTo rolls back to the given version, where 0 means "all the way down".
//
// This is safe to run only while the phase is unreleased. Once deployed code
// writes these tables, a down migration is a data-destroying operation, not a
// rollback.
func DownTo(ctx context.Context, db *sql.DB, version int64) error {
	provider, err := newProvider(db)
	if err != nil {
		return err
	}

	if _, err := provider.DownTo(ctx, version); err != nil {
		return fmt.Errorf("db: rolling back to version %d: %w", version, err)
	}

	return nil
}

// Version reports the schema version currently recorded in the database.
func Version(ctx context.Context, db *sql.DB) (int64, error) {
	provider, err := newProvider(db)
	if err != nil {
		return 0, err
	}

	version, err := provider.GetDBVersion(ctx)
	if err != nil {
		return 0, fmt.Errorf("db: reading schema version: %w", err)
	}

	return version, nil
}
