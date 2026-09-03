package auth_test

import (
	"strings"
	"testing"

	"github.com/gentleman/mascotapp/apps/api/internal/auth"
)

// Argon2id password hashing, the credential half of every login this phase
// builds (spec: identity-and-session / *Password-based registration and login
// use Argon2id*).
//
// The encoded form is the PHC string format:
//
//	$argon2id$v=19$m=65536,t=3,p=1$<base64 salt>$<base64 hash>
//
// **The parameters live IN the encoded hash, and that is the design.** Verifying
// reads the cost parameters back out of the stored string instead of taking them
// from the constants below, so raising the cost later re-hashes new passwords
// without invalidating a single existing one. A verifier that used the current
// constants would lock the project out of ever changing them.

func TestHashPassword_VerifiesItsOwnPassword(t *testing.T) {
	t.Parallel()

	const password = "correct horse battery staple"

	encoded, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hashing: %v", err)
	}

	ok, err := auth.VerifyPassword(encoded, password)
	if err != nil {
		t.Fatalf("verifying: %v", err)
	}
	if !ok {
		t.Error("the correct password did not verify against its own hash. Nobody can log in")
	}
}

func TestVerifyPassword_RejectsAnyOtherPassword(t *testing.T) {
	t.Parallel()

	encoded, err := auth.HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hashing: %v", err)
	}

	// Near misses on purpose. A comparison that stops at the first differing
	// byte, or one that compares only a prefix, passes against an obviously
	// different password and fails against these.
	for name, wrong := range map[string]string{
		"one_character_short":     "correct horse battery stapl",
		"one_character_different": "correct horse battery staplf",
		"empty":                   "",
		"different_case":          "Correct Horse Battery Staple",
		"trailing_space":          "correct horse battery staple ",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ok, err := auth.VerifyPassword(encoded, wrong)
			if err != nil {
				t.Fatalf("verifying a wrong password returned an error rather than false: %v. "+
					"A caller that treats error and mismatch differently would leak which "+
					"one happened", err)
			}
			if ok {
				t.Errorf("%q verified against a different password's hash", wrong)
			}
		})
	}
}

// The property the whole exercise exists for. §5.2 says the system must not
// store a plaintext or reversibly-encrypted password, and the cheapest way to
// break that is a hash function that returns its input on some path.
func TestHashPassword_NeverStoresThePlaintext(t *testing.T) {
	t.Parallel()

	const password = "hunter2-with-enough-entropy-to-find"

	encoded, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hashing: %v", err)
	}

	if strings.Contains(encoded, password) {
		t.Fatal("the encoded hash CONTAINS the plaintext password")
	}
	// Substring containment alone is satisfied by an encoding that merely
	// obscures the password -- base64, hex, a rotation. Checking a long
	// substring catches the encodings that keep it recoverable in one piece.
	if half := password[:len(password)/2]; strings.Contains(encoded, half) {
		t.Errorf("the encoded hash contains %q, half of the plaintext", half)
	}
}

// A random salt per hash, asserted rather than assumed.
//
// Without it the scheme is a lookup table: two users with the same password get
// the same stored string, so one cracked hash cracks every account sharing it,
// and a leak of the table alone reveals which accounts share a password. It is
// also what an implementation that hashes with a CONSTANT salt would pass
// everything else with.
func TestHashPassword_UsesAFreshSaltEveryTime(t *testing.T) {
	t.Parallel()

	const password = "the same password twice"

	first, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hashing the first time: %v", err)
	}
	second, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hashing the second time: %v", err)
	}

	if first == second {
		t.Fatal("hashing the same password twice produced the SAME string, so the salt is " +
			"constant. Identical passwords become identical rows, and one cracked hash " +
			"cracks every account that shares it")
	}

	// Anti-vacuity: both still verify. A "salt" that was really a broken hash
	// would also differ every time and would fail here.
	for i, encoded := range []string{first, second} {
		ok, err := auth.VerifyPassword(encoded, password)
		if err != nil {
			t.Fatalf("verifying hash %d: %v", i, err)
		}
		if !ok {
			t.Errorf("hash %d does not verify its own password, so the difference above is "+
				"not a salt, it is a bug", i)
		}
	}
}

