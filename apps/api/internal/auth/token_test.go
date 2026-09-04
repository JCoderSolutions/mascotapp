package auth_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gentleman/mascotapp/apps/api/internal/auth"
)

// The access token — identity-and-session's *JWT access tokens are short-lived
// and carry the tenant claim*, and design P2-D11.
//
// HS256 with `JWT_SECRET`, fifteen minutes, and a `shelter_id` claim that is
// the SOLE input to `WithTenant`. The router never reads a shelter identifier
// from a URL or a header (§5.3, ADR-0008), which makes this claim the whole
// tenant boundary: a token carrying the wrong one is a cross-tenant read that
// every layer below will happily serve.
//
// WHY THIS FILE DECODES TOKENS BY HAND.
//
// The implementation uses `golang-jwt/v5`, and that is correct. P2-D11 stands:
// unlike RFC 6238 (see the TOTP deviation), JOSE has algorithm families, known
// confusion modes and an ACTIVE history of implementation vulnerabilities. The
// library exists to defend against exactly those.
//
// But a test that parses with the same library the implementation writes with
// proves a ROUND TRIP, not correctness — it would pass just as happily if both
// sides agreed on something that is not a JWT. So the assertions below split
// the token on `.`, base64url-decode it, and recompute the MAC with
// `crypto/hmac`, checking the bytes on the wire rather than the library's own
// account of them. This is not hand-rolling crypto; it is refusing to let the
// library grade its own homework.

const (
	testIssuer   = "https://api.mascotapp.test"
	testAudience = "mascotapp-web"

	// accessTokenLifetime is the fifteen minutes the requirement fixes. It is
	// written here rather than imported from the package under test, so that
	// changing the constant in `token.go` has to be a deliberate act in two
	// places, one of which is derived from the spec.
	accessTokenLifetime = 15 * time.Minute
)

// testSecret is a synthetic 32-byte secret. No real `JWT_SECRET` is needed and
// none is read: this file never touches the environment.
var testSecret = []byte("0123456789abcdef0123456789abcdef")

func newTestIssuer(t *testing.T) *auth.TokenIssuer {
	t.Helper()

	issuer, err := auth.NewTokenIssuer(testSecret, testIssuer, testAudience)
	if err != nil {
		t.Fatalf("building the token issuer: %v", err)
	}

	return issuer
}

func testClaims() auth.AccessClaims {
	shelter := uuid.MustParse("11111111-1111-4111-8111-111111111111")

	return auth.AccessClaims{
		Subject:   uuid.MustParse("22222222-2222-4222-8222-222222222222"),
		ShelterID: &shelter,
		Role:      "owner",
		AMR:       []string{"pwd", "otp"},
	}
}

// The secret is a boot-time refusal, not a runtime one.
//
// HS256's security is bounded by the secret's entropy: a short one is brute
// forced offline, from a single captured token, and the prize is the ability to
// MINT tokens for any user, shelter and role. That is the whole system, so the
// refusal belongs at construction where it is unmissable, not at the first
// request where it is a log line nobody reads.
func TestNewTokenIssuer_RefusesASecretUnder32Bytes(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		secret  []byte
		wantErr bool
	}{
		"nil":                {nil, true},
		"empty":              {[]byte{}, true},
		"one_byte":           {[]byte("x"), true},
		"thirty_one_bytes":   {make([]byte, 31), true},
		"exactly_thirty_two": {make([]byte, 32), false},
		"sixty_four":         {make([]byte, 64), false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := auth.NewTokenIssuer(tc.secret, testIssuer, testAudience)
			if tc.wantErr && err == nil {
				t.Errorf("a %d-byte secret was ACCEPTED. HS256 with a short secret is brute "+
					"forced offline from one captured token, and the prize is minting tokens "+
					"for any user, any shelter and any role", len(tc.secret))
			}
			if !tc.wantErr && err != nil {
				t.Errorf("a %d-byte secret was refused: %v", len(tc.secret), err)
			}
		})
	}
}

// An issuer also has to know who it is and who the token is for. An empty `iss`
// or `aud` produces a token whose provenance no verifier can check.
func TestNewTokenIssuer_RefusesAnEmptyIssuerOrAudience(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct{ issuer, audience string }{
		"no_issuer":   {"", testAudience},
		"no_audience": {testIssuer, ""},
		"neither":     {"", ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := auth.NewTokenIssuer(testSecret, tc.issuer, tc.audience); err == nil {
				t.Errorf("issuer=%q audience=%q was accepted; a token with an empty `iss` or "+
					"`aud` cannot have its provenance checked by any verifier",
					tc.issuer, tc.audience)
			}
		})
	}
}

