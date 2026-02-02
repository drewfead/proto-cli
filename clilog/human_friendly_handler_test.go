package clilog_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/drewfead/proto-cli/clilog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnit_HumanFriendlyHandler_BasicFormatting(t *testing.T) {
	var buf bytes.Buffer
	handler := clilog.HumanFriendlySlogHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	logger := slog.New(handler)

	tests := []struct {
		name            string
		logFunc         func()
		wantContains    []string
		wantNotContains []string
	}{
		{
			name: "info level",
			logFunc: func() {
				logger.Info("test message")
			},
			wantContains:    []string{"[INFO]", "test message"},
			wantNotContains: []string{"time=", "level="},
		},
		{
			name: "warn level",
			logFunc: func() {
				logger.Warn("warning message")
			},
			wantContains: []string{"[WARN]", "warning message"},
		},
		{
			name: "error level",
			logFunc: func() {
				logger.Error("error message")
			},
			wantContains: []string{"[ERROR]", "error message"},
		},
		{
			name: "debug level",
			logFunc: func() {
				logger.Debug("debug message")
			},
			wantContains: []string{"[DEBUG]", "debug message"},
		},
		{
			name: "with attributes",
			logFunc: func() {
				logger.Info("test message", "key", "value", "count", 42)
			},
			wantContains: []string{"[INFO]", "test message", "key=value", "count=42"},
		},
		{
			name: "with quoted value",
			logFunc: func() {
				logger.Info("test message", "key", "value with spaces")
			},
			wantContains: []string{"[INFO]", "test message", `key="value with spaces"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			tt.logFunc()
			output := buf.String()

			for _, want := range tt.wantContains {
				assert.Contains(t, output, want, "output should contain %q", want)
			}
			for _, notWant := range tt.wantNotContains {
				assert.NotContains(t, output, notWant, "output should not contain %q", notWant)
			}
		})
	}
}

func TestUnit_HumanHandler_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	handler := clilog.HumanFriendlySlogHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	})
	logger := slog.New(handler)

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")

	output := buf.String()
	assert.NotContains(t, output, "debug message")
	assert.NotContains(t, output, "info message")
	assert.Contains(t, output, "warn message")
	assert.Contains(t, output, "error message")
}

func TestUnit_HumanHandler_WithAttrs(t *testing.T) {
	var buf bytes.Buffer
	handler := clilog.HumanFriendlySlogHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger := slog.New(handler).With("service", "test-service", "version", "1.0")

	logger.Info("test message", "request", "123")

	output := buf.String()
	assert.Contains(t, output, "[INFO]")
	assert.Contains(t, output, "test message")
	assert.Contains(t, output, "service=test-service")
	assert.Contains(t, output, "version=1.0")
	assert.Contains(t, output, "request=123")
}

func TestUnit_HumanHandler_Enabled(t *testing.T) {
	handler := clilog.HumanFriendlySlogHandler(&bytes.Buffer{}, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	})

	ctx := context.Background()
	assert.False(t, handler.Enabled(ctx, slog.LevelDebug))
	assert.False(t, handler.Enabled(ctx, slog.LevelInfo))
	assert.True(t, handler.Enabled(ctx, slog.LevelWarn))
	assert.True(t, handler.Enabled(ctx, slog.LevelError))
}

func TestUnit_HumanHandler_ComplexTypes(t *testing.T) {
	var buf bytes.Buffer
	handler := clilog.HumanFriendlySlogHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger := slog.New(handler)

	logger.Info("test message",
		"bool", true,
		"int", 42,
		"float", 3.14,
		"duration", 5*time.Second,
	)

	output := buf.String()
	assert.Contains(t, output, "bool=true")
	assert.Contains(t, output, "int=42")
	assert.Contains(t, output, "float=3.14")
	assert.Contains(t, output, "duration=5s")
}

func TestUnit_HumanHandler_NoTimestamp(t *testing.T) {
	var buf bytes.Buffer
	handler := clilog.HumanFriendlySlogHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger := slog.New(handler)

	logger.Info("test message")

	output := buf.String()
	// Should not contain timestamp patterns
	assert.NotContains(t, output, "time=")
	assert.NotContains(t, output, "2024")
	assert.NotContains(t, output, "2025")
	assert.NotContains(t, output, "2026")
	// Should not match common timestamp formats
	assert.False(t, strings.Contains(output, ":"))
}

func TestUnit_HumanHandler_Colorization(t *testing.T) {
	var buf bytes.Buffer
	handler := clilog.HumanFriendlySlogHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	logger := slog.New(handler)

	tests := []struct {
		name     string
		logFunc  func()
		wantANSI string
	}{
		{
			name: "error has red color",
			logFunc: func() {
				logger.Error("test")
			},
			wantANSI: "\033[31m", // red
		},
		{
			name: "warn has yellow color",
			logFunc: func() {
				logger.Warn("test")
			},
			wantANSI: "\033[33m", // yellow
		},
		{
			name: "info has blue color",
			logFunc: func() {
				logger.Info("test")
			},
			wantANSI: "\033[34m", // blue
		},
		{
			name: "debug has gray color",
			logFunc: func() {
				logger.Debug("test")
			},
			wantANSI: "\033[90m", // gray
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			tt.logFunc()
			output := buf.String()
			assert.Contains(t, output, tt.wantANSI, "should contain ANSI color code")
			assert.Contains(t, output, "\033[0m", "should contain reset code")
		})
	}
}

func TestUnit_HumanHandler_NilOptions(t *testing.T) {
	var buf bytes.Buffer
	// Should not panic with nil options
	handler := clilog.HumanFriendlySlogHandler(&buf, nil)
	require.NotNil(t, handler)

	logger := slog.New(handler)
	logger.Info("test message")

	output := buf.String()
	assert.Contains(t, output, "[INFO]")
	assert.Contains(t, output, "test message")
}
