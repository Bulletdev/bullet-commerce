package main

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetupLogger_AllLevels(t *testing.T) {
	for _, level := range []string{"debug", "warn", "error", "info", "unknown"} {
		setupLogger(level)
	}
	assert.NotNil(t, slog.Default())
}
