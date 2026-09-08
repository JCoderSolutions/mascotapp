package config

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func TestLoad_AppliesDefaultsWhenEnvEmpty(t *testing.T) {
	clearConfigEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.Env != "development" {
		t.Errorf("Env = %q, want %q", cfg.Env, "development")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.DatabaseURL != "" {
		t.Errorf("DatabaseURL = %q, want empty", cfg.DatabaseURL)
	}
}

func TestLoad_HonorsExplicitValues(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("PORT", "9090")
	t.Setenv("ENV", "production")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/mascotapp")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.Port != 9090 {
		t.Errorf("Port = %d, want 9090", cfg.Port)
	}
	if cfg.Env != "production" {
		t.Errorf("Env = %q, want %q", cfg.Env, "production")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
	if cfg.DatabaseURL != "postgres://user:pass@localhost:5432/mascotapp" {
		t.Errorf("DatabaseURL = %q, want the explicit connection string", cfg.DatabaseURL)
	}
}

func TestLoad_RejectsInvalidPort(t *testing.T) {
	tests := []struct {
		name string
		port string
	}{
		{"not a number", "abc"},
		{"zero", "0"},
		{"negative", "-1"},
		{"above max", "65536"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearConfigEnv(t)
			t.Setenv("PORT", tt.port)

			_, err := Load()
			if err == nil {
				t.Fatalf("Load() with PORT=%q: expected error, got nil", tt.port)
			}
		})
	}
}

func TestLoad_RejectsInvalidEnv(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("ENV", "not-a-real-env")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() with invalid ENV: expected error, got nil")
	}
}

// clearConfigEnv ensures each test starts from a clean slate regardless of
// what is set in the process environment, and restores it after the test.
//
// Every key Load reads belongs here. One left out does not fail loudly: it leaks
// the developer's own environment into the assertion, so the suite passes on one
// machine and fails on another, and the failure names the wrong cause.
func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"PORT", "ENV", "LOG_LEVEL", "DATABASE_URL", "TRUSTED_PROXIES",
		"DATABASE_URL_AUTH", "JWT_SECRET", "AUTH_KEK", "WEB_ORIGINS", "API_PUBLIC_ORIGIN",
	} {
		t.Setenv(key, "")
	}
}

func TestLoad_ParsesTrustedProxies(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"unset means trust nothing", "", nil},
		{"single cidr", "10.0.0.0/8", []string{"10.0.0.0/8"}},
		{"bare ipv4 becomes a host prefix", "10.0.0.1", []string{"10.0.0.1/32"}},
		{"bare ipv6 becomes a host prefix", "2001:db8::1", []string{"2001:db8::1/128"}},
		{"multiple entries with whitespace", " 10.0.0.0/8 , 192.168.0.0/16 ", []string{"10.0.0.0/8", "192.168.0.0/16"}},
		{"trailing separator is ignored", "10.0.0.0/8,", []string{"10.0.0.0/8"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearConfigEnv(t)
			t.Setenv("TRUSTED_PROXIES", tt.raw)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}

			if len(cfg.TrustedProxies) != len(tt.want) {
				t.Fatalf("TrustedProxies = %v, want %v", cfg.TrustedProxies, tt.want)
			}
			for i, want := range tt.want {
				if got := cfg.TrustedProxies[i].String(); got != want {
					t.Errorf("TrustedProxies[%d] = %q, want %q", i, got, want)
				}
			}
		})
	}
}

func TestLoad_RejectsInvalidTrustedProxies(t *testing.T) {
	for _, raw := range []string{"not-an-ip", "10.0.0.0/33", "10.0.0.0/8;192.168.0.0/16"} {
		t.Run(raw, func(t *testing.T) {
			clearConfigEnv(t)
			t.Setenv("TRUSTED_PROXIES", raw)

			if _, err := Load(); err == nil {
				t.Fatalf("Load() with TRUSTED_PROXIES=%q: expected error, got nil", raw)
			}
		})
	}
}

// --- T-02-026: auth DSN, secrets, origins, and P2-D9's boot refusal ---

