package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("JWT_SECRET", "secret-at-least-32-chars-long-xx")
	t.Setenv("PORT", "")
	t.Setenv("ALLOWED_ORIGINS", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("CACHE_ENABLED", "")
	t.Setenv("CACHE_L1_MAX_SIZE", "")
	t.Setenv("CACHE_TTL_PRODUCTS", "")

	cfg := Load()
	require.NotNil(t, cfg)
	assert.Equal(t, "postgres://localhost/test", cfg.DatabaseURL)
	assert.Equal(t, "secret-at-least-32-chars-long-xx", cfg.JWTSecret)
	assert.Equal(t, "4444", cfg.Port)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.True(t, cfg.CacheEnabled)
	assert.Equal(t, 1000, cfg.CacheL1MaxSize)
	assert.Equal(t, 5*time.Minute, cfg.CacheTTL)
}

func TestLoad_CustomValues(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://custom/db")
	t.Setenv("JWT_SECRET", "custom-secret-min-32-chars-xxxxx")
	t.Setenv("PORT", "9090")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("CACHE_ENABLED", "false")
	t.Setenv("CACHE_L1_MAX_SIZE", "500")
	t.Setenv("CACHE_TTL_PRODUCTS", "10m")

	cfg := Load()
	assert.Equal(t, "9090", cfg.Port)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.False(t, cfg.CacheEnabled)
	assert.Equal(t, 500, cfg.CacheL1MaxSize)
	assert.Equal(t, 10*time.Minute, cfg.CacheTTL)
}
