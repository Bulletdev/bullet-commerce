package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetEnv_Fallback(t *testing.T) {
	t.Setenv("TEST_KEY_MISSING", "")
	assert.Equal(t, "default", getEnv("TEST_KEY_MISSING", "default"))
}

func TestGetEnv_Present(t *testing.T) {
	t.Setenv("TEST_KEY", "value")
	assert.Equal(t, "value", getEnv("TEST_KEY", "fallback"))
}

func TestGetInt_Valid(t *testing.T) {
	t.Setenv("TEST_INT", "42")
	assert.Equal(t, 42, getInt("TEST_INT", 0))
}

func TestGetInt_Invalid(t *testing.T) {
	t.Setenv("TEST_INT_BAD", "notanint")
	assert.Equal(t, 99, getInt("TEST_INT_BAD", 99))
}

func TestGetInt_Missing(t *testing.T) {
	assert.Equal(t, 5, getInt("TEST_INT_MISSING_XYZ", 5))
}

func TestGetBool_True(t *testing.T) {
	t.Setenv("TEST_BOOL", "true")
	assert.True(t, getBool("TEST_BOOL", false))
}

func TestGetBool_False(t *testing.T) {
	t.Setenv("TEST_BOOL_F", "false")
	assert.False(t, getBool("TEST_BOOL_F", true))
}

func TestGetBool_Invalid(t *testing.T) {
	t.Setenv("TEST_BOOL_BAD", "yes")
	assert.True(t, getBool("TEST_BOOL_BAD", true))
}

func TestGetDuration_Valid(t *testing.T) {
	t.Setenv("TEST_DUR", "5m")
	assert.Equal(t, 5*time.Minute, getDuration("TEST_DUR", time.Second))
}

func TestGetDuration_Invalid(t *testing.T) {
	t.Setenv("TEST_DUR_BAD", "notaduration")
	assert.Equal(t, time.Hour, getDuration("TEST_DUR_BAD", time.Hour))
}

func TestGetDuration_Missing(t *testing.T) {
	assert.Equal(t, 30*time.Second, getDuration("TEST_DUR_MISSING_XYZ", 30*time.Second))
}
