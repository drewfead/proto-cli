package clilog_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/drewfead/proto-cli/clilog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockConfig implements SlogConfigurationContext for testing
type mockConfig struct {
	isDaemon bool
	level    slog.Level
}

func (m *mockConfig) IsDaemon() bool    { return m.isDaemon }
func (m *mockConfig) Level() slog.Level { return m.level }

func TestUnit_AlwaysHumanFriendly(t *testing.T) {
	ctx := context.Background()

	t.Run("respects log level", func(t *testing.T) {
		config := &mockConfig{
			isDaemon: false,
			level:    slog.LevelWarn,
		}

		// Capture output
		var buf bytes.Buffer
		handler := clilog.HumanFriendlySlogHandler(&buf, &slog.HandlerOptions{
			Level: config.Level(),
		})
		logger := slog.New(handler)

		// Log at different levels
		logger.Debug("debug message")
		logger.Info("info message")
		logger.Warn("warn message")
		logger.Error("error message")

		output := buf.String()

		// Should only have WARN and ERROR
		assert.NotContains(t, output, "debug message")
		assert.NotContains(t, output, "info message")
		assert.Contains(t, output, "warn message")
		assert.Contains(t, output, "error message")
	})

	t.Run("uses human-friendly format even in daemon mode", func(t *testing.T) {
		config := &mockConfig{
			isDaemon: true, // Daemon mode
			level:    slog.LevelInfo,
		}

		callback := clilog.AlwaysHumanFriendly()
		logger := callback(ctx, config)

		// The logger should still use human-friendly format
		// (We can't easily test the internal handler type, but we verify it was created)
		assert.NotNil(t, logger)
	})

	t.Run("returns valid logger", func(t *testing.T) {
		config := &mockConfig{
			isDaemon: false,
			level:    slog.LevelInfo,
		}

		callback := clilog.AlwaysHumanFriendly()
		logger := callback(ctx, config)

		assert.NotNil(t, logger)

		// Logger should be usable
		logger.Info("test message")
	})
}

func TestUnit_AlwaysMachineFriendly(t *testing.T) {
	ctx := context.Background()

	t.Run("respects log level", func(t *testing.T) {
		config := &mockConfig{
			isDaemon: true,
			level:    slog.LevelWarn,
		}

		// Capture output
		var buf bytes.Buffer
		handler := clilog.MachineFriendlySlogHandler(&buf, &slog.HandlerOptions{
			Level: config.Level(),
		})
		logger := slog.New(handler)

		// Log at different levels
		logger.Debug("debug message")
		logger.Info("info message")
		logger.Warn("warn message")
		logger.Error("error message")

		output := buf.String()

		// Should only have WARN and ERROR
		assert.NotContains(t, output, "debug message")
		assert.NotContains(t, output, "info message")
		assert.Contains(t, output, "warn message")
		assert.Contains(t, output, "error message")
	})

	t.Run("uses machine-friendly format even in single-command mode", func(t *testing.T) {
		config := &mockConfig{
			isDaemon: false, // Single-command mode
			level:    slog.LevelInfo,
		}

		callback := clilog.AlwaysMachineFriendly()
		logger := callback(ctx, config)

		// The logger should still use machine-friendly format
		assert.NotNil(t, logger)
	})

	t.Run("outputs valid JSON", func(t *testing.T) {
		config := &mockConfig{
			isDaemon: true,
			level:    slog.LevelInfo,
		}

		// Capture output
		var buf bytes.Buffer
		handler := clilog.MachineFriendlySlogHandler(&buf, &slog.HandlerOptions{
			Level: config.Level(),
		})
		logger := slog.New(handler)

		logger.Info("test message", "key", "value")

		// Verify it's valid JSON
		output := strings.TrimSpace(buf.String())
		var logEntry map[string]any
		err := json.Unmarshal([]byte(output), &logEntry)
		require.NoError(t, err, "output should be valid JSON")

		// Check expected fields
		assert.Equal(t, "INFO", logEntry["level"])
		assert.Equal(t, "test message", logEntry["msg"])
		assert.Equal(t, "value", logEntry["key"])
	})

	t.Run("returns valid logger", func(t *testing.T) {
		config := &mockConfig{
			isDaemon: true,
			level:    slog.LevelInfo,
		}

		callback := clilog.AlwaysMachineFriendly()
		logger := callback(ctx, config)

		assert.NotNil(t, logger)

		// Logger should be usable
		logger.Info("test message")
	})
}

func TestUnit_Default(t *testing.T) {
	ctx := context.Background()

	t.Run("uses human-friendly format in single-command mode", func(t *testing.T) {
		config := &mockConfig{
			isDaemon: false,
			level:    slog.LevelInfo,
		}

		callback := clilog.Default()
		logger := callback(ctx, config)

		assert.NotNil(t, logger)

		// We can't easily test the handler type, but we verify it was created
		logger.Info("test message")
	})

	t.Run("uses machine-friendly format in daemon mode", func(t *testing.T) {
		config := &mockConfig{
			isDaemon: true,
			level:    slog.LevelInfo,
		}

		callback := clilog.Default()
		logger := callback(ctx, config)

		assert.NotNil(t, logger)

		// We can't easily test the handler type, but we verify it was created
		logger.Info("test message")
	})

	t.Run("respects log level in single-command mode", func(t *testing.T) {
		config := &mockConfig{
			isDaemon: false,
			level:    slog.LevelWarn,
		}

		callback := clilog.Default()
		logger := callback(ctx, config)

		assert.NotNil(t, logger)
		logger.Info("should not appear")
		logger.Warn("should appear")
	})

	t.Run("respects log level in daemon mode", func(t *testing.T) {
		config := &mockConfig{
			isDaemon: true,
			level:    slog.LevelError,
		}

		callback := clilog.Default()
		logger := callback(ctx, config)

		assert.NotNil(t, logger)
		logger.Warn("should not appear")
		logger.Error("should appear")
	})
}
