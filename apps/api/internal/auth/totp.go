package auth

import (
	"crypto/hmac"
	"crypto/rand"
	// #nosec G505 -- RFC 6238 defines TOTP over HMAC-SHA1, and every authenticator
	// app implements exactly that. The weakness gosec flags is SHA-1 COLLISION
	// resistance, which HMAC does not rely on; HMAC-SHA1 has no practical break.
	// Choosing a "stronger" hash here would produce codes no authenticator can
	// reproduce -- interoperability is the security property that matters.
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// TOTP — the second factor §5.2 makes mandatory for `owner` and `admin`
// (design P2-D8).
//
// Implemented on the standard library rather than a dependency. RFC 6238 has
// been frozen since 2011, the algorithm is thirty lines, and Appendix B's test
// vectors pin its correctness from OUTSIDE this codebase. A library would add
// supply-chain surface to the auth package to avoid code that a published
// standard already specifies exactly.
const (
	// TOTPSecretLength is 160 bits, RFC 4226 §4's recommendation and the size
	// P2-D8 fixes. It is also the length of RFC 6238's own test key.
	TOTPSecretLength = 20

	// totpPeriod and totpDigits are the values every authenticator app assumes
	// by default. They are written into the URI explicitly anyway: an app that
	// assumed different ones would show codes that never verify, and the user
	// would have no way to tell why.
	totpPeriod = 30 * time.Second
	totpDigits = 6

	// totpSkewSteps is how many periods either side of the current one are
	// accepted. One, because real clocks drift and people type slowly.
	//
	// It is deliberately not larger. Past one step this stops being clock
	// tolerance and becomes a replay window: a code observed over someone's
	// shoulder stays usable for that much longer.
	//
	// This does NOT make a code single-use. A code presented twice inside its
	// own window is accepted twice, and refusing that needs a record of spent
	// codes -- the login handler's job, not this function's.
	totpSkewSteps = 1
)

// totpEncoding is the base32 alphabet the `otpauth://` URI uses. Unpadded,
// because the `=` characters padding adds are not what authenticator apps
// expect in a `secret` parameter.
var totpEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewTOTPSecret returns a fresh 160-bit secret from crypto/rand.
func NewTOTPSecret() ([]byte, error) {
	secret := make([]byte, TOTPSecretLength)
	if _, err := rand.Read(secret); err != nil {
		// Returned rather than swallowed. A secret that silently fell back to
		// anything predictable would produce a second factor that verifies
		// perfectly and protects nobody.
		return nil, fmt.Errorf("auth: reading a TOTP secret: %w", err)
	}

	return secret, nil
}

// TOTPURI builds the `otpauth://` URI an authenticator app scans as a QR code.
//
// Every parameter is written explicitly rather than left to the app's defaults,
// because the defaults are convention, not specification.
func TOTPURI(secret []byte, issuer, account string) (string, error) {
	if len(secret) != TOTPSecretLength {
		return "", fmt.Errorf("auth: the TOTP secret is %d bytes, want exactly %d",
			len(secret), TOTPSecretLength)
	}
	if issuer == "" || account == "" {
		return "", fmt.Errorf("auth: the TOTP URI needs both an issuer and an account "+
			"(issuer=%q account=%q); apps show the pair as the entry's name, and a user "+
			"with two accounts cannot tell unlabelled entries apart", issuer, account)
	}

	query := url.Values{}
	query.Set("secret", totpEncoding.EncodeToString(secret))
	query.Set("issuer", issuer)
	query.Set("algorithm", "SHA1")
	query.Set("digits", strconv.Itoa(totpDigits))
	query.Set("period", strconv.Itoa(int(totpPeriod/time.Second)))

	uri := url.URL{
		Scheme: "otpauth",
		Host:   "totp", // `hotp` is the counter-based algorithm; this is not it.
		// The label is `Issuer:Account`. url.URL escapes what needs escaping
		// here, which is why this is built rather than concatenated.
		Path:     "/" + issuer + ":" + account,
		RawQuery: query.Encode(),
	}

	return uri.String(), nil
}

// VerifyTOTP reports whether code is valid for secret at the instant at.
//
// It returns a bool rather than an error because there is exactly one thing the
// caller may do with the answer, and no distinction it is safe to expose: a
// caller told WHY a code failed can tell an attacker whether a secret exists,
// whether it is the right length, or how far off their clock is.
func VerifyTOTP(secret []byte, code string, at time.Time) bool {
	// The secret arrives decrypted from `users.totp_secret_enc`. A truncated or
	// empty column has to fail here rather than validate codes against a key
	// nobody chose.
	if len(secret) != TOTPSecretLength {
		return false
	}
	if !isTOTPCode(code) {
		return false
	}

	step := at.Unix() / int64(totpPeriod/time.Second)

	// Every candidate in the window is compared, and the loop does not break on
	// a match. Returning early would make the function's runtime reveal WHICH
	// step matched, which is a clock oracle.
	var matched int
	for offset := -totpSkewSteps; offset <= totpSkewSteps; offset++ {
		candidate := totpCode(secret, step+int64(offset))
		matched |= subtle.ConstantTimeCompare([]byte(candidate), []byte(code))
	}

	return matched == 1
}

// isTOTPCode reports whether code is exactly six ASCII digits.
//
// The byte range is checked rather than unicode.IsDigit on purpose. `０８１８０４`
// is six digits to Unicode and is not what any authenticator produces; treating
// it as one would mean accepting an encoding the rest of this function cannot
// reason about.
func isTOTPCode(code string) bool {
	if len(code) != totpDigits {
		return false
	}
	for i := 0; i < len(code); i++ {
		if code[i] < '0' || code[i] > '9' {
			return false
		}
	}

	return true
}

// totpCode is RFC 6238 §4 / RFC 4226 §5.3: HMAC the counter, then truncate
// dynamically.
func totpCode(secret []byte, step int64) string {
	var counter [8]byte
	// RFC 4226 defines the counter as a big-endian UNSIGNED 64-bit value, so the
	// conversion is the specification, not a shortcut.
	//
	// A pre-epoch `at` makes step negative and wraps it here, and that is safe
	// rather than merely unlikely: the wrapped counter yields a code, the code is
	// only ever COMPARED against what the user typed, and no counter -- wrapped
	// or not -- makes a wrong code match. The conversion cannot turn a rejection
	// into an acceptance, which is the only direction that would matter.
	binary.BigEndian.PutUint64(counter[:], uint64(step)) // #nosec G115

	mac := hmac.New(sha1.New, secret)
	mac.Write(counter[:])
	sum := mac.Sum(nil)

	// Dynamic truncation: the low nibble of the last byte picks the offset, and
	// the top bit of the selected word is masked off so the result is the same
	// on platforms that would read it as signed.
	offset := sum[len(sum)-1] & 0x0F
	truncated := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7FFFFFFF

	return fmt.Sprintf("%0*d", totpDigits, truncated%pow10(totpDigits))
}

// pow10 returns 10^n. Computed rather than written as a literal so that
// totpDigits stays the single place the code length is stated.
func pow10(n int) uint32 {
	result := uint32(1)
	for i := 0; i < n; i++ {
		result *= 10
	}

	return result
}
