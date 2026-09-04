package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// The access token — identity-and-session's *JWT access tokens are short-lived
// and carry the tenant claim*, and design P2-D11.
//
// HS256 over `JWT_SECRET`. Rejected by P2-D11: EdDSA — asymmetric signing pays
// for itself when a SECOND service verifies without being able to mint, and
// there is one service. That second verifier is the design's own reopening
// condition, written down so this does not get re-argued every quarter.
//
// This file uses `golang-jwt/v5` rather than the standard library, and the
// contrast with `totp.go` next door is deliberate. TOTP met three conditions
// for writing it here: a frozen standard, published external test vectors, and
// a dependency landing where a compromise is unrecoverable. JOSE fails all
// three — it has algorithm families, known confusion modes (`alg: none`,
// HS/RS substitution) and an ACTIVE history of implementation
// vulnerabilities. Parsing attacker-supplied tokens is exactly the work a
// maintained library should be doing.
const (
	// AccessTokenLifetime is the fifteen minutes identity-and-session fixes.
	//
	// It is short because it is the window in which a stolen token is useful,
	// and nothing revokes it: there is no per-request lookup to consult. The
	// refresh token underneath is what carries revocation, which is why the
	// access token can afford to be a pure bearer credential and why this
	// number must not drift upward for convenience.
	AccessTokenLifetime = 15 * time.Minute

	// MinJWTSecretLength is the floor `JWT_SECRET` must clear.
	//
	// HS256's security is bounded by the secret's entropy. A short one is brute
	// forced offline from a single captured token, and the prize is minting
	// tokens for any user, any shelter and any role -- so this is refused at
	// construction, where it cannot be missed, rather than at the first request
	// where it would be a log line.
	MinJWTSecretLength = 32

	// accessTokenAlgorithm is fixed at build time, not read from the token.
	//
	// A token's `alg` header is attacker-controlled input. A verifier that
	// dispatches on it hands the attacker the choice of verification algorithm,
	// which is how `alg: none` and HS/RS confusion both work.
	accessTokenAlgorithm = "HS256"
)

// ErrInvalidToken is returned for any token that does not verify.
//
// One sentinel covers a bad signature, an expired token, a wrong issuer or
// audience, an unexpected algorithm and an unparseable string. That is
// deliberate: telling a caller WHICH check failed tells an attacker how close
// their forgery got, and no legitimate caller changes its behaviour based on
// the distinction -- every one of these is a 401.
var ErrInvalidToken = errors.New("auth: the access token is not valid")

// AccessClaims is what a verified token asserts.
//
// It is a separate type from the wire format below on purpose: callers work
// with `uuid.UUID`, and the JSON shape is a contract with anything that reads
// these tokens. Letting one leak into the other means a rename in Go silently
// changes the wire.
type AccessClaims struct {
	// Subject is the user the request is attributed to.
	Subject uuid.UUID

	// ShelterID is the tenant this token is scoped to, and NIL when the user
	// has not chosen one yet.
	//
	// It is a pointer rather than a plain uuid because "no shelter" has to be
	// distinguishable from a shelter: `uuid.Nil` is a perfectly non-nil value
	// that would pass a middleware's presence check and land in `WithTenant`.
	// This claim is the SOLE input to tenant resolution (§5.3, ADR-0008), so
	// the distinction is the tenant boundary itself.
	ShelterID *uuid.UUID

	// Role is the membership role RBAC evaluates.
	Role string

	// AMR is the set of authentication methods actually USED, so a route can
	// demand a second factor was exercised rather than merely enrolled.
	AMR []string
}

// accessTokenClaims is the wire format, written out field by field.
//
// `jwt.RegisteredClaims` is deliberately not embedded. Its `aud` is a
// `ClaimStrings`, which serialises a single audience as a one-element ARRAY by
// default -- a legal JWT, but a different document from the one this project
// issues. Spelling the claims out means the bytes on the wire are chosen here
// rather than inherited from a library default that a minor release could
// reasonably change.
type accessTokenClaims struct {
	Issuer    string   `json:"iss"`
	Audience  string   `json:"aud"`
	Subject   string   `json:"sub"`
	IssuedAt  int64    `json:"iat"`
	ExpiresAt int64    `json:"exp"`
	ShelterID *string  `json:"shelter_id,omitempty"`
	Role      string   `json:"role"`
	AMR       []string `json:"amr"`
}

// The six accessors below satisfy `jwt.Claims` so the library's validator can
// check expiry, issuer and audience against this hand-written shape.

func (c accessTokenClaims) GetExpirationTime() (*jwt.NumericDate, error) {
	return jwt.NewNumericDate(time.Unix(c.ExpiresAt, 0)), nil
}

func (c accessTokenClaims) GetIssuedAt() (*jwt.NumericDate, error) {
	return jwt.NewNumericDate(time.Unix(c.IssuedAt, 0)), nil
}

// GetNotBefore returns nil: these tokens are valid from `iat`, and a separate
// `nbf` would only be another clock to disagree with.
func (c accessTokenClaims) GetNotBefore() (*jwt.NumericDate, error) { return nil, nil }

func (c accessTokenClaims) GetIssuer() (string, error) { return c.Issuer, nil }

func (c accessTokenClaims) GetSubject() (string, error) { return c.Subject, nil }

func (c accessTokenClaims) GetAudience() (jwt.ClaimStrings, error) {
	return jwt.ClaimStrings{c.Audience}, nil
}

// TokenIssuer mints and verifies access tokens.
//
// It holds the secret, and nothing on this type hands it back out: no method
// returns it and no error it produces mentions it. Token failures get logged,
// and a log is the one place the signing key must never reach.
type TokenIssuer struct {
	secret   []byte
	issuer   string
	audience string
}

