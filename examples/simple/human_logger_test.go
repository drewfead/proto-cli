package simple_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	protocli "github.com/drewfead/proto-cli"
	"github.com/drewfead/proto-cli/clilog"
	"github.com/drewfead/proto-cli/examples/simple"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

// TestIntegration_HumanLogger_SingleCommandMode tests that single-command mode uses human-friendly logging
func TestIntegration_HumanLogger_SingleCommandMode(t *testing.T) {
	// Setup test CLI
	origExiter := cli.OsExiter
	t.Cleanup(func() { cli.OsExiter = origExiter })
	cli.OsExiter = func(_ int) {}

	ctx := context.Background()

	// Create a service that logs messages
	mockServiceFactory := func(_ *simple.UserServiceConfig) simple.UserServiceServer {
		return &loggingUserService{}
	}

	userServiceCLI := simple.UserServiceCommand(ctx, mockServiceFactory)

	// Custom logger to capture logs
	var logBuf bytes.Buffer

	rootCmd, err := protocli.RootCommand("testcli",
		protocli.Service(userServiceCLI),
		protocli.ConfigureLogging(func(_ context.Context, config protocli.SlogConfigurationContext) *slog.Logger {
			handler := clilog.HumanFriendlySlogHandler(&logBuf, &slog.HandlerOptions{
				Level: config.Level(), // Respects --verbosity flag
			})
			return slog.New(handler)
		}),
		protocli.WithDefaultVerbosity(slog.LevelDebug), // Set to debug for this test
	)
	require.NoError(t, err)

	// Capture command output
	var cmdBuf bytes.Buffer
	rootCmd.Writer = &cmdBuf
	setWriterOnAllCommands(rootCmd, &cmdBuf)

	// Execute command
	args := []string{
		"testcli", "user-service", "get",
		"--id", "123",
	}
	err = rootCmd.Run(ctx, args)
	require.NoError(t, err)

	logOutput := logBuf.String()

	// Verify human-friendly format
	t.Logf("Log output:\n%s", logOutput)

	// Should contain colorized log levels (ANSI codes)
	assert.Contains(t, logOutput, "\033[", "should contain ANSI color codes")
	assert.Contains(t, logOutput, "[INFO]", "should contain [INFO] level")

	// Should NOT contain timestamp or traditional structured fields
	assert.NotContains(t, logOutput, "time=", "should not have time= field")
	assert.NotContains(t, logOutput, "level=", "should not have level= field")
	assert.NotContains(t, logOutput, "msg=", "should not have msg= field")

	// Should contain the actual log message
	assert.Contains(t, logOutput, "fetching user", "should contain log message")
}

// TestIntegration_HumanLogger_Attributes tests that attributes are formatted nicely
func TestIntegration_HumanLogger_Attributes(t *testing.T) {
	var buf bytes.Buffer
	handler := clilog.HumanFriendlySlogHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger := slog.New(handler)

	// Log with various attributes
	logger.Info("processing request",
		"user_id", 123,
		"action", "create_user",
		"duration", "150ms",
		"success", true,
	)

	output := buf.String()
	t.Logf("Output:\n%s", output)

	// Verify formatted output
	assert.Contains(t, output, "[INFO]")
	assert.Contains(t, output, "processing request")
	assert.Contains(t, output, "user_id=123")
	assert.Contains(t, output, "action=create_user")
	assert.Contains(t, output, "duration=150ms")
	assert.Contains(t, output, "success=true")
}

// TestIntegration_HumanLogger_ErrorsStandOut tests that errors are highlighted
func TestIntegration_HumanLogger_ErrorsStandOut(t *testing.T) {
	var buf bytes.Buffer
	handler := clilog.HumanFriendlySlogHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	logger := slog.New(handler)

	// Log different levels
	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warning message")
	logger.Error("error message")

	output := buf.String()
	t.Logf("Output:\n%s", output)

	// Verify each level has distinct color
	lines := bytes.Split(buf.Bytes(), []byte("\n"))

	// Find lines with different log levels
	var debugLine, infoLine, warnLine, errorLine string
	for _, line := range lines {
		s := string(line)
		if len(s) == 0 {
			continue
		}
		switch {
		case bytes.Contains(line, []byte("debug message")):
			debugLine = s
		case bytes.Contains(line, []byte("info message")):
			infoLine = s
		case bytes.Contains(line, []byte("warning message")):
			warnLine = s
		case bytes.Contains(line, []byte("error message")):
			errorLine = s
		}
	}

	// Verify colors are different
	assert.Contains(t, debugLine, "\033[90m", "debug should be gray")
	assert.Contains(t, infoLine, "\033[34m", "info should be blue")
	assert.Contains(t, warnLine, "\033[33m", "warn should be yellow")
	assert.Contains(t, errorLine, "\033[31m", "error should be red")
}

