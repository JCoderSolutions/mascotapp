// Package auth holds the cryptographic primitives behind identity: password
// hashing, TOTP, envelope encryption, tokens and sessions.
//
// Everything here is pure Go with no I/O. Storage belongs to internal/db and
// transport to internal/httpapi, so these functions can be exercised without a
// container and, more importantly, so a security property can be asserted
// without a database standing between the test and the thing it is testing.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Params are Argon2id's cost parameters plus the two lengths that decide the
// shape of the stored string.
type Params struct {
	// Memory is the memory cost in KiB. It dominates the security argument:
	// an attacker's advantage comes from parallel hardware, and memory is what
	// that hardware cannot cheaply multiply.
	Memory uint32

	// Time is the number of passes over that memory.
	Time uint32

	// Parallelism is the number of lanes. It does NOT change how much memory
	// the hash costs in total, only how that work is spread across cores.
	Parallelism uint8

	// SaltLength in bytes. 16 is the PHC/RFC 9106 recommendation.
	SaltLength uint32

	// KeyLength in bytes, the length of the derived hash.
	KeyLength uint32
}

// DefaultParams is what new passwords are hashed with.
//
// RFC 9106 §4's SECOND recommended option -- time=3, memory=64 MiB. The first
// (time=1, memory=2 GiB) is the stronger choice and cannot run on the free-tier
// instance this project deploys to, so it was not available to pick.
//
// Parallelism is 1 rather than the RFC's 4, and that is a deliberate deviation:
// the target runs on a single vCPU, and lanes beyond the available cores buy no
// security while adding latency. The memory-hardness argument is untouched --
// `Memory` is the total regardless of how many lanes divide it.
//
// THE COST, stated rather than discovered later: 64 MiB is allocated for the
// duration of every password hash, on an instance with 512 MiB. Concurrent
// logins contend for it. If that ever becomes the problem, `Memory` is the knob
// and TestHashPassword_PinsTheCostParameters is what makes lowering it a
// decision somebody had to make on purpose.
var DefaultParams = Params{
	Memory:      64 * 1024,
	Time:        3,
	Parallelism: 1,
	SaltLength:  16,
	KeyLength:   32,
}

// ErrInvalidHash is returned when a stored hash cannot be parsed.
//
// It is deliberately distinct from a wrong password, which is (false, nil). A
// caller has to tell "this row is corrupt" from "this person typed the wrong
// password": the first is an incident, the second is Tuesday.
var ErrInvalidHash = errors.New("auth: the stored password hash is not valid PHC-format Argon2id")

// argon2idVersion is the only Argon2 version this package accepts.
//
// A hash carrying any other version is refused rather than verified against
// today's algorithm: the derivation genuinely differs between versions, so
// "close enough" here means silently comparing against the wrong bytes.
const argon2idVersion = argon2.Version // 0x13 == 19

// HashPassword hashes password with DefaultParams and returns its PHC encoding.
func HashPassword(password string) (string, error) {
	return HashPasswordWithParams(password, DefaultParams)
}

