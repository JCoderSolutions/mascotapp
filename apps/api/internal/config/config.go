// Package config loads application configuration from environment
// variables, applying defaults and validating values before the rest of
// the application starts.
package config

import (
	"encoding/base64"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"

	"golang.org/x/net/publicsuffix"
)

const (
	defaultPort     = 8080
	defaultEnv      = "development"
	defaultLogLevel = "info"

	minPort = 1
	maxPort = 65535

	// minJWTSecretBytes is the HS256 key floor: a key shorter than the hash
	// output is one an attacker brute-forces rather than steals.
	minJWTSecretBytes = 32

	// authKEKBytes is what AES-256-GCM requires, exactly. Not a minimum: a
	// longer value is as wrong as a shorter one, because it means whoever
	// generated it produced something other than a 256-bit key.
	authKEKBytes = 32
)

var validEnvs = map[string]bool{
	"development": true,
	"staging":     true,
	"production":  true,
}

// Config holds the application's runtime configuration, sourced from
// environment variables with sane defaults applied.
type Config struct {
	Port        int
	Env         string
	LogLevel    string
	DatabaseURL string

	// TrustedProxies lists the address ranges of the infrastructure that
	// fronts this service, used to decide whether a request's
	// X-Forwarded-For chain may be believed at all. It is empty unless
	// TRUSTED_PROXIES is set, which makes client IP resolution fall back to
	// the raw peer address — non-forgeable, but the proxy rather than the
	// client. Populate it at deploy time with the real front-end ranges.
	TrustedProxies []netip.Prefix

	// DatabaseURLAuth is the connection string for the `app_auth` role, a
	// separate login from the tenant role on purpose (P2-D3): its grants are
	// per column, so the authentication door can read a password hash without
	// being able to read a full name or a phone number.
	DatabaseURLAuth string

	// JWTSecret signs access tokens. Held as the raw string rather than
	// decoded bytes because that is what the issuer takes.
	JWTSecret string

	// AuthKEK is the key-encryption key that wraps the per-record data keys
	// for encrypted PII. It holds the DECODED bytes: the environment carries
	// base64 because a 256-bit key is not printable, and the decode belongs
	// here rather than at every use site.
	AuthKEK []byte

	// WebOrigins is the CORS allowlist, handed verbatim to
	// httpapi.NewOriginAllowlist, which is where each origin's own shape is
	// validated. Entries are trimmed and never rewritten: normalising one
	// would invent a match the browser never sent.
	WebOrigins []string

	// APIPublicOrigin is where this API answers a browser. It is in the
	// configuration for one reason beyond documentation: paired with
	// WebOrigins it decides whether P2-D9's revisit condition has been met.
	APIPublicOrigin string
}

// Load reads configuration from the process environment, applying defaults
// for unset values and validating the result. It returns a descriptive
// error when a value is present but invalid.
func Load() (Config, error) {
	cfg := Config{
		Port:        defaultPort,
		Env:         defaultEnv,
		LogLevel:    defaultLogLevel,
		DatabaseURL: os.Getenv("DATABASE_URL"),
	}

	if raw := os.Getenv("PORT"); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, fmt.Errorf("config: invalid PORT %q: must be a number", raw)
		}
		cfg.Port = port
	}
	if cfg.Port < minPort || cfg.Port > maxPort {
		return Config{}, fmt.Errorf("config: invalid PORT %d: must be between %d and %d", cfg.Port, minPort, maxPort)
	}

	if raw := os.Getenv("ENV"); raw != "" {
		cfg.Env = raw
	}
	if !validEnvs[cfg.Env] {
		return Config{}, fmt.Errorf("config: invalid ENV %q: must be one of development|staging|production", cfg.Env)
	}

	if raw := os.Getenv("LOG_LEVEL"); raw != "" {
		cfg.LogLevel = raw
	}

	if raw := os.Getenv("TRUSTED_PROXIES"); raw != "" {
		proxies, err := parseTrustedProxies(raw)
		if err != nil {
			return Config{}, err
		}
		cfg.TrustedProxies = proxies
	}

	cfg.DatabaseURLAuth = os.Getenv("DATABASE_URL_AUTH")
	cfg.APIPublicOrigin = strings.TrimSpace(os.Getenv("API_PUBLIC_ORIGIN"))

	// The auth values are validated when PRESENT, not required. An unset
	// JWT_SECRET is a missing secret rather than a weak one, and it is refused
	// where it is consumed; demanding it here would stop every command that
	// loads configuration without serving HTTP, the migration runner included.
	if raw := os.Getenv("JWT_SECRET"); raw != "" {
		if len(raw) < minJWTSecretBytes {
			return Config{}, fmt.Errorf("config: JWT_SECRET is %d bytes: must be at least %d, because a key shorter than the HS256 hash output is brute-forceable", len(raw), minJWTSecretBytes)
		}
		cfg.JWTSecret = raw
	}

	if raw := os.Getenv("AUTH_KEK"); raw != "" {
		kek, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return Config{}, fmt.Errorf("config: invalid AUTH_KEK: must be standard base64: %w", err)
		}
		// Measured after decoding, never before. Thirty-two base64 characters
		// decode to twenty-four bytes, so a length check on the raw text
		// accepts a value AES-256-GCM has to reject.
		if len(kek) != authKEKBytes {
			return Config{}, fmt.Errorf("config: AUTH_KEK decodes to %d bytes: must be exactly %d for AES-256-GCM", len(kek), authKEKBytes)
		}
		cfg.AuthKEK = kek
	}

	if raw := os.Getenv("WEB_ORIGINS"); raw != "" {
		cfg.WebOrigins = parseWebOrigins(raw)
	}

	if err := checkSameSiteRevisitCondition(cfg.WebOrigins, cfg.APIPublicOrigin); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// parseWebOrigins splits the comma-separated allowlist, trimming each entry and
