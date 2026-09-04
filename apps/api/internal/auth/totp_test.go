package auth_test

import (
	"encoding/base32"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gentleman/mascotapp/apps/api/internal/auth"
)

// TOTP — the second factor §5.2 makes mandatory for `owner` and `admin`
// (design P2-D8).
//
// The secret is 20 bytes from `crypto/rand` (160 bits, RFC 4226's
// recommendation), base32 for the `otpauth://` URI an authenticator app scans.

// rfcSecret is RFC 6238 Appendix B's test key: the ASCII string
// "12345678901234567890", 20 bytes, which is exactly the length P2-D8 specifies.
var rfcSecret = []byte("12345678901234567890")

// THE case that separates "this is TOTP" from "this round-trips with itself".
//
// Every other test here could pass against a private scheme that generates and
// verifies its own codes perfectly and shares nothing with the authenticator app
// in the user's pocket. These vectors are from RFC 6238 Appendix B and they are
// what proves interoperability: an app implementing the RFC produces these exact
// digits for this exact key at these exact instants.
//
// The expected values were COMPUTED, not recalled — with crypto/hmac and
// crypto/sha1 from the standard library, independently of whatever the
// implementation ends up using. The RFC publishes the 8-digit forms
// (94287082, 07081804, 14050471, 89005924, 69279037); a 6-digit code is that
// value mod 10^6, which is the same truncation the RFC's own algorithm applies.
func TestVerifyTOTP_MatchesRFC6238TestVectors(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		unix int64
		code string
	}{
		"T=59":         {59, "287082"},
		"T=1111111109": {1111111109, "081804"},
		"T=1111111111": {1111111111, "050471"},
		"T=1234567890": {1234567890, "005924"},
		"T=2000000000": {2000000000, "279037"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			at := time.Unix(tc.unix, 0).UTC()
			if !auth.VerifyTOTP(rfcSecret, tc.code, at) {
				t.Errorf("the RFC 6238 vector %s did not verify at %s. This implementation "+
					"does not agree with the authenticator app in the user's pocket, and no "+
					"amount of self-consistency fixes that", tc.code, at)
			}
		})
	}
}

// The secret: 160 bits, and a different 160 bits every time.
//
// The length is RFC 4226's recommendation and P2-D8 states it. The FRESHNESS is
// what a constant-secret implementation would fail and everything else would
// miss — and two users sharing a TOTP secret means either one's authenticator
// passes the other's second factor.
func TestNewTOTPSecret_Is160FreshBits(t *testing.T) {
	t.Parallel()

	const wantBytes = 20 // 160 bits

	first, err := auth.NewTOTPSecret()
	if err != nil {
		t.Fatalf("generating the first secret: %v", err)
	}
	if len(first) != wantBytes {
		t.Errorf("the secret is %d bytes, want %d (160 bits, RFC 4226)", len(first), wantBytes)
	}

	second, err := auth.NewTOTPSecret()
	if err != nil {
		t.Fatalf("generating the second secret: %v", err)
	}

	if string(first) == string(second) {
		t.Fatal("two calls produced the SAME secret, so it does not come from crypto/rand. " +
			"Every user would share one second factor, and holding any one authenticator " +
			"would pass all of them")
	}

	// A "random" secret of twenty zero bytes differs from nothing and would slip
	// past the comparison above only if the second call differed -- but a
	// half-filled buffer is a real failure mode worth naming.
	if allZero(first) {
		t.Error("the secret is twenty zero bytes, so the buffer was never filled")
	}
}

// The URI an authenticator app scans. Its shape is a contract with software this
// project does not control, so every part of it is asserted rather than eyeballed.
func TestTOTPURI_IsAScannableOtpauthURI(t *testing.T) {
	t.Parallel()

	const (
		issuer  = "MascotApp"
		account = "persona@example.test"
	)

	raw, err := auth.TOTPURI(rfcSecret, issuer, account)
	if err != nil {
		t.Fatalf("building the URI: %v", err)
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("the URI does not parse: %v (%q)", err, raw)
	}

	if parsed.Scheme != "otpauth" {
		t.Errorf("scheme is %q, want %q", parsed.Scheme, "otpauth")
	}
	if parsed.Host != "totp" {
		t.Errorf("host is %q, want %q -- `hotp` is a different algorithm", parsed.Host, "totp")
	}

	// The label is `Issuer:Account`. Apps show it, and an app that cannot tell
	// two accounts apart is an app the user cannot use.
	wantLabel := "/" + issuer + ":" + account
	if parsed.Path != wantLabel {
		t.Errorf("label is %q, want %q", parsed.Path, wantLabel)
	}

	query := parsed.Query()
	for key, want := range map[string]string{
		"secret":    base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(rfcSecret),
		"issuer":    issuer,
		"algorithm": "SHA1",
		"digits":    "6",
		"period":    "30",
	} {
		if got := query.Get(key); got != want {
			t.Errorf("query %s is %q, want %q. These are not defaults to leave implicit: "+
				"an app that assumes a different algorithm or period shows codes that never "+
				"verify, and the user has no way to tell why", key, got, want)
		}
	}

	// The secret must never appear unencoded in the URI -- it is base32 there,
	// and a raw copy would mean the encoder was skipped.
	if strings.Contains(raw, string(rfcSecret)) {
		t.Error("the URI contains the raw secret bytes rather than their base32 encoding")
	}
}

