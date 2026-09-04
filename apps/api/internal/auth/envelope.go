package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// The at-rest format for `users.totp_secret_enc` (design P2-D8):
//
//	version(1) || key_id(1) || nonce(12) || ciphertext || tag(16)
//
// AES-256-GCM. The associated data is the two header bytes followed by the
// owner's uuid, which is what binds a blob to its row: a value copied from one
// user's column into another's fails to open rather than decrypting into a
// perfectly valid TOTP secret.
const (
	// EnvelopeVersion is the format version written into byte 0. It changes
	// only when the LAYOUT changes; a new key is a new EnvelopeKeyID, not a new
	// version.
	EnvelopeVersion byte = 1

	// EnvelopeKeyID identifies which key sealed a blob. Reserved by P2-D8 for
	// the rotation Phase 07 introduces, and written now so the column does not
	// have to be rewritten to migrate.
	//
	// It starts at 1 rather than 0 so that a zero byte -- what an uninitialised
	// buffer or a truncated write leaves behind -- is never a valid key id.
	EnvelopeKeyID byte = 1

	envelopeKeyLength    = 32 // AES-256, and nothing else
	envelopeNonceLength  = 12 // GCM standard; Go's gcm.NonceSize()
	envelopeTagLength    = 16
	envelopeHeaderLength = 2 // version + key_id

	// envelopeMinLength is a blob carrying an empty plaintext. Anything shorter
	// cannot be parsed at all, and checking it up front is what stops a
	// fixed-offset slice from panicking on a truncated column.
	envelopeMinLength = envelopeHeaderLength + envelopeNonceLength + envelopeTagLength
)

// ErrInvalidEnvelope is returned for a blob that cannot be parsed or
// authenticated.
//
// It is deliberately one sentinel for both, and that is not laziness. Telling a
// caller WHICH of the two happened tells an attacker whether their forgery got
// past the parser, and there is no legitimate caller that needs to know.
var ErrInvalidEnvelope = errors.New("auth: the stored envelope is not valid")

// Envelope seals and opens the values kept encrypted at rest.
//
// It holds the AEAD rather than the raw key so the key material is derived once
// and never handed back out. Nothing on this type returns it, and no error it
// produces mentions it -- envelope failures get logged, and a log is the one
// place the secret must never reach.
type Envelope struct {
	aead cipher.AEAD
}

