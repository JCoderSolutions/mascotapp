package auth_test

import (
	"os"
	"strings"
	"testing"
)

// The one property of this package that a behavioural test CANNOT reach.
//
// Replacing `subtle.ConstantTimeCompare(got, want) == 1` with `bytes.Equal` or
// with `string(got) == string(want)` changes NOTHING observable: the same
// passwords verify, the same passwords fail, every case in password_test.go
// stays green. The difference is how long the comparison takes, and a Go test
// cannot measure that reliably enough to assert on -- a timing test on a shared
// CI runner is a flake generator, not a gate.
//
// So it is asserted structurally, by reading the source. That is not a
// workaround for a missing test; it is the correct shape for a property that
// lives in WHICH FUNCTION was called rather than in what the code returns. This
// repository already does the same thing where the property is in the text:
// `TestDevcontainerInstallsEveryToolTheMakefileInvokes` reads the Makefile, and
// `TestMigrations_ContainNoPasswordLiteral` reads the migrations.
//
// What it costs: a rename or a refactor into a helper makes this go red without
// anything being wrong. That is the right trade. A false alarm here is a
// two-minute read; the failure it prevents is a timing side channel on the one
// comparison an attacker can steer a byte at a time.
func TestVerifyPassword_ComparesInConstantTime(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("password.go")
	if err != nil {
		t.Fatalf("reading password.go: %v", err)
	}
	text := string(source)

	if !strings.Contains(text, "subtle.ConstantTimeCompare(") {
		t.Error("password.go does not call subtle.ConstantTimeCompare. The digest " +
			"comparison must be constant-time: a comparison that returns at the first " +
			"differing byte leaks how many leading bytes were correct, and a derived hash " +
			"is exactly the input an attacker can steer one byte at a time")
	}

	// The negative half. Adding the constant-time call while ALSO leaving a
	// short-circuiting comparison on the same path defeats it entirely, and the
	// positive check above would not notice.
	for _, forbidden := range []string{
		"bytes.Equal(got",
		"string(got) == string(want)",
		"string(want) == string(got)",
	} {
		if strings.Contains(text, forbidden) {
			t.Errorf("password.go contains %q. A short-circuiting comparison on the digest "+
				"defeats the constant-time one no matter what else runs alongside it",
				forbidden)
		}
	}

	// Anti-vacuity: prove this test can actually read the thing it claims to
	// inspect. Without it a renamed or moved file would make every assertion
	// above pass over an empty string.
	if !strings.Contains(text, "func VerifyPassword(") {
		t.Fatal("password.go does not define VerifyPassword, so this test read the wrong " +
			"file and its assertions above proved nothing")
	}
}
