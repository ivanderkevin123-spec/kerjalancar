// Package config loads and validates process configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
)

// Config is the validated process configuration.
type Config struct {
	Env         string // development, test, or production
	Port        int
	DatabaseURL string
	LogLevel    slog.Level
	BaseURL     string // no trailing slash
	// TrustedProxy enables reading the client IP from X-Forwarded-For / X-Real-IP.
	// Only true when the app sits behind a proxy that overwrites those headers.
	TrustedProxy bool
}

// Getter returns the value of an environment variable, or "" when unset.
// os.Getenv satisfies it; tests pass a map-backed function.
type Getter func(key string) string

// Load reads every variable, validates all of them, and returns every problem
// joined into one error so the operator fixes them in one go.
func Load(get Getter) (Config, error) {
	var errs []error

	cfg := Config{
		Env:     strings.TrimSpace(or(get("APP_ENV"), "development")),
		BaseURL: strings.TrimRight(strings.TrimSpace(or(get("BASE_URL"), "http://localhost:8080")), "/"),
	}

	switch cfg.Env {
	case "development", "test", "production":
	default:
		errs = append(errs, fmt.Errorf("APP_ENV must be development, test, or production (got %q)", cfg.Env))
	}

	rawPort := or(get("PORT"), "8080")
	port, err := strconv.Atoi(strings.TrimSpace(rawPort))
	if err != nil || port < 1 || port > 65535 {
		errs = append(errs, fmt.Errorf("PORT must be a number between 1 and 65535 (got %q)", rawPort))
	}
	cfg.Port = port

	cfg.DatabaseURL = strings.TrimSpace(get("DATABASE_URL"))
	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}

	rawLevel := or(get("LOG_LEVEL"), "info")
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(strings.TrimSpace(rawLevel))); err != nil {
		errs = append(errs, fmt.Errorf("LOG_LEVEL must be debug, info, warn, or error (got %q)", rawLevel))
	}
	cfg.LogLevel = lvl

	if raw := strings.TrimSpace(get("TRUSTED_PROXY")); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			errs = append(errs, fmt.Errorf("TRUSTED_PROXY must be true or false (got %q)", raw))
		}
		cfg.TrustedProxy = v
	}

	if cfg.Env == "production" && !strings.HasPrefix(cfg.BaseURL, "https://") {
		errs = append(errs, errors.New("BASE_URL must use https in production"))
	}

	return cfg, errors.Join(errs...)
}

func or(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