// THE case: what actually goes out on the wire.
//
// Every claim P2-D11 names is asserted against a payload this test decoded
// itself, and the signature is recomputed from scratch. If `token.go` and
// `golang-jwt` ever agree on something that is not a JWT, this is the test that
// notices.
func TestIssue_ProducesAJWTAnIndependentVerifierAccepts(t *testing.T) {
	t.Parallel()

	issued := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	claims := testClaims()

	raw, err := newTestIssuer(t).Issue(claims, issued)
	if err != nil {
		t.Fatalf("issuing: %v", err)
	}

	header, payload, signingInput, signature := decodeJWT(t, raw)

	// The header. `alg` is the field an attacker edits first, so it is pinned
	// exactly rather than merely checked for presence.
	if got := header["alg"]; got != "HS256" {
		t.Errorf("header alg is %v, want HS256 (P2-D11)", got)
	}
	if got := header["typ"]; got != "JWT" {
		t.Errorf("header typ is %v, want JWT", got)
	}

	// The signature, recomputed independently of whatever produced it.
	mac := hmac.New(sha256.New, testSecret)
	mac.Write([]byte(signingInput))
	if !hmac.Equal(mac.Sum(nil), signature) {
		t.Fatal("the signature does not verify against an independently computed " +
			"HMAC-SHA256 of the signing input. Whatever this token is, it is not one a " +
			"standards-conforming verifier will accept")
	}

	// The claims.
	if got := payload["iss"]; got != testIssuer {
		t.Errorf("iss is %v, want %q", got, testIssuer)
	}
	if got := payload["aud"]; got != testAudience {
		t.Errorf("aud is %v, want %q", got, testAudience)
	}
	if got := payload["sub"]; got != claims.Subject.String() {
		t.Errorf("sub is %v, want %q", got, claims.Subject)
	}
	if got := payload["shelter_id"]; got != claims.ShelterID.String() {
		t.Errorf("shelter_id is %v, want %q. This claim is the SOLE input to WithTenant, so "+
			"a wrong one is a cross-tenant read the database will serve without complaint",
			got, claims.ShelterID)
	}
	if got := payload["role"]; got != claims.Role {
		t.Errorf("role is %v, want %q", got, claims.Role)
	}

	// `amr` exists so a route can demand a second factor was actually USED, not
	// merely enrolled (P2-D11). A route that cannot tell those apart cannot
	// enforce step-up authentication at all.
	amr, ok := payload["amr"].([]any)
	if !ok {
		t.Fatalf("amr is %T, want a JSON array", payload["amr"])
	}
	if len(amr) != 2 || amr[0] != "pwd" || amr[1] != "otp" {
		t.Errorf("amr is %v, want [pwd otp]", amr)
	}

	// The clock claims, and the lifetime between them.
	iat, expiry := numericDate(t, payload, "iat"), numericDate(t, payload, "exp")
	if !iat.Equal(issued) {
		t.Errorf("iat is %s, want %s", iat, issued)
	}
	if want := issued.Add(accessTokenLifetime); !expiry.Equal(want) {
		t.Errorf("exp is %s, want %s (%s after iat)", expiry, want, accessTokenLifetime)
	}
}

// A user who has not chosen a shelter yet gets a token with no `shelter_id`,
// and that has to be DISTINGUISHABLE from one scoped to a shelter.
//
// The claim must be absent or null, never the zero uuid. P2-D11 has the tenant
// middleware require a non-nil `shelter_id` for a tenant-scoped route, and
// `00000000-0000-0000-0000-000000000000` is a perfectly non-nil value — it
// would sail past that check and land in `WithTenant`.
func TestIssue_OmitsShelterIDWhenTheTokenIsNotScopedToAShelter(t *testing.T) {
	t.Parallel()

	claims := testClaims()
	claims.ShelterID = nil

	raw, err := newTestIssuer(t).Issue(claims, time.Now())
	if err != nil {
		t.Fatalf("issuing: %v", err)
	}

	_, payload, _, _ := decodeJWT(t, raw)

	if got, present := payload["shelter_id"]; present && got != nil {
		t.Errorf("shelter_id is %v on an unscoped token, want absent or null. The zero uuid "+
			"is not nil, so it passes the middleware's non-nil check and reaches WithTenant",
			got)
	}
}

