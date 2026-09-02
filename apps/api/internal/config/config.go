// Package config loads application configuration from environment
// variables, applying defaults and validating values before the rest of
// the application starts.
package config

import (
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
)

const (
	defaultPort     = 8080
	defaultEnv      = "development"
	defaultLogLevel = "info"

	minPort = 1
	maxPort = 65535
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

	return cfg, nil
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
