package auth_test

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gentleman/mascotapp/apps/api/internal/auth"
)

// Envelope encryption for `users.totp_secret_enc` (design P2-D8).
//
// The at-rest blob is exactly:
//
//	version(1) || key_id(1) || nonce(12) || ciphertext || tag(16)
//
// AES-256-GCM, with **`AAD = user.id` bytes**. That binding is the decision this
// file exists to defend: without it, a blob copied from one user's row to
// another's decrypts into a perfectly valid TOTP secret, and whoever holds the
// first user's authenticator now passes the second user's second factor. With
// it, the copy fails to open at all.
//
// Where this deliberately stops short of §5.4: that section describes a data key
// wrapped by a KEK. For one column the indirection buys nothing today — both
// keys would live in the same environment — so `AUTH_KEK` (base64, 32 bytes) is
// used as the key directly, and the `key_id` byte is reserved for the rotation
// Phase 07 introduces. The format already carries the bytes needed to migrate
// without rewriting the column.

// testKEK is a fixed 32-byte key. Fixed on purpose: these tests assert on blob
// LAYOUT, and a random key per run would make a layout failure look flaky.
func testKEK(t *testing.T) []byte {
	t.Helper()

	kek, err := base64.StdEncoding.DecodeString("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	if err != nil {
		t.Fatalf("decoding the test KEK: %v", err)
	}
	if len(kek) != 32 {
		t.Fatalf("the test KEK is %d bytes, want 32", len(kek))
	}

	return kek
}

func TestEnvelope_RoundTrips(t *testing.T) {
	t.Parallel()

	env, err := auth.NewEnvelope(testKEK(t))
	if err != nil {
		t.Fatalf("building the envelope: %v", err)
	}

	user := uuid.New()
	secret := []byte("JBSWY3DPEHPK3PXP20byte")

	blob, err := env.Seal(secret, user)
	if err != nil {
		t.Fatalf("sealing: %v", err)
	}

	// The ciphertext must not BE the plaintext. A "cipher" that returns its
	// input round-trips perfectly and protects nothing.
	if bytes.Contains(blob, secret) {
		t.Fatal("the sealed blob CONTAINS the plaintext secret")
	}

	got, err := env.Open(blob, user)
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if !bytes.Equal(got, secret) {
		t.Errorf("opened %q, want %q", got, secret)
	}
}

// THE decision of P2-D8, asserted.
//
// A blob lifted out of user A's row and written into user B's must not open.
// The AAD is what refuses it: GCM authenticates the associated data alongside
// the ciphertext, so a tag computed over A's id cannot validate against B's.
func TestEnvelope_RefusesABlobBoundToAnotherUser(t *testing.T) {
	t.Parallel()

	env, err := auth.NewEnvelope(testKEK(t))
	if err != nil {
		t.Fatalf("building the envelope: %v", err)
	}

	victim, attacker := uuid.New(), uuid.New()
	secret := []byte("the victim's TOTP secret")

	blob, err := env.Seal(secret, victim)
	if err != nil {
		t.Fatalf("sealing for the victim: %v", err)
	}

	// Anti-vacuity FIRST. Without proving the blob opens for its own owner,
	// "it does not open for the attacker" is satisfied by an Open that never
	// opens anything -- which would pass this test and break the product.
	if _, err := env.Open(blob, victim); err != nil {
		t.Fatalf("the blob does not open for its OWN user (%v), so the refusal below "+
			"proves nothing about the AAD binding", err)
	}

	if _, err := env.Open(blob, attacker); err == nil {
		t.Error("a blob sealed for one user OPENED for another. The user id is not bound " +
			"into the AAD, so copying a row between users hands over a working second " +
			"factor -- and the copy leaves no trace, because the ciphertext is unchanged")
	}
}

