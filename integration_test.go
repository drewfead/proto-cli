package protocli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	protocli "github.com/drewfead/proto-cli"
	"github.com/drewfead/proto-cli/examples/simple"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

// Test helper functions

// setupTestCLI overrides os.Exit behavior for testing
func setupTestCLI(t *testing.T) {
	t.Helper()
	origExiter := cli.OsExiter
	t.Cleanup(func() { cli.OsExiter = origExiter })
	cli.OsExiter = func(code int) {
		// Don't actually exit during tests
	}
}

// setWriterOnAllCommands sets the writer on all commands and subcommands
func setWriterOnAllCommands(cmd *cli.Command, w io.Writer) {
	cmd.Writer = w
	for _, subCmd := range cmd.Commands {
		setWriterOnAllCommands(subCmd, w)
	}
}

// Test fixtures

var (
	errHookFailed    = errors.New("hook failed")
	errCleanupFailed = errors.New("cleanup failed")
)

// mockUserService implements the UserServiceServer interface for testing
type mockUserService struct {
	simple.UnimplementedUserServiceServer
}

func (m *mockUserService) GetUser(_ context.Context, req *simple.GetUserRequest) (*simple.UserResponse, error) {
	return &simple.UserResponse{
		User: &simple.User{
			Id:    req.Id,
			Name:  "Test User",
			Email: "test@example.com",
		},
	}, nil
}

// newMockUserService is a factory function that takes config and returns a service implementation
func newMockUserService(_ *simple.UserServiceConfig) simple.UserServiceServer {
	return &mockUserService{}
}
// TestHoistedService_FlatCommandStructure tests that hoisted services have commands at root level.
func TestIntegration_HoistedService_FlatCommandStructure(t *testing.T) {
	ctx := context.Background()

	// Create a service CLI
	userServiceCLI := simple.UserServiceCommand(ctx, &simple.UnimplementedUserServiceServer{})

	// Create root command with hoisted service
	rootCmd, err := protocli.RootCommand("testcli",
		protocli.Service(userServiceCLI, protocli.Hoisted()),
	)
	require.NoError(t, err)

	// Verify commands are at root level
	require.NotNil(t, rootCmd)
	require.NotNil(t, rootCmd.Commands)

	// Collect command names
	commandNames := make(map[string]bool)
	for _, cmd := range rootCmd.Commands {
		commandNames[cmd.Name] = true
	}

	// Should have RPC commands at root level
	assert.True(t, commandNames["get"], "get command should be at root level")
	assert.True(t, commandNames["create"], "create command should be at root level")
	assert.True(t, commandNames["daemonize"], "daemonize command should always be present")

	// Should NOT have nested service command
	assert.False(t, commandNames["user-service"], "user-service nested command should not exist when hoisted")
}

// TestHoistedService_NamingCollision tests that naming collisions return an error.
func TestIntegration_HoistedService_NamingCollision(t *testing.T) {
	ctx := context.Background()

	// Create two service CLIs with overlapping command names
	adminServiceCLI := simple.AdminServiceCommand(ctx, &simple.UnimplementedAdminServiceServer{})

	// This should return error because both registrations have the same "health-check" command
	_, err := protocli.RootCommand("testcli",
		protocli.Service(adminServiceCLI, protocli.Hoisted()),
		protocli.Service(adminServiceCLI, protocli.Hoisted()), // Same service twice = guaranteed collision
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, protocli.ErrAmbiguousCommandInvocation)
	assert.True(t,
		strings.Contains(err.Error(), "more than one action registered for the same command"),
		"error message should mention collision: %s", err.Error())
}

// TestHoistedService_DaemonizeCollision tests that 'daemonize' collision is detected.
func TestIntegration_HoistedService_DaemonizeCollision(t *testing.T) {
	// This test would require a service with an RPC named "daemonize" to properly test
	// For now, we'll document that the collision detection exists at root.go:150
	t.Skip("Would need a service with a 'daemonize' RPC to test this collision")
}