// TestIntegration_HumanLogger_RespectsVerbosityFlag tests that custom logger respects --verbosity
func TestIntegration_HumanLogger_RespectsVerbosityFlag(t *testing.T) {
	// Setup test CLI
	origExiter := cli.OsExiter
	t.Cleanup(func() { cli.OsExiter = origExiter })
	cli.OsExiter = func(_ int) {}

	ctx := context.Background()

	// Create a service that logs at different levels
	mockServiceFactory := func(_ *simple.UserServiceConfig) simple.UserServiceServer {
		return &multiLevelLoggingUserService{}
	}

	userServiceCLI := simple.UserServiceCommand(ctx, mockServiceFactory)

	// Custom logger to capture logs
	var logBuf bytes.Buffer

	rootCmd, err := protocli.RootCommand("testcli",
		protocli.Service(userServiceCLI),
		protocli.ConfigureLogging(func(_ context.Context, config protocli.SlogConfigurationContext) *slog.Logger {
			handler := clilog.HumanFriendlySlogHandler(&logBuf, &slog.HandlerOptions{
				Level: config.Level(), // Respects --verbosity flag
			})
			return slog.New(handler)
		}),
	)
	require.NoError(t, err)

	// Capture command output
	var cmdBuf bytes.Buffer
	rootCmd.Writer = &cmdBuf
	setWriterOnAllCommands(rootCmd, &cmdBuf)

	// Execute command with --verbosity=warn (should only show WARN and ERROR)
	args := []string{
		"testcli", "user-service", "get",
		"--id", "123",
		"--verbosity", "warn",
	}
	err = rootCmd.Run(ctx, args)
	require.NoError(t, err)

	logOutput := logBuf.String()
	t.Logf("Log output with --verbosity=warn:\n%s", logOutput)

	// Should NOT contain DEBUG or INFO (filtered by level)
	assert.NotContains(t, logOutput, "[DEBUG]", "should not have debug logs")
	assert.NotContains(t, logOutput, "[INFO]", "should not have info logs")
	assert.NotContains(t, logOutput, "debug message", "should not contain debug message")
	assert.NotContains(t, logOutput, "info message", "should not contain info message")

	// Should contain WARN and ERROR
	assert.Contains(t, logOutput, "[WARN]", "should have warn logs")
	assert.Contains(t, logOutput, "warn message", "should contain warn message")
	assert.Contains(t, logOutput, "[ERROR]", "should have error logs")
	assert.Contains(t, logOutput, "error message", "should contain error message")
}

// loggingUserService is a mock service that logs during operations
type loggingUserService struct {
	simple.UnimplementedUserServiceServer
}

func (s *loggingUserService) GetUser(_ context.Context, req *simple.GetUserRequest) (*simple.UserResponse, error) {
	slog.Info("fetching user", "user_id", req.Id)
	return &simple.UserResponse{
		User: &simple.User{
			Id:    req.Id,
			Name:  "Test User",
			Email: "test@example.com",
		},
	}, nil
}

// multiLevelLoggingUserService logs at multiple levels to test filtering
type multiLevelLoggingUserService struct {
	simple.UnimplementedUserServiceServer
}

func (s *multiLevelLoggingUserService) GetUser(_ context.Context, req *simple.GetUserRequest) (*simple.UserResponse, error) {
	slog.Debug("debug message", "user_id", req.Id)
	slog.Info("info message", "user_id", req.Id)
	slog.Warn("warn message", "user_id", req.Id)
	slog.Error("error message", "user_id", req.Id)
	return &simple.UserResponse{
		User: &simple.User{
			Id:    req.Id,
			Name:  "Test User",
			Email: "test@example.com",
		},
	}, nil
}
