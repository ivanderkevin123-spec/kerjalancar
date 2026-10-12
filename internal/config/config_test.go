package config_test

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/config"
)

func getter(m map[string]string) config.Getter {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := config.Load(getter(map[string]string{
		"DATABASE_URL": "postgres://u:p@localhost:5432/db",
	}))
	require.NoError(t, err)
	assert.Equal(t, "development", cfg.Env)
	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, "http://localhost:8080", cfg.BaseURL)
	assert.Equal(t, slog.LevelInfo, cfg.LogLevel)
	assert.Equal(t, "postgres://u:p@localhost:5432/db", cfg.DatabaseURL)
}

func TestLoadParsesValues(t *testing.T) {
	cfg, err := config.Load(getter(map[string]string{
		"APP_ENV":      "production",
		"PORT":         "9090",
		"BASE_URL":     "https://app.kerjalancar.id/",
		"LOG_LEVEL":    "warn",
		"DATABASE_URL": "postgres://u:p@db:5432/app",
	}))
	require.NoError(t, err)
	assert.Equal(t, "production", cfg.Env)
	assert.Equal(t, 9090, cfg.Port)
	assert.Equal(t, "https://app.kerjalancar.id", cfg.BaseURL, "trailing slash removed")
	assert.Equal(t, slog.LevelWarn, cfg.LogLevel)
}

func TestLoadReportsAllErrors(t *testing.T) {
	_, err := config.Load(getter(map[string]string{
		"APP_ENV":   "prod",
		"PORT":      "abc",
		"LOG_LEVEL": "verbose",
	}))
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "APP_ENV")
	assert.Contains(t, msg, "PORT")
	assert.Contains(t, msg, "LOG_LEVEL")
	assert.Contains(t, msg, "DATABASE_URL")
}

func TestLoadRejectsPortOutOfRange(t *testing.T) {
	_, err := config.Load(getter(map[string]string{"PORT": "70000", "DATABASE_URL": "x"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PORT")
}

func TestLoadRequiresHTTPSInProduction(t *testing.T) {
	_, err := config.Load(getter(map[string]string{
		"APP_ENV":      "production",
		"BASE_URL":     "http://example.com",
		"DATABASE_URL": "x",
	}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "https")
}

func TestLoadTrustedProxy(t *testing.T) {
	cfg, err := config.Load(getter(map[string]string{"DATABASE_URL": "x"}))
	require.NoError(t, err)
	assert.False(t, cfg.TrustedProxy, "off by default: headers from the internet must not be trusted")

	cfg, err = config.Load(getter(map[string]string{"DATABASE_URL": "x", "TRUSTED_PROXY": "true"}))
	require.NoError(t, err)
	assert.True(t, cfg.TrustedProxy)

	_, err = config.Load(getter(map[string]string{"DATABASE_URL": "x", "TRUSTED_PROXY": "yes"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TRUSTED_PROXY")
}

func TestLoadRegistrationDefaultsByEnvironment(t *testing.T) {
	dev, err := config.Load(getter(map[string]string{"DATABASE_URL": "x"}))
	require.NoError(t, err)
	assert.True(t, dev.RegistrationOpen, "open in development")

	prod, err := config.Load(getter(map[string]string{"DATABASE_URL": "x", "APP_ENV": "production", "BASE_URL": "https://app.example"}))
	require.NoError(t, err)
	assert.False(t, prod.RegistrationOpen, "closed in production unless enabled")

	prodOpen, err := config.Load(getter(map[string]string{"DATABASE_URL": "x", "APP_ENV": "production", "BASE_URL": "https://app.example", "REGISTRATION_OPEN": "true", "REGISTRATION_CODE": " pilot-2026 "}))
	require.NoError(t, err)
	assert.True(t, prodOpen.RegistrationOpen)
	assert.Equal(t, "pilot-2026", prodOpen.RegistrationCode, "code is trimmed")

	_, err = config.Load(getter(map[string]string{"DATABASE_URL": "x", "REGISTRATION_OPEN": "maybe"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "REGISTRATION_OPEN")
}