// The cost parameters, asserted so they cannot drift in silence.
//
// RFC 9106 §4's SECOND recommended option — time=3, memory=64 MiB — because the
// first (2 GiB) cannot run on the free-tier instance this project deploys to.
// Parallelism is 1 rather than the RFC's 4: that target has a single vCPU, and
// lanes beyond the available cores buy no security while costing latency. Total
// memory is `memory` regardless of lanes, so the memory-hardness argument is
// unchanged.
//
// This is a JUDGEMENT with a cost, written down rather than defaulted: 64 MiB
// per concurrent login is real on a 512 MiB instance, and the number to revisit
// if login ever contends for memory is `memory`, deliberately and with this test
// going red to prove somebody meant it.
func TestHashPassword_PinsTheCostParameters(t *testing.T) {
	t.Parallel()

	encoded, err := auth.HashPassword("any password")
	if err != nil {
		t.Fatalf("hashing: %v", err)
	}

	fields := strings.Split(encoded, "$")
	// "", "argon2id", "v=19", "m=...,t=...,p=...", salt, hash
	if len(fields) != 6 {
		t.Fatalf("the encoded hash has %d $-separated fields, want 6. It is not PHC format, "+
			"so nothing below can be read out of it: %q", len(fields), encoded)
	}

	for i, want := range map[int]string{
		1: "argon2id",
		2: "v=19",
		3: "m=65536,t=3,p=1",
	} {
		if fields[i] != want {
			t.Errorf("field %d is %q, want %q. Read the doc comment above before changing "+
				"this expectation: these parameters are a security decision with a stated "+
				"cost, not a default", i, fields[i], want)
		}
	}

	if fields[1] != "argon2id" {
		t.Errorf("the algorithm is %q. Argon2i and Argon2d are NOT interchangeable here: "+
			"§5.2 and the spec both name Argon2id", fields[1])
	}
}

// Verification must not take its parameters from today's constants.
//
// A hash produced under a DIFFERENT cost still has to verify, or the day the
// cost is raised every existing user is locked out — and the person raising it
// finds out in production. The parameters are in the string precisely so that
// cannot happen.
func TestVerifyPassword_HonoursTheParametersInTheStoredHash(t *testing.T) {
	t.Parallel()

	const password = "a password hashed under an older cost"

	// Deliberately cheaper than the current constants, standing in for a hash
	// written before the cost was raised.
	encoded, err := auth.HashPasswordWithParams(password, auth.Params{
		Memory: 8 * 1024, Time: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32,
	})
	if err != nil {
		t.Fatalf("hashing under the older cost: %v", err)
	}

	ok, err := auth.VerifyPassword(encoded, password)
	if err != nil {
		t.Fatalf("verifying a hash written under an older cost: %v", err)
	}
	if !ok {
		t.Error("a hash written under a DIFFERENT cost did not verify. Verification is " +
			"using today's constants instead of the parameters stored in the string, so " +
			"raising the cost would lock out every existing user")
	}
}

// Malformed input is a mismatch, never a panic and never a success.
//
// These strings arrive from the `users.password_hash` column. A row written by
// a different version, truncated by a migration, or simply NULL-turned-empty
// must fail closed.
func TestVerifyPassword_FailsClosedOnMalformedInput(t *testing.T) {
	t.Parallel()

	for name, encoded := range map[string]string{
		"empty":               "",
		"not_phc":             "just-some-text",
		"wrong_algorithm":     "$argon2i$v=19$m=65536,t=3,p=1$c2FsdHNhbHRzYWx0c2E$aGFzaGhhc2hoYXNoaGFzaA",
		"missing_fields":      "$argon2id$v=19$m=65536,t=3,p=1",
		"unparsable_params":   "$argon2id$v=19$m=nope,t=3,p=1$c2FsdHNhbHRzYWx0c2E$aGFzaGhhc2hoYXNoaGFzaA",
		"bad_base64_salt":     "$argon2id$v=19$m=65536,t=3,p=1$!!!not-base64!!!$aGFzaGhhc2hoYXNoaGFzaA",
		"unsupported_version": "$argon2id$v=16$m=65536,t=3,p=1$c2FsdHNhbHRzYWx0c2E$aGFzaGhhc2hoYXNoaGFzaA",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ok, err := auth.VerifyPassword(encoded, "any password")
			if ok {
				t.Fatalf("a malformed stored hash VERIFIED: %q", encoded)
			}
			if err == nil {
				t.Errorf("a malformed stored hash returned (false, nil), which is "+
					"indistinguishable from a wrong password. The caller needs to tell a "+
					"corrupt row from a failed login: %q", encoded)
			}
		})
	}
}
