package config

import "testing"

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
func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"PORT", "ENV", "LOG_LEVEL", "DATABASE_URL", "TRUSTED_PROXIES"} {
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