// The layout, byte for byte, because a later reader has to be able to migrate
// this column without a decryption oracle.
//
//	offset 0        version
//	offset 1        key_id
//	offset 2..13    nonce (12 bytes, GCM standard)
//	offset 14..n-17 ciphertext
//	offset n-16..n  tag (16 bytes)
func TestEnvelope_BlobLayoutIsExactlyAsDesigned(t *testing.T) {
	t.Parallel()

	env, err := auth.NewEnvelope(testKEK(t))
	if err != nil {
		t.Fatalf("building the envelope: %v", err)
	}

	secret := make([]byte, 20) // a 160-bit TOTP secret, P2-D8's size
	blob, err := env.Seal(secret, uuid.New())
	if err != nil {
		t.Fatalf("sealing: %v", err)
	}

	const (
		versionOffset = 0
		keyIDOffset   = 1
		nonceOffset   = 2
		nonceLength   = 12
		tagLength     = 16
	)

	wantLen := 1 + 1 + nonceLength + len(secret) + tagLength
	if len(blob) != wantLen {
		t.Fatalf("the blob is %d bytes, want %d (version 1 + key_id 1 + nonce %d + "+
			"ciphertext %d + tag %d). Nothing below can be read at a fixed offset if the "+
			"layout is not this one", len(blob), wantLen, nonceLength, len(secret), tagLength)
	}

	if blob[versionOffset] != auth.EnvelopeVersion {
		t.Errorf("version byte is %d, want %d", blob[versionOffset], auth.EnvelopeVersion)
	}
	if blob[keyIDOffset] != auth.EnvelopeKeyID {
		t.Errorf("key_id byte is %d, want %d. It is reserved for the Phase 07 rotation and "+
			"must be written, not left implicit", blob[keyIDOffset], auth.EnvelopeKeyID)
	}

	// The nonce must not be all zeroes: that is what a forgotten
	// `make([]byte, 12)` with no rand.Read looks like, and it is nonce reuse
	// with extra steps.
	nonce := blob[nonceOffset : nonceOffset+nonceLength]
	if allZero(nonce) {
		t.Error("the nonce is twelve zero bytes, so it was never filled from crypto/rand. " +
			"Under AES-GCM a repeated nonce with the same key does not merely leak the " +
			"plaintext, it leaks the authentication key")
	}
}

// The failure mode that makes AES-GCM catastrophic rather than merely broken.
//
// A repeated (key, nonce) pair under GCM leaks the XOR of the two plaintexts AND
// the authentication subkey, which lets an attacker forge tags for anything. It
// is not a weakening; it is a total loss. Sealing the same secret twice has to
// produce two different nonces.
func TestEnvelope_UsesAFreshNonceEverySeal(t *testing.T) {
	t.Parallel()

	env, err := auth.NewEnvelope(testKEK(t))
	if err != nil {
		t.Fatalf("building the envelope: %v", err)
	}

	user := uuid.New()
	secret := []byte("the same secret, sealed twice")

	first, err := env.Seal(secret, user)
	if err != nil {
		t.Fatalf("sealing the first time: %v", err)
	}
	second, err := env.Seal(secret, user)
	if err != nil {
		t.Fatalf("sealing the second time: %v", err)
	}

	const nonceOffset, nonceLength = 2, 12
	if bytes.Equal(first[nonceOffset:nonceOffset+nonceLength],
		second[nonceOffset:nonceOffset+nonceLength]) {
		t.Fatal("two seals produced the SAME nonce. Under AES-GCM a repeated nonce with " +
			"the same key leaks the authentication subkey, which lets an attacker forge " +
			"tags at will. This is a total loss, not a weakening")
	}

	// Anti-vacuity: both still open. A "nonce" that was really corruption would
	// also differ every time.
	for i, blob := range [][]byte{first, second} {
		if _, err := env.Open(blob, user); err != nil {
			t.Errorf("blob %d does not open (%v), so the difference above is not a nonce, "+
				"it is a bug", i, err)
		}
	}
}

// The tag doing its job: any edit to the blob has to be refused.
//
// Each case flips one byte in a different region, because the regions fail for
// different reasons and a test that only tampered with the ciphertext would miss
// a nonce or a header that nothing authenticates.
func TestEnvelope_RefusesATamperedBlob(t *testing.T) {
	t.Parallel()

	env, err := auth.NewEnvelope(testKEK(t))
	if err != nil {
		t.Fatalf("building the envelope: %v", err)
	}

	user := uuid.New()
	original, err := env.Seal(make([]byte, 20), user)
	if err != nil {
		t.Fatalf("sealing: %v", err)
	}

	for name, at := range map[string]int{
		"key_id":     1,
		"nonce":      5,
		"ciphertext": 20,
		"tag":        len(original) - 1,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tampered := bytes.Clone(original)
			tampered[at] ^= 0xFF

			if _, err := env.Open(tampered, user); err == nil {
				t.Errorf("a blob with byte %d flipped OPENED. Whatever that byte is, it is "+
					"not authenticated, so an attacker with write access to the column can "+
					"change it without detection", at)
			}
		})
	}
}