// NewEnvelope builds an Envelope from a 32-byte key, which is `AUTH_KEK`
// decoded from base64.
//
// The length check is the point of this constructor. `aes.NewCipher` accepts 16,
// 24 or 32 bytes and silently selects AES-128, AES-192 or AES-256 -- so a short
// AUTH_KEK produces an envelope that works perfectly and is not the algorithm
// §5.4 specifies. After this function there is nothing left to notice.
//
// P2-D8's deliberate deviation from §5.4: that section describes a data key
// wrapped by a KEK, and for one column the indirection buys nothing today
// because both keys would live in the same environment. This uses the one key
// directly and reserves EnvelopeKeyID for the rotation Phase 07 introduces.
func NewEnvelope(key []byte) (*Envelope, error) {
	if len(key) != envelopeKeyLength {
		return nil, fmt.Errorf(
			"auth: the envelope key is %d bytes, want exactly %d. Go would accept 16 or 24 "+
				"here and quietly give you AES-128 or AES-192 instead of AES-256",
			len(key), envelopeKeyLength)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("auth: building the AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("auth: building GCM: %w", err)
	}
	if aead.NonceSize() != envelopeNonceLength {
		// Asserted rather than assumed: the layout above hardcodes 12, and a Go
		// release that changed the default would silently produce blobs nothing
		// can parse at a fixed offset.
		return nil, fmt.Errorf("auth: GCM wants a %d-byte nonce, but the envelope format "+
			"fixes %d", aead.NonceSize(), envelopeNonceLength)
	}

	return &Envelope{aead: aead}, nil
}

// Seal encrypts plaintext for owner and returns the at-rest blob.
func (e *Envelope) Seal(plaintext []byte, owner uuid.UUID) ([]byte, error) {
	nonce := make([]byte, envelopeNonceLength)
	if _, err := rand.Read(nonce); err != nil {
		// Failing the operation is the only correct outcome. Under GCM a
		// repeated (key, nonce) pair leaks the authentication subkey, so a
		// fallback nonce would be worse than no encryption at all -- it would
		// look like it worked.
		return nil, fmt.Errorf("auth: reading a nonce: %w", err)
	}

	header := [envelopeHeaderLength]byte{EnvelopeVersion, EnvelopeKeyID}

	blob := make([]byte, 0, envelopeMinLength+len(plaintext))
	blob = append(blob, header[:]...)
	blob = append(blob, nonce...)

	return e.aead.Seal(blob, nonce, plaintext, associatedData(header, owner)), nil
}

// Open decrypts a blob that was sealed for owner.
//
// It returns ErrInvalidEnvelope for anything it cannot parse OR cannot
// authenticate, which covers a truncated column, an unknown version, a blob
// belonging to a different user, and a single flipped bit anywhere in it.
func (e *Envelope) Open(blob []byte, owner uuid.UUID) ([]byte, error) {
	// Length first, before any indexing. These bytes come from a bytea column,
	// and a fixed-offset slice over a short row panics -- which takes the
	// process down instead of failing one login.
	if len(blob) < envelopeMinLength {
		return nil, fmt.Errorf("%w: %d bytes, and the shortest possible blob is %d",
			ErrInvalidEnvelope, len(blob), envelopeMinLength)
	}

	header := [envelopeHeaderLength]byte{blob[0], blob[1]}
	if header[0] != EnvelopeVersion {
		return nil, fmt.Errorf("%w: format version %d is not supported (want %d)",
			ErrInvalidEnvelope, header[0], EnvelopeVersion)
	}
	// Only one key exists today, so an unrecognised id means the blob was
	// written by something this build does not know about. WHEN ROTATION LANDS
	// this check becomes a key LOOKUP -- Phase 07 replaces it rather than
	// deletes it, because a blob whose key cannot be identified must still be
	// refused rather than tried against whatever key is at hand.
	if header[1] != EnvelopeKeyID {
		return nil, fmt.Errorf("%w: key id %d is unknown to this build (want %d)",
			ErrInvalidEnvelope, header[1], EnvelopeKeyID)
	}

	nonce := blob[envelopeHeaderLength : envelopeHeaderLength+envelopeNonceLength]
	sealed := blob[envelopeHeaderLength+envelopeNonceLength:]

	plaintext, err := e.aead.Open(nil, nonce, sealed, associatedData(header, owner))
	if err != nil {
		// The underlying error is wrapped deliberately: it is Go's constant
		// "cipher: message authentication failed" and carries no material. What
		// is NOT included, on purpose, is the blob, the owner or anything
		// derived from the key.
		return nil, fmt.Errorf("%w: %v", ErrInvalidEnvelope, err)
	}

	return plaintext, nil
}

// associatedData is what GCM authenticates alongside the ciphertext: the header
// bytes, then the owner's id.
//
// The OWNER half is P2-D8's decision — it binds a blob to its row, so a value
// copied between users fails to open instead of yielding a working second
// factor.
//
// The HEADER half is why tampering with `version` or `key_id` is caught even
// though neither is encrypted. A header outside the AAD is a header an attacker
// with write access to the column can edit freely, and version confusion is
// exactly the shape of attack a rotation byte invites.
func associatedData(header [envelopeHeaderLength]byte, owner uuid.UUID) []byte {
	aad := make([]byte, 0, envelopeHeaderLength+len(owner))
	aad = append(aad, header[:]...)

	return append(aad, owner[:]...)
}