// TestIntegration_CommandHooks_ExecutionOrder tests hook execution order and error handling.
func TestIntegration_CommandHooks_ExecutionOrder(t *testing.T) {
	tests := []struct {
		name            string
		beforeHooks     []func(*[]string) protocli.ServiceOption
		afterHooks      []func(*[]string) protocli.ServiceOption
		expectedOrder   []string
		expectError     bool
		errorInHook     string
		description     string
	}{
		{
			name: "before hooks execute in FIFO order",
			beforeHooks: []func(*[]string) protocli.ServiceOption{
				func(order *[]string) protocli.ServiceOption {
					return protocli.BeforeCommand(func(_ context.Context, _ *cli.Command) error {
						*order = append(*order, "before-1")
						return nil
					})
				},
				func(order *[]string) protocli.ServiceOption {
					return protocli.BeforeCommand(func(_ context.Context, _ *cli.Command) error {
						*order = append(*order, "before-2")
						return nil
					})
				},
				func(order *[]string) protocli.ServiceOption {
					return protocli.BeforeCommand(func(_ context.Context, _ *cli.Command) error {
						*order = append(*order, "before-3")
						return nil
					})
				},
			},
			expectedOrder: []string{"before-1", "before-2", "before-3"},
			description:   "BeforeCommand hooks should execute in FIFO order (registration order)",
		},
		{
			name: "after hooks execute in LIFO order",
			afterHooks: []func(*[]string) protocli.ServiceOption{
				func(order *[]string) protocli.ServiceOption {
					return protocli.AfterCommand(func(_ context.Context, _ *cli.Command) error {
						*order = append(*order, "after-1")
						return nil
					})
				},
				func(order *[]string) protocli.ServiceOption {
					return protocli.AfterCommand(func(_ context.Context, _ *cli.Command) error {
						*order = append(*order, "after-2")
						return nil
					})
				},
				func(order *[]string) protocli.ServiceOption {
					return protocli.AfterCommand(func(_ context.Context, _ *cli.Command) error {
						*order = append(*order, "after-3")
						return nil
					})
				},
			},
			expectedOrder: []string{"after-3", "after-2", "after-1"},
			description:   "AfterCommand hooks should execute in LIFO order (reverse registration order)",
		},
		{
			name: "before error stops execution but after hooks still run",
			beforeHooks: []func(*[]string) protocli.ServiceOption{
				func(order *[]string) protocli.ServiceOption {
					return protocli.BeforeCommand(func(_ context.Context, _ *cli.Command) error {
						*order = append(*order, "before-1")
						return nil
					})
				},
				func(order *[]string) protocli.ServiceOption {
					return protocli.BeforeCommand(func(_ context.Context, _ *cli.Command) error {
						*order = append(*order, "before-2-error")
						return errHookFailed
					})
				},
				func(order *[]string) protocli.ServiceOption {
					return protocli.BeforeCommand(func(_ context.Context, _ *cli.Command) error {
						*order = append(*order, "before-3-should-not-run")
						return nil
					})
				},
			},
			afterHooks: []func(*[]string) protocli.ServiceOption{
				func(order *[]string) protocli.ServiceOption {
					return protocli.AfterCommand(func(_ context.Context, _ *cli.Command) error {
						*order = append(*order, "after-still-runs")
						return nil
					})
				},
			},
			expectedOrder: []string{"before-1", "before-2-error", "after-still-runs"},
			expectError:   true,
			description:   "BeforeCommand error should stop further before hooks but still run after hooks",
		},
		{
			name: "after hook error does not fail command",
			afterHooks: []func(*[]string) protocli.ServiceOption{
				func(order *[]string) protocli.ServiceOption {
					return protocli.AfterCommand(func(_ context.Context, _ *cli.Command) error {
						*order = append(*order, "after-1")
						return nil
					})
				},
				func(order *[]string) protocli.ServiceOption {
					return protocli.AfterCommand(func(_ context.Context, _ *cli.Command) error {
						*order = append(*order, "after-2-error")
						return errCleanupFailed
					})
				},
				func(order *[]string) protocli.ServiceOption {
					return protocli.AfterCommand(func(_ context.Context, _ *cli.Command) error {
						*order = append(*order, "after-3")
						return nil
					})
				},
			},
			expectedOrder: []string{"after-3", "after-2-error", "after-1"},
			expectError:   false,
			description:   "All AfterCommand hooks should run even if one returns error",
		},
		{
			name: "RAII pattern - acquire in order release in reverse",
			beforeHooks: []func(*[]string) protocli.ServiceOption{
				func(order *[]string) protocli.ServiceOption {
					return protocli.BeforeCommand(func(_ context.Context, _ *cli.Command) error {
						*order = append(*order, "acquire-database")
						return nil
					})
				},
				func(order *[]string) protocli.ServiceOption {
					return protocli.BeforeCommand(func(_ context.Context, _ *cli.Command) error {
						*order = append(*order, "start-transaction")
						return nil
					})
				},
				func(order *[]string) protocli.ServiceOption {
					return protocli.BeforeCommand(func(_ context.Context, _ *cli.Command) error {
						*order = append(*order, "acquire-lock")
						return nil
					})
				},
			},
			// Register after hooks in reverse order for RAII (LIFO will reverse them back)
			afterHooks: []func(*[]string) protocli.ServiceOption{
				func(order *[]string) protocli.ServiceOption {
					return protocli.AfterCommand(func(_ context.Context, _ *cli.Command) error {
						*order = append(*order, "close-database")
						return nil
					})
				},
				func(order *[]string) protocli.ServiceOption {
					return protocli.AfterCommand(func(_ context.Context, _ *cli.Command) error {
						*order = append(*order, "commit-transaction")
						return nil
					})
				},
				func(order *[]string) protocli.ServiceOption {
					return protocli.AfterCommand(func(_ context.Context, _ *cli.Command) error {
						*order = append(*order, "release-lock")
						return nil
					})
				},
			},
			expectedOrder: []string{
				"acquire-database", "start-transaction", "acquire-lock",
				"release-lock", "commit-transaction", "close-database",
			},
			description: "Hooks should follow RAII pattern: acquire in order, release in reverse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupTestCLI(t)
			ctx := context.Background()
			var executionOrder []string

			var opts []protocli.ServiceOption
			for _, hookFn := range tt.beforeHooks {
				opts = append(opts, hookFn(&executionOrder))
			}
			for _, hookFn := range tt.afterHooks {
				opts = append(opts, hookFn(&executionOrder))
			}

			userServiceCLI := simple.UserServiceCommand(ctx, newMockUserService, opts...)
			rootCmd, err := protocli.RootCommand("testcli", protocli.Service(userServiceCLI))
			require.NoError(t, err)

			// Capture output
			var buf bytes.Buffer
			rootCmd.Writer = &buf
			setWriterOnAllCommands(rootCmd, &buf)

			// Execute command
			args := []string{"testcli", "user-service", "get", "--id", "1"}
			err = rootCmd.Run(ctx, args)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tt.expectedOrder, executionOrder, tt.description)
		})
	}
}