// HashPasswordWithParams hashes password with explicit cost parameters.
//
// It is exported for two reasons and neither is configurability at call sites:
// a test needs to produce a hash under a cost other than today's, and a future
// cost change needs somewhere to stand while old hashes are re-derived on
// login. Production code calls HashPassword.
func HashPasswordWithParams(password string, p Params) (string, error) {
	if p.SaltLength == 0 || p.KeyLength == 0 || p.Memory == 0 || p.Time == 0 || p.Parallelism == 0 {
		return "", fmt.Errorf("auth: refusing to hash with a zero cost parameter: %+v", p)
	}

	salt := make([]byte, p.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		// Not recoverable and not something to paper over: without a random
		// salt this is a lookup table, so failing the login is the correct
		// outcome.
		return "", fmt.Errorf("auth: reading a random salt: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, p.Time, p.Memory, p.Parallelism, p.KeyLength)

	// The parameters travel WITH the hash. Verification reads them back out of
	// this string rather than from DefaultParams, which is what lets the cost
	// be raised later without invalidating a single existing password.
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2idVersion, p.Memory, p.Time, p.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword reports whether password matches the stored PHC-format hash.
//
// Three outcomes, and keeping them apart is the point:
//
//	(true,  nil)  the password matches
//	(false, nil)  it does not -- an ordinary failed login
//	(false, err)  the stored hash could not be parsed at all
func VerifyPassword(encoded, password string) (bool, error) {
	p, salt, want, err := parse(encoded)
	if err != nil {
		return false, err
	}

	got := argon2.IDKey([]byte(password), salt, p.Time, p.Memory, p.Parallelism, p.KeyLength)

	// subtle.ConstantTimeCompare, never bytes.Equal and never ==. Those return
	// as soon as two bytes differ, so how long the comparison took reveals how
	// many leading bytes were right -- and a derived hash is something an
	// attacker can steer one byte at a time.
	//
	// It also returns 0 rather than panicking on a length mismatch, which is
	// what a hash whose KeyLength does not match its own digest produces.
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// parse reads a PHC-format Argon2id string back into its parts.
//
// Every failure path returns ErrInvalidHash wrapped with what specifically was
// wrong. The wrapping is for the operator reading a log; the sentinel is for the
// caller deciding whether this is an incident.
func parse(encoded string) (Params, []byte, []byte, error) {
	fields := strings.Split(encoded, "$")
	if len(fields) != 6 || fields[0] != "" {
		return Params{}, nil, nil, fmt.Errorf("%w: expected 6 $-separated fields, got %d",
			ErrInvalidHash, len(fields))
	}
	if fields[1] != "argon2id" {
		return Params{}, nil, nil, fmt.Errorf("%w: algorithm is %q, not argon2id",
			ErrInvalidHash, fields[1])
	}

	var version int
	if _, err := fmt.Sscanf(fields[2], "v=%d", &version); err != nil {
		return Params{}, nil, nil, fmt.Errorf("%w: unreadable version %q", ErrInvalidHash, fields[2])
	}
	if version != argon2idVersion {
		return Params{}, nil, nil, fmt.Errorf("%w: version %d is not supported (want %d)",
			ErrInvalidHash, version, argon2idVersion)
	}

	var p Params
	if _, err := fmt.Sscanf(fields[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Time, &p.Parallelism); err != nil {
		return Params{}, nil, nil, fmt.Errorf("%w: unreadable parameters %q", ErrInvalidHash, fields[3])
	}

	salt, err := base64.RawStdEncoding.DecodeString(fields[4])
	if err != nil {
		return Params{}, nil, nil, fmt.Errorf("%w: undecodable salt: %v", ErrInvalidHash, err)
	}
	want, err := base64.RawStdEncoding.DecodeString(fields[5])
	if err != nil {
		return Params{}, nil, nil, fmt.Errorf("%w: undecodable hash: %v", ErrInvalidHash, err)
	}
	// KeyLength and SaltLength come from the components that are actually
	// stored, not from DefaultParams. Deriving a different length would produce
	// a value that can never match, and the failure would read as a wrong
	// password rather than as the corrupt row it is.
	saltLen, ok := componentLength(len(salt))
	if !ok {
		return Params{}, nil, nil, fmt.Errorf("%w: salt is %d bytes", ErrInvalidHash, len(salt))
	}
	keyLen, ok := componentLength(len(want))
	if !ok {
		return Params{}, nil, nil, fmt.Errorf("%w: hash is %d bytes", ErrInvalidHash, len(want))
	}
	p.SaltLength, p.KeyLength = saltLen, keyLen

	if p.Memory == 0 || p.Time == 0 || p.Parallelism == 0 {
		return Params{}, nil, nil, fmt.Errorf("%w: zero cost parameter in %q",
			ErrInvalidHash, fields[3])
	}

	return p, salt, want, nil
}

// componentLength bounds a decoded salt or digest length and narrows it to the
// uint32 argon2 takes.
//
// The upper bound is not linter appeasement. These bytes come from
// `users.password_hash`, so their length is whatever is in that column, and
// argon2 hashes over a salt of exactly that size: a row carrying a
// megabyte-long salt would make one login attempt allocate and churn through
// it. Refusing it as malformed is both correct and free.
//
// Bounding here is also what makes the conversion provably safe rather than
// merely unlikely -- `len()` is an int, and this is a 64-bit build.
func componentLength(n int) (uint32, bool) {
	const maxComponent = 1024
	if n <= 0 || n > maxComponent {
		return 0, false
	}

	return uint32(n), true
}