// The requirement's two paired scenarios, kept in one test because they ARE a
// pair: either one alone is satisfied by a verifier that always answers the
// same way.
func TestVerify_RejectsATokenIssued16MinutesAgoAndAcceptsOneIssued1MinuteAgo(t *testing.T) {
	t.Parallel()

	issuer := newTestIssuer(t)
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

	for name, tc := range map[string]struct {
		age  time.Duration
		want bool
	}{
		"issued_1_minute_ago":   {time.Minute, true},
		"issued_16_minutes_ago": {16 * time.Minute, false},

		// The boundary. RFC 7519 §4.1.4: the current time MUST be BEFORE `exp`,
		// so at exactly `exp` the token is already dead. `<=` versus `<` is the
		// classic off-by-one here and it is worth two lines to pin it.
		"one_second_before_expiry": {accessTokenLifetime - time.Second, true},
		"exactly_at_expiry":        {accessTokenLifetime, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			raw, err := issuer.Issue(testClaims(), now.Add(-tc.age))
			if err != nil {
				t.Fatalf("issuing: %v", err)
			}

			got, err := issuer.Verify(raw, now)
			switch {
			case tc.want && err != nil:
				t.Fatalf("a token issued %s ago was REJECTED: %v", tc.age, err)
			case tc.want && got.Subject != testClaims().Subject:
				t.Errorf("sub round-tripped as %s, want %s", got.Subject, testClaims().Subject)
			case !tc.want && err == nil:
				t.Errorf("a token issued %s ago was ACCEPTED. Fifteen minutes is the entire "+
					"reason the refresh token exists; an access token that outlives its "+
					"window makes the rotation underneath it decorative", tc.age)
			}
		})
	}
}

// `alg: none` — the oldest JWT attack there is, and the single best reason this
// package uses a library instead of a hand-rolled verifier.
//
// The token below is well-formed, its claims are entirely plausible, and it has
// NO signature. A verifier that trusts the header's `alg` accepts it and hands
// the caller a fully populated set of claims for whatever user, shelter and
// role the attacker typed in.
func TestVerify_RefusesTheNoneAlgorithm(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	forged := craftToken(t,
		map[string]any{"alg": "none", "typ": "JWT"},
		map[string]any{
			"iss": testIssuer,
			"aud": testAudience,
			"sub": "22222222-2222-4222-8222-222222222222",

			"role":       "owner",
			"shelter_id": "11111111-1111-4111-8111-111111111111",
			"iat":        now.Unix(),
			"exp":        now.Add(accessTokenLifetime).Unix(),
		},
		nil, // no signature at all
	)

	if _, err := newTestIssuer(t).Verify(forged, now); err == nil {
		t.Fatal("an `alg: none` token was ACCEPTED. Anyone who can reach the API can now " +
			"mint a token for any user, any shelter and any role, and the signature check " +
			"they bypassed was the only thing that was ever stopping them")
	}
}

// Algorithm substitution: the same secret, a DIFFERENT algorithm.
//
// The token below carries a real, correct HMAC — computed with SHA-512 instead
// of SHA-256. A verifier that reads `alg` from the header and dispatches on it
// accepts this happily. `alg` is attacker-controlled input, so the only safe
// reading is the one fixed at build time: HS256 or nothing.
func TestVerify_RefusesAnAlgorithmOtherThanHS256(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	header := map[string]any{"alg": "HS512", "typ": "JWT"}
	payload := map[string]any{
		"iss":  testIssuer,
		"aud":  testAudience,
		"sub":  "22222222-2222-4222-8222-222222222222",
		"role": "owner",
		"iat":  now.Unix(),
		"exp":  now.Add(accessTokenLifetime).Unix(),
	}

	signingInput := encodeSegment(t, header) + "." + encodeSegment(t, payload)
	mac := hmac.New(sha512.New, testSecret)
	mac.Write([]byte(signingInput))
	forged := signingInput + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if _, err := newTestIssuer(t).Verify(forged, now); err == nil {
		t.Fatal("an HS512 token was ACCEPTED. The header's `alg` is attacker-controlled, so " +
			"dispatching on it hands the attacker the choice of verification algorithm")
	}
}