// NewTokenIssuer builds an issuer from `JWT_SECRET` and the identifiers that go
// into every token's `iss` and `aud`.
//
// All three checks are here rather than at first use because all three are
// configuration errors: they are true or false at boot, they do not depend on
// the request, and a process that started with a bad one will keep serving
// until someone reads a log.
func NewTokenIssuer(secret []byte, issuer, audience string) (*TokenIssuer, error) {
	if len(secret) < MinJWTSecretLength {
		return nil, fmt.Errorf(
			"auth: JWT_SECRET is %d bytes, want at least %d. HS256 with a short secret is "+
				"brute forced offline from a single captured token, and the result is the "+
				"ability to mint tokens for any user, any shelter and any role",
			len(secret), MinJWTSecretLength)
	}
	if issuer == "" {
		return nil, errors.New("auth: the token issuer (`iss`) is empty; a token whose " +
			"provenance cannot be checked is a token any service can mint")
	}
	if audience == "" {
		return nil, errors.New("auth: the token audience (`aud`) is empty; without it a " +
			"token minted for one service is accepted by another")
	}

	return &TokenIssuer{
		// Copied, not aliased. The caller's slice is theirs to reuse or zero,
		// and a signing key that changes underneath this struct would produce
		// tokens nothing can verify.
		secret:   append([]byte(nil), secret...),
		issuer:   issuer,
		audience: audience,
	}, nil
}

// Issue mints a token for claims, valid for AccessTokenLifetime from now.
//
// `now` is a parameter rather than a call to `time.Now` so that expiry is
// testable without sleeping, and so that the one clock reading a request uses
// is the caller's, not this function's.
func (i *TokenIssuer) Issue(claims AccessClaims, now time.Time) (string, error) {
	if err := claims.validate(); err != nil {
		return "", err
	}

	wire := accessTokenClaims{
		Issuer:    i.issuer,
		Audience:  i.audience,
		Subject:   claims.Subject.String(),
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(AccessTokenLifetime).Unix(),
		Role:      claims.Role,
		AMR:       claims.AMR,
	}
	if claims.ShelterID != nil {
		scoped := claims.ShelterID.String()
		wire.ShelterID = &scoped
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, wire).SignedString(i.secret)
	if err != nil {
		return "", fmt.Errorf("auth: signing the access token: %w", err)
	}

	return signed, nil
}

// Verify parses and validates a token, returning what it asserts.
//
// Every failure is ErrInvalidToken. The parser is built per call because it
// carries `now`, and building one is cheap next to the HMAC it wraps.
func (i *TokenIssuer) Verify(raw string, now time.Time) (AccessClaims, error) {
	parser := jwt.NewParser(
		// The whole defence against algorithm confusion, in one option: the
		// accepted algorithm is fixed here, so the token's own `alg` header is
		// checked AGAINST this list rather than used to choose a verifier.
		// Without it, `alg: none` and an HS512 signature computed with the same
		// secret both verify.
		jwt.WithValidMethods([]string{accessTokenAlgorithm}),
		jwt.WithIssuer(i.issuer),
		jwt.WithAudience(i.audience),
		// A token with no `exp` is a token that never expires. Required, not
		// merely validated when present.
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithTimeFunc(func() time.Time { return now }),
	)

	var wire accessTokenClaims
	if _, err := parser.ParseWithClaims(raw, &wire, func(*jwt.Token) (any, error) {
		return i.secret, nil
	}); err != nil {
		// The underlying error is wrapped for a developer reading a log, and
		// the sentinel is what callers match on. What is NOT included, ever, is
		// the secret or anything derived from it.
		return AccessClaims{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	subject, err := uuid.Parse(wire.Subject)
	if err != nil {
		return AccessClaims{}, fmt.Errorf("%w: `sub` is not a uuid", ErrInvalidToken)
	}

	claims := AccessClaims{Subject: subject, Role: wire.Role, AMR: wire.AMR}
	if wire.ShelterID != nil {
		shelter, err := uuid.Parse(*wire.ShelterID)
		if err != nil {
			return AccessClaims{}, fmt.Errorf("%w: `shelter_id` is not a uuid", ErrInvalidToken)
		}
		claims.ShelterID = &shelter
	}

	// Re-validated on the way OUT, not only on the way in. A signature proves a
	// token came from this issuer; it does not prove this issuer was correct
	// when it minted it. A token signed by an older build, or by a code path
	// that skipped `Issue`, still has to identify somebody before a handler
	// treats it as a user.
	if err := claims.validate(); err != nil {
		return AccessClaims{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	return claims, nil
}

// validate refuses claims that identify nobody.
//
// `uuid.Nil` and the empty string are the values a partially-populated struct
// carries, and neither names a real user or role. A token minted from them is a
// credential for an account that does not exist -- and the audit trail it
// leaves blames the zero uuid for whatever happens next.
func (c AccessClaims) validate() error {
	if c.Subject == uuid.Nil {
		return errors.New("auth: the token subject is the zero uuid, which is not a user")
	}
	if c.Role == "" {
		return errors.New("auth: the token role is empty, and RBAC reads this claim")
	}
	if len(c.AMR) == 0 {
		// Empty `amr` does not mean "no second factor" -- it means nothing was
		// recorded, and a route asking whether one was used cannot tell those
		// apart. P2-D11 gives this claim a job; an empty one cannot do it.
		return errors.New("auth: the token records no authentication method (`amr` is empty)")
	}

	return nil
}
