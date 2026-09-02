package db

import (
	"strings"
	"testing"
)

// These assertions are about the statement TEXT, which is why they live inside
// the package. Design decision D8's whole claim is that no secret is ever
// concatenated into SQL, and the only way to check that without a database is
// to look at the strings the program is built from.

func TestBootstrapStatements_BindTheirValuesInsteadOfConcatenating(t *testing.T) {
	t.Parallel()

	for name, sql := range map[string]string{
		"role":     setBootstrapRoleSQL,
		"password": setBootstrapPasswordSQL,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if !strings.Contains(sql, "$1") {
				t.Errorf("no bind placeholder: %q", sql)
			}
			if strings.Contains(sql, "%s") || strings.Contains(sql, "%v") ||
				strings.Contains(sql, "%q") {
				t.Errorf("carries a Go format verb, so the value would be "+
					"concatenated rather than bound: %q", sql)
			}
			if !strings.Contains(sql, "set_config") {
				t.Errorf("expected set_config, got %q", sql)
			}
			// Transaction-local. With `false` the value would outlive the
			// transaction and sit on a pooled connection for whoever acquires
			// it next.
			if !strings.HasSuffix(strings.TrimSpace(sql), ", true)") {
				t.Errorf("value is not transaction-local: %q", sql)
			}
		})
	}
}

func TestApplyRolePasswordSQL_QuotesServerSideAndTakesNoParameters(t *testing.T) {
	t.Parallel()

	// %I and %L are PostgreSQL's own identifier and literal quoting, applied to
	// values that are already data by the time format sees them.
	if !strings.Contains(applyRolePasswordSQL, "%I") {
		t.Error("the role name is not quoted with format('%I')")
	}
	if !strings.Contains(applyRolePasswordSQL, "%L") {
		t.Error("the password is not quoted with format('%L')")
	}
	if !strings.Contains(applyRolePasswordSQL, "current_setting") {
		t.Error("values are not read back through current_setting")
	}

	// A DO block cannot take bind parameters. If a placeholder ever appears
	// here, somebody has tried to parameterise a statement that cannot be
	// parameterised, and the next step is always concatenation.
	if strings.Contains(applyRolePasswordSQL, "$1") {
		t.Error("a DO block cannot take bind parameters, yet this one has $1")
	}

	// PostgreSQL's extended protocol carries one statement per parameterised
	// call. Each of these constants must therefore be a single statement.
	for name, sql := range map[string]string{
		"role":     setBootstrapRoleSQL,
		"password": setBootstrapPasswordSQL,
	} {
		if strings.Contains(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(sql), ";")), ";") {
			t.Errorf("%s statement contains more than one statement; the extended "+
				"protocol would reject it: %q", name, sql)
		}
	}
}