// dropping empty ones so a trailing separator is not an origin. It validates
// nothing else: httpapi.NewOriginAllowlist owns the rules about what an origin
// may look like, and duplicating them here would give the project two answers
// that drift apart.
func parseWebOrigins(raw string) []string {
	var origins []string
	for _, entry := range strings.Split(raw, ",") {
		if entry = strings.TrimSpace(entry); entry != "" {
			origins = append(origins, entry)
		}
	}
	return origins
}

// checkSameSiteRevisitCondition enforces the condition P2-D9 attached to its own
// decision. The refresh cookie is fixed at SameSite=None because the web app and
// the API live on separate origins by decision; the day they land under one
// registrable domain that justification is gone, and the server has to say so on
// boot rather than wait for someone to remember a comment.
//
// It needs both sides to compare, so an unset one is not a failure: there is
// nothing to compare and nothing to refuse. But an origin whose registrable
// domain cannot be COMPUTED is refused rather than skipped. Skipping would turn
// the check off for that value while the config still boots reporting nothing —
// an exemption nobody verifies, drifting in the permissive direction.
func checkSameSiteRevisitCondition(webOrigins []string, apiOrigin string) error {
	if len(webOrigins) == 0 || apiOrigin == "" {
		return nil
	}

	apiDomain, err := registrableDomain(apiOrigin)
	if err != nil {
		return fmt.Errorf("config: cannot evaluate API_PUBLIC_ORIGIN: %w", err)
	}

	for _, origin := range webOrigins {
		webDomain, err := registrableDomain(origin)
		if err != nil {
			return fmt.Errorf("config: cannot evaluate WEB_ORIGINS entry %q: %w", origin, err)
		}
		if webDomain == apiDomain {
			return fmt.Errorf(
				"config: %q and %q share the registrable domain %q, so the refresh cookie no longer needs SameSite=None: revisit P2-D9 and ADR-0004 before starting",
				origin, apiOrigin, webDomain,
			)
		}
	}

	return nil
}

// registrableDomain returns the domain someone can actually register for an
// origin — `mascotapp.com.mx` for `https://api.mascotapp.com.mx`. The public
// suffix list is what makes that correct: a comparison of the last two labels
// reads `com.mx` as registrable, and reads two distinct `pages.dev` tenants as
// one owner. Both shapes are this project's real deployment.
func registrableDomain(origin string) (string, error) {
	parsed, err := url.Parse(origin)
	if err != nil {
		return "", fmt.Errorf("%q is not a URL: %w", origin, err)
	}
	host := parsed.Hostname()
	if host == "" {
		return "", fmt.Errorf("%q has no host", origin)
	}

	domain, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		return "", fmt.Errorf("%q has no registrable domain: %w", host, err)
	}
	return domain, nil
}

// parseTrustedProxies reads a comma-separated list of CIDR blocks. A bare
// address is accepted as a shorthand for a single-host prefix, and prefixes are
// canonicalised so that a sloppy value such as 10.1.2.3/8 is stored as the
// range it actually denotes. An unparseable entry is an error rather than a
// silent skip: quietly dropping one would shrink the trusted set and change
// which forwarded chains are believed.
func parseTrustedProxies(raw string) ([]netip.Prefix, error) {
	var proxies []netip.Prefix

	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		if prefix, err := netip.ParsePrefix(entry); err == nil {
			proxies = append(proxies, prefix.Masked())
			continue
		}

		addr, err := netip.ParseAddr(entry)
		if err != nil {
			return nil, fmt.Errorf("config: invalid TRUSTED_PROXIES entry %q: must be an IP address or a CIDR block", entry)
		}
		addr = addr.Unmap()
		proxies = append(proxies, netip.PrefixFrom(addr, addr.BitLen()))
	}

	return proxies, nil
}