func TestLoad_ReadsTheAuthConfiguration(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("DATABASE_URL_AUTH", "postgres://app_auth:pass@localhost:5432/mascotapp")
	t.Setenv("JWT_SECRET", strings.Repeat("k", 32))
	t.Setenv("AUTH_KEK", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)))
	t.Setenv("WEB_ORIGINS", "https://mascotapp.pages.dev, https://www.mascotapp.pages.dev")
	t.Setenv("API_PUBLIC_ORIGIN", "https://mascotapp-api.example.org")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.DatabaseURLAuth != "postgres://app_auth:pass@localhost:5432/mascotapp" {
		t.Errorf("DatabaseURLAuth = %q, want the explicit auth connection string", cfg.DatabaseURLAuth)
	}
	if cfg.JWTSecret != strings.Repeat("k", 32) {
		t.Errorf("JWTSecret = %q, want the 32-byte value", cfg.JWTSecret)
	}
	if !bytes.Equal(cfg.AuthKEK, bytes.Repeat([]byte{7}, 32)) {
		t.Errorf("AuthKEK = %x, want the DECODED bytes, not the base64 text", cfg.AuthKEK)
	}

	want := []string{"https://mascotapp.pages.dev", "https://www.mascotapp.pages.dev"}
	if len(cfg.WebOrigins) != len(want) {
		t.Fatalf("WebOrigins = %v, want %v", cfg.WebOrigins, want)
	}
	for i, w := range want {
		if cfg.WebOrigins[i] != w {
			t.Errorf("WebOrigins[%d] = %q, want %q -- entries are trimmed, never rewritten", i, cfg.WebOrigins[i], w)
		}
	}
	if cfg.APIPublicOrigin != "https://mascotapp-api.example.org" {
		t.Errorf("APIPublicOrigin = %q, want the explicit origin", cfg.APIPublicOrigin)
	}
}

// TestLoad_RefusesAWeakJWTSecret pins the HS256 key floor at the hash output
// size. Anything shorter is a key an attacker brute-forces rather than steals.
func TestLoad_RefusesAWeakJWTSecret(t *testing.T) {
	for _, tt := range []struct {
		name   string
		secret string
	}{
		{"one byte short", strings.Repeat("k", 31)},
		{"a short passphrase", "correct horse battery"},
		{"a single character", "k"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			clearConfigEnv(t)
			t.Setenv("JWT_SECRET", tt.secret)

			if _, err := Load(); err == nil {
				t.Fatalf("Load() with a %d-byte JWT_SECRET: expected refusal, got nil", len(tt.secret))
			}
		})
	}
}

// TestLoad_RefusesAnAuthKEKThatIsNotThirtyTwoDecodedBytes covers the trap that a
// key of the right TEXT length is the wrong KEY length: 32 base64 characters
// decode to 24 bytes, and AES-256-GCM needs exactly 32. A check written against
// len(raw) passes that case while handing the cipher a key it must reject.
func TestLoad_RefusesAnAuthKEKThatIsNotThirtyTwoDecodedBytes(t *testing.T) {
	for _, tt := range []struct {
		name string
		kek  string
	}{
		{"not base64 at all", "this is not base64 !!"},
		{"decodes to 31 bytes", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 31))},
		{"decodes to 33 bytes", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 33))},
		{"32 base64 characters decode to 24 bytes", strings.Repeat("A", 32)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			clearConfigEnv(t)
			t.Setenv("AUTH_KEK", tt.kek)

			if _, err := Load(); err == nil {
				t.Fatalf("Load() with AUTH_KEK=%q: expected refusal, got nil", tt.kek)
			}
		})
	}
}