// Malformed input fails closed, never panics.
//
// These bytes come from a `bytea` column. A row truncated by a bad migration, a
// value written by an older format, or an empty column must all produce an error
// -- and a slice expression at a fixed offset panics on a short blob, which
// would take the process down rather than fail one login.
func TestEnvelope_FailsClosedOnMalformedBlobs(t *testing.T) {
	t.Parallel()

	env, err := auth.NewEnvelope(testKEK(t))
	if err != nil {
		t.Fatalf("building the envelope: %v", err)
	}

	user := uuid.New()
	valid, err := env.Seal(make([]byte, 20), user)
	if err != nil {
		t.Fatalf("sealing: %v", err)
	}

	unsupportedVersion := bytes.Clone(valid)
	unsupportedVersion[0] = auth.EnvelopeVersion + 1

	for name, blob := range map[string][]byte{
		"nil":                 nil,
		"empty":               {},
		"header_only":         valid[:2],
		"truncated_mid_nonce": valid[:8],
		"no_room_for_a_tag":   valid[:len(valid)-1],
		"unsupported_version": unsupportedVersion,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// A panic here fails the test rather than the process, which is the
			// point: this is exactly where a fixed-offset slice would blow up.
			got, err := env.Open(blob, user)
			if err == nil {
				t.Fatalf("a malformed blob of %d bytes OPENED, returning %d bytes",
					len(blob), len(got))
			}
		})
	}
}

// AES-256 means a 32-byte key, and nothing else may be accepted silently.
//
// Go's aes.NewCipher takes 16, 24 or 32 bytes and picks AES-128, AES-192 or
// AES-256 accordingly -- WITHOUT complaining. A 16-byte AUTH_KEK would give a
// working envelope that is quietly AES-128, and §5.4 says AES-256. The
// constructor is where that has to be caught, because after it there is nothing
// left to notice.
func TestNewEnvelope_RequiresATwelveEightBitKey(t *testing.T) {
	t.Parallel()

	for name, size := range map[string]int{
		"empty":       0,
		"aes_128":     16,
		"aes_192":     24,
		"one_short":   31,
		"one_too_far": 33,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := auth.NewEnvelope(make([]byte, size)); err == nil {
				t.Errorf("a %d-byte key was ACCEPTED. Go picks the AES variant from the key "+
					"length without complaining, so this is how a deployment ends up on "+
					"AES-128 with nothing saying so", size)
			}
		})
	}

	// Anti-vacuity: the right size is accepted. Without this a constructor that
	// rejected everything would pass every case above.
	if _, err := auth.NewEnvelope(make([]byte, 32)); err != nil {
		t.Errorf("a 32-byte key was REJECTED (%v), so the cases above prove nothing", err)
	}
}

// The error text must not leak the key or the plaintext.
//
// Envelope failures get logged, and §5.4 says logs never contain PII. A message
// that echoed the blob or the key would put the secret in the one place it was
// encrypted to stay out of.
func TestEnvelope_ErrorsLeakNeitherKeyNorPlaintext(t *testing.T) {
	t.Parallel()

	kek := testKEK(t)
	env, err := auth.NewEnvelope(kek)
	if err != nil {
		t.Fatalf("building the envelope: %v", err)
	}

	secret := []byte("SUPERSECRETTOTPVALUE")
	blob, err := env.Seal(secret, uuid.New())
	if err != nil {
		t.Fatalf("sealing: %v", err)
	}

	// Opening under the wrong user is the realistic failure that gets logged.
	_, err = env.Open(blob, uuid.New())
	if err == nil {
		t.Fatal("opening under the wrong user succeeded, so there is no error to inspect")
	}

	message := err.Error()
	for what, needle := range map[string]string{
		"the plaintext secret": string(secret),
		"the raw key":          string(kek),
	} {
		if strings.Contains(message, needle) {
			t.Errorf("the error message contains %s. Envelope failures are logged, and §5.4 "+
				"says logs never carry this: %q", what, message)
		}
	}
}

func allZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}

	return true
}