// A signature from the wrong key, and a claim edited after signing.
func TestVerify_RefusesAForeignSignatureOrAnEditedClaim(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	issuer := newTestIssuer(t)

	honest, err := issuer.Issue(testClaims(), now)
	if err != nil {
		t.Fatalf("issuing: %v", err)
	}

	// Anti-vacuity first: the honest token verifies. Without this, a Verify
	// that always errored would pass both subtests below.
	if _, err := issuer.Verify(honest, now); err != nil {
		t.Fatalf("the honest token does not verify (%v), so the refusals below prove nothing",
			err)
	}

	t.Run("signed_with_another_secret", func(t *testing.T) {
		t.Parallel()

		other, err := auth.NewTokenIssuer(
			[]byte("ffffffffffffffffffffffffffffffff"), testIssuer, testAudience)
		if err != nil {
			t.Fatalf("building the other issuer: %v", err)
		}
		foreign, err := other.Issue(testClaims(), now)
		if err != nil {
			t.Fatalf("issuing with the other secret: %v", err)
		}

		if _, err := issuer.Verify(foreign, now); err == nil {
			t.Error("a token signed with a DIFFERENT secret was accepted, so the signature " +
				"is not being checked against this issuer's key at all")
		}
	})

	t.Run("role_escalated_after_signing", func(t *testing.T) {
		t.Parallel()

		header, payload, _, signature := decodeJWT(t, honest)
		payload["role"] = "owner_but_edited"

		tampered := encodeSegment(t, header) + "." + encodeSegment(t, payload) + "." +
			base64.RawURLEncoding.EncodeToString(signature)

		if _, err := issuer.Verify(tampered, now); err == nil {
			t.Error("a token whose `role` claim was rewritten after signing was accepted. " +
				"The signature covers the payload precisely so that this cannot happen")
		}
	})
}

// Whatever arrives in the `Authorization` header has to be refused, not crashed
// on. None of this may panic and none of it may verify.
func TestVerify_RejectsMalformedTokens(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	issuer := newTestIssuer(t)

	for name, raw := range map[string]string{
		"empty":          "",
		"not_a_token":    "hello",
		"two_segments":   "aaaa.bbbb",
		"four_segments":  "aaaa.bbbb.cccc.dddd",
		"empty_segments": "..",
		"not_base64":     "!!!.???.***",
		"header_not_json": base64.RawURLEncoding.EncodeToString([]byte("nope")) +
			".e30.x",

		// The caller is expected to strip `Bearer `. If it does not, the token
		// must still be refused rather than parsed leniently.
		"bearer_prefix_kept": "Bearer aaaa.bbbb.cccc",
		"very_long":          strings.Repeat("a", 8192),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := issuer.Verify(raw, now); err == nil {
				t.Errorf("the malformed token %.40q VERIFIED", raw)
			}
		})
	}
}

// --- helpers: an independent JWT reader, built on nothing but the stdlib ---

// decodeJWT splits a compact JWS and returns its header, payload, signing input
// and raw signature bytes. It verifies nothing: verification is the assertion's
// job, not the helper's.
func decodeJWT(t *testing.T, raw string) (
	header, payload map[string]any, signingInput string, signature []byte,
) {
	t.Helper()

	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		t.Fatalf("the token has %d segments, want 3: %.60q", len(parts), raw)
	}

	header = decodeSegment(t, parts[0], "header")
	payload = decodeSegment(t, parts[1], "payload")

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("the signature segment is not base64url: %v", err)
	}

	return header, payload, parts[0] + "." + parts[1], signature
}

func decodeSegment(t *testing.T, segment, what string) map[string]any {
	t.Helper()

	// RawURLEncoding, not StdEncoding: JWS segments are base64URL and UNPADDED
	// (RFC 7515 §2). A decoder that accepts `+`, `/` or `=` here is accepting
	// something that is not a JWS segment.
	decoded, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		t.Fatalf("the %s segment is not unpadded base64url: %v", what, err)
	}

	var out map[string]any
	if err := json.Unmarshal(decoded, &out); err != nil {
		t.Fatalf("the %s segment is not a JSON object: %v (%s)", what, err, decoded)
	}

	return out
}

func encodeSegment(t *testing.T, value map[string]any) string {
	t.Helper()

	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encoding a segment: %v", err)
	}

	return base64.RawURLEncoding.EncodeToString(encoded)
}

// craftToken builds a compact JWS from parts the test chose, including ones no
// honest issuer would ever produce. A nil signature yields the trailing empty
// segment an `alg: none` token carries.
func craftToken(t *testing.T, header, payload map[string]any, signature []byte) string {
	t.Helper()

	return encodeSegment(t, header) + "." + encodeSegment(t, payload) + "." +
		base64.RawURLEncoding.EncodeToString(signature)
}

// numericDate reads a JWT NumericDate claim, which JSON hands back as float64.
func numericDate(t *testing.T, payload map[string]any, claim string) time.Time {
	t.Helper()

	seconds, ok := payload[claim].(float64)
	if !ok {
		t.Fatalf("claim %s is %T (%v), want a JSON number of seconds since the epoch",
			claim, payload[claim], payload[claim])
	}

	return time.Unix(int64(seconds), 0).UTC()
}