// TestLoad_RefusesToBootWhenWebAndAPIShareARegistrableDomain enforces P2-D9's
// revisit condition. The refresh cookie is fixed at SameSite=None because the
// origins are separate BY DECISION; the day they stop being separate, that
// choice is no longer justified, and the server has to say so on boot instead of
// waiting for someone to remember a comment.
//
// The pairs are chosen so that a hand-rolled "last two labels" comparison fails
// at least twice, in both directions. `.com.mx` is a two-part public suffix such
// a check reads as a registrable domain, so it would MISS a real sharing;
// `pages.dev` is a public suffix it would read as one shared owner, so it would
// INVENT one. Both are this project's real deployment shapes: Cloudflare Pages,
// and the Mexican region the plan assumes.
func TestLoad_RefusesToBootWhenWebAndAPIShareARegistrableDomain(t *testing.T) {
	for _, tt := range []struct {
		name        string
		webOrigins  string
		apiOrigin   string
		wantRefusal bool
	}{
		{
			name:        "the same host",
			webOrigins:  "https://mascotapp.org",
			apiOrigin:   "https://mascotapp.org",
			wantRefusal: true,
		},
		{
			name:        "subdomains of one registrable domain",
			webOrigins:  "https://app.mascotapp.org",
			apiOrigin:   "https://api.mascotapp.org",
			wantRefusal: true,
		},
		{
			name:        "two-part public suffix, which a naive check reads as separate",
			webOrigins:  "https://mascotapp.com.mx",
			apiOrigin:   "https://api.mascotapp.com.mx",
			wantRefusal: true,
		},
		{
			name:        "only one of several web origins shares it",
			webOrigins:  "https://mascotapp.pages.dev, https://app.mascotapp.org",
			apiOrigin:   "https://api.mascotapp.org",
			wantRefusal: true,
		},
		{
			name:        "genuinely different registrable domains",
			webOrigins:  "https://mascotapp.pages.dev",
			apiOrigin:   "https://mascotapp-api.example.org",
			wantRefusal: false,
		},
		{
			name:        "one public suffix, two owners, which a naive check reads as shared",
			webOrigins:  "https://mascotapp.pages.dev",
			apiOrigin:   "https://mascotapp-api.pages.dev",
			wantRefusal: false,
		},
		{
			name:        "two owners under a two-part public suffix",
			webOrigins:  "https://mascotapp.com.mx",
			apiOrigin:   "https://mascotapp-api.com.mx",
			wantRefusal: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			clearConfigEnv(t)
			t.Setenv("WEB_ORIGINS", tt.webOrigins)
			t.Setenv("API_PUBLIC_ORIGIN", tt.apiOrigin)

			_, err := Load()
			if tt.wantRefusal && err == nil {
				t.Fatalf("Load() with WEB_ORIGINS=%q and API_PUBLIC_ORIGIN=%q: expected a boot refusal, got nil",
					tt.webOrigins, tt.apiOrigin)
			}
			if !tt.wantRefusal && err != nil {
				t.Fatalf("Load() with WEB_ORIGINS=%q and API_PUBLIC_ORIGIN=%q: unexpected refusal: %v",
					tt.webOrigins, tt.apiOrigin, err)
			}
		})
	}
}

// TestLoad_SaysWhyItRefusedTheSharedDomain keeps the refusal actionable. A boot
// failure that names neither the domain it found nor the decision it enforces
// sends the next reader hunting through the config. The message is what makes
// this a revisit condition rather than a crash.
func TestLoad_SaysWhyItRefusedTheSharedDomain(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("WEB_ORIGINS", "https://app.mascotapp.org")
	t.Setenv("API_PUBLIC_ORIGIN", "https://api.mascotapp.org")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() with a shared registrable domain: expected a boot refusal, got nil")
	}
	for _, want := range []string{"mascotapp.org", "SameSite", "P2-D9"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal %q does not mention %q", err.Error(), want)
		}
	}
}

// TestLoad_LeavesTheAuthConfigurationEmptyWhenUnset states the BOUNDARY of these
// checks in a test rather than in a comment nobody runs. Load validates values
// that are PRESENT; it does not require them. An empty JWT_SECRET is not a weak
// secret, it is a missing one, and it is refused where it is consumed, when the
// router is wired in T-02-040. Making Load demand them instead would break every
// command that loads configuration without touching auth -- the migration runner
// among them.
func TestLoad_LeavesTheAuthConfigurationEmptyWhenUnset(t *testing.T) {
	clearConfigEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() with no auth configuration set: unexpected error: %v", err)
	}

	if cfg.DatabaseURLAuth != "" {
		t.Errorf("DatabaseURLAuth = %q, want empty", cfg.DatabaseURLAuth)
	}
	if cfg.JWTSecret != "" {
		t.Errorf("JWTSecret = %q, want empty", cfg.JWTSecret)
	}
	if cfg.AuthKEK != nil {
		t.Errorf("AuthKEK = %x, want nil", cfg.AuthKEK)
	}
	if cfg.WebOrigins != nil {
		t.Errorf("WebOrigins = %v, want nil", cfg.WebOrigins)
	}
	if cfg.APIPublicOrigin != "" {
		t.Errorf("APIPublicOrigin = %q, want empty", cfg.APIPublicOrigin)
	}
}