// The window, and where it stops.
//
// A code is valid for its 30-second step. Real clocks drift and users type
// slowly, so ONE step either side is accepted too — the policy every
// authenticator assumes. Two steps is refused: past that, tolerance stops being
// clock skew and starts being a replay window.
//
// This does NOT make a code single-use. A code accepted twice inside its window
// is still accepted twice; refusing that needs a used-code record, which is the
// login handler's job and not this function's.
func TestVerifyTOTP_AcceptsOneStepOfSkewAndNoMore(t *testing.T) {
	t.Parallel()

	// The vector at T=1111111109 is "081804"; the steps either side of it are
	// what this case walks.
	const (
		base = int64(1111111109)
		code = "081804"
		step = 30 * time.Second
	)
	at := time.Unix(base, 0).UTC()

	for name, tc := range map[string]struct {
		offset time.Duration
		want   bool
	}{
		"same_step":       {0, true},
		"one_step_early":  {-step, true},
		"one_step_late":   {step, true},
		"two_steps_early": {-2 * step, false},
		"two_steps_late":  {2 * step, false},
		"an_hour_late":    {time.Hour, false},
		"a_day_early":     {-24 * time.Hour, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := auth.VerifyTOTP(rfcSecret, code, at.Add(tc.offset))
			if got != tc.want {
				if tc.want {
					t.Errorf("a code %v from its own step was REJECTED. One step of skew has "+
						"to be tolerated or ordinary clock drift locks people out", tc.offset)
				} else {
					t.Errorf("a code %v from its own step was ACCEPTED. Past one step this "+
						"stops being clock tolerance and becomes a replay window", tc.offset)
				}
			}
		})
	}
}

// A code proves possession of ONE secret, not of any secret.
func TestVerifyTOTP_RejectsACodeFromAnotherSecret(t *testing.T) {
	t.Parallel()

	other, err := auth.NewTOTPSecret()
	if err != nil {
		t.Fatalf("generating the other secret: %v", err)
	}

	at := time.Unix(1111111109, 0).UTC()

	// Anti-vacuity first: the code is genuinely valid for its own secret. Without
	// this, a VerifyTOTP that always returned false would pass.
	if !auth.VerifyTOTP(rfcSecret, "081804", at) {
		t.Fatal("the RFC vector does not verify against its own secret, so the rejection " +
			"below proves nothing")
	}

	if auth.VerifyTOTP(other, "081804", at) {
		t.Error("a code generated for one secret verified against a DIFFERENT one, so the " +
			"code proves nothing about which authenticator produced it")
	}
}

// Whatever arrives from the request body must be refused, not crashed on.
//
// The code is user input: an empty field, a pasted word, a code with spaces, or
// forty digits. None of it may panic, and none of it may verify.
func TestVerifyTOTP_RejectsMalformedCodes(t *testing.T) {
	t.Parallel()

	at := time.Unix(1111111109, 0).UTC()

	for name, code := range map[string]string{
		"empty":          "",
		"too_short":      "0818",
		"too_long":       "0818040",
		"not_numeric":    "abcdef",
		"mixed":          "08a804",
		"with_space":     "081 804",
		"leading_plus":   "+81804",
		"unicode_digits": "０８１８０４",
		"very_long":      strings.Repeat("0", 4096),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if auth.VerifyTOTP(rfcSecret, code, at) {
				t.Errorf("the malformed code %q VERIFIED", code)
			}
		})
	}
}

// A secret of the wrong size is a configuration error, not a code that happens
// not to match.
//
// It matters because the caller is handing over bytes decrypted from
// `users.totp_secret_enc`. A truncated column must not produce a verifier that
// quietly accepts codes derived from a shorter key.
func TestVerifyTOTP_RefusesASecretOfTheWrongSize(t *testing.T) {
	t.Parallel()

	at := time.Unix(1111111109, 0).UTC()

	for name, secret := range map[string][]byte{
		"nil":       nil,
		"empty":     {},
		"truncated": rfcSecret[:10],
		"padded":    append(append([]byte{}, rfcSecret...), 0),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if auth.VerifyTOTP(secret, "081804", at) {
				t.Errorf("a %d-byte secret was used to verify a code. The bytes come from a "+
					"decrypted column, and a short one has to fail loudly rather than "+
					"validate against a key nobody chose", len(secret))
			}
		})
	}
}