// TestIntegration_CommandHooks_WithHoistedService tests that hooks work with hoisted services.
func TestIntegration_CommandHooks_WithHoistedService(t *testing.T) {
	setupTestCLI(t)
	ctx := context.Background()
	var executionOrder []string

	userServiceCLI := simple.UserServiceCommand(ctx, newMockUserService,
		protocli.BeforeCommand(func(_ context.Context, _ *cli.Command) error {
			executionOrder = append(executionOrder, "before")
			return nil
		}),
		protocli.AfterCommand(func(_ context.Context, _ *cli.Command) error {
			executionOrder = append(executionOrder, "after")
			return nil
		}),
	)

	rootCmd, err := protocli.RootCommand("testcli",
		protocli.Service(userServiceCLI, protocli.Hoisted()),
	)
	require.NoError(t, err)

	// Capture output
	var buf bytes.Buffer
	rootCmd.Writer = &buf
	setWriterOnAllCommands(rootCmd, &buf)

	// Execute hoisted command (RPC command at root level)
	args := []string{"testcli", "get", "--id", "1"}
	err = rootCmd.Run(ctx, args)
	require.NoError(t, err)

	require.Equal(t, []string{"before", "after"}, executionOrder)
}

// TestIntegration_CommandHooks_EmptyHooks tests commands work with no hooks.
func TestIntegration_CommandHooks_EmptyHooks(t *testing.T) {
	setupTestCLI(t)
	ctx := context.Background()

	userServiceCLI := simple.UserServiceCommand(ctx, newMockUserService)
	rootCmd, err := protocli.RootCommand("testcli", protocli.Service(userServiceCLI))
	require.NoError(t, err)

	// Capture output
	var buf bytes.Buffer
	rootCmd.Writer = &buf
	setWriterOnAllCommands(rootCmd, &buf)

	args := []string{"testcli", "user-service", "get", "--id", "1"}
	err = rootCmd.Run(ctx, args)
	require.NoError(t, err, "Commands should work with empty hook slices")
}
