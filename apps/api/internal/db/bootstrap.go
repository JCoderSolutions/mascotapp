package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// The two statements below set a role's password without the value ever being
// concatenated into SQL text by this program.
//
// The problem: `ALTER ROLE ... PASSWORD` is a utility statement and cannot take
// bind parameters — the same trap as `SET LOCAL`. The naive way out is
// fmt.Sprintf, which turns a secret into string concatenation and the role name
// into an injection point.
//
// So each value arrives as a real bind parameter, and the DO block reads them
// back through current_setting and lets `format` quote them server-side, where
// they are already data rather than text: `%L` for the literal, `%I` for the
// identifier.
//
// They are two separate statements because PostgreSQL's extended protocol
// carries exactly one statement per parameterised call, and they must run in
// ONE transaction because `set_config(..., true)` is transaction-local. That is
// also why they vanish at COMMIT instead of lingering on a pooled connection.
//
// What this does NOT do is hide the password from the database. With
// log_statement = 'all', PostgreSQL logs the bind parameter. This makes the
// value hard to leak by accident — into the repository, into the binary, into
// `goose status` output — not impossible for a superuser to observe.
const (
	setBootstrapRoleSQL = `SELECT set_config('app.bootstrap_role', $1, true)`

	// gosec G101 flags these two because the identifier contains "password"
	// next to a string literal. It is a name heuristic, and here it is exactly
	// backwards: these are SQL templates whose entire purpose is that no
	// credential appears in them.
	//
	// This is not "the linter is annoying". The property G101 is guessing at is
	// asserted directly, and more strictly, by
	// TestBootstrapStatements_BindTheirValuesInsteadOfConcatenating: the text
	// must contain a `$1` placeholder and must NOT contain a Go format verb. A
	// real hardcoded credential fails that test. Suppress the heuristic, keep
	// the test.
	//nolint:gosec // G101 false positive: an SQL template with a bind placeholder, asserted by test.
	setBootstrapPasswordSQL = `SELECT set_config('app.bootstrap_password', $1, true)`

	//nolint:gosec // G101 false positive: a DO block that reads values via current_setting, asserted by test.
	applyRolePasswordSQL = `
DO $$
BEGIN
    EXECUTE format(
        'ALTER ROLE %I PASSWORD %L',
        current_setting('app.bootstrap_role'),
        current_setting('app.bootstrap_password')
    );
END
$$`
)

// roleNamePattern is deliberately strict. The name reaches `format('%I')` as
// data, so quoting is already handled server-side; this rejects anything that is
// not one of our own roles long before that, because a bootstrap willing to
// accept an arbitrary identifier is one that eventually gets pointed at
// `postgres`.
var roleNamePattern = regexp.MustCompile(`^app_[a-z][a-z0-9_]{0,40}$`)

var (
	// ErrInvalidRoleName is returned for a name this application does not own.
	ErrInvalidRoleName = errors.New("db: role name must match app_<name>")

	// ErrEmptyPassword is returned rather than setting an empty password, which
	// PostgreSQL accepts and which leaves the role usable with no credential.
	ErrEmptyPassword = errors.New("db: refusing to set an empty role password")
)

// ValidateRoleCredentials reports whether a role name and password may be used
// for bootstrapping.
//
// It is exported and separate from SetRolePassword so the rules can be asserted
// without a database. The rules are the security-relevant part; the transaction
// around them is plumbing.
func ValidateRoleCredentials(role, password string) error {
	if !roleNamePattern.MatchString(role) {
		return fmt.Errorf("%w: got %q", ErrInvalidRoleName, role)
	}
	if strings.TrimSpace(password) == "" {
		return fmt.Errorf("%w for %q", ErrEmptyPassword, role)
	}

	return nil
}

// SetRolePassword sets the password for one application role.
//
// It runs OUTSIDE goose, as the owning role, and is never part of a migration:
// design decision D8 keeps every secret out of the migration set, which is
// committed to the repository and embedded in the binary.
//
// It opens its own transaction rather than trusting the caller to supply one.
// The two statements share a transaction-local setting, so a caller who passed a
// pooled *sql.DB would have the second statement land on a different connection
// and read an unset value — a failure that depends on pool timing, which is the
// worst kind to leave to a documented convention.
func SetRolePassword(ctx context.Context, db *sql.DB, role, password string) error {
	if err := ValidateRoleCredentials(role, password); err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db: beginning bootstrap transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, setBootstrapRoleSQL, role); err != nil {
		return fmt.Errorf("db: naming the bootstrap role: %w", err)
	}
	// The password travels as $1. It is never concatenated into a statement,
	// and bootstrap_test.go asserts that about the statement text itself.
	if _, err := tx.ExecContext(ctx, setBootstrapPasswordSQL, password); err != nil {
		return fmt.Errorf("db: staging the password for %q: %w", role, err)
	}
	if _, err := tx.ExecContext(ctx, applyRolePasswordSQL); err != nil {
		return fmt.Errorf("db: setting the password for %q: %w", role, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db: committing the password for %q: %w", role, err)
	}

	return nil
}
