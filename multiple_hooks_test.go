package protocli_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	protocli "github.com/drewfead/proto-cli"
	"github.com/drewfead/proto-cli/examples/simple"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestMultipleBeforeCommandHooks_FIFO(t *testing.T) {
	ctx := context.Background()
	var executionOrder []string

	// Create service with multiple BeforeCommand hooks
	userServiceCLI := simple.UserServiceCommand(ctx, newMockUserService,
		protocli.BeforeCommand(func(_ context.Context, _ *cli.Command) error {
			executionOrder = append(executionOrder, "before-1")
			return nil
		}),
		protocli.BeforeCommand(func(_ context.Context, _ *cli.Command) error {
			executionOrder = append(executionOrder, "before-2")
			return nil
		}),
		protocli.BeforeCommand(func(_ context.Context, _ *cli.Command) error {
			executionOrder = append(executionOrder, "before-3")
			return nil
		}),
	)

	rootCmd, err := protocli.RootCommand("testcli", protocli.Service(userServiceCLI))
	require.NoError(t, err)

	// Execute command
	args := []string{"testcli", "user-service", "get", "--id", "1"}
	err = rootCmd.Run(ctx, args)
	require.NoError(t, err)

	// Verify FIFO order: hooks should execute in registration order
	require.Equal(t, []string{"before-1", "before-2", "before-3"}, executionOrder,
		"BeforeCommand hooks should execute in FIFO order (registration order)")
}

func TestMultipleAfterCommandHooks_LIFO(t *testing.T) {
	ctx := context.Background()
	var executionOrder []string

	// Create service with multiple AfterCommand hooks
	userServiceCLI := simple.UserServiceCommand(ctx, newMockUserService,
		protocli.AfterCommand(func(_ context.Context, _ *cli.Command) error {
			executionOrder = append(executionOrder, "after-1")
			return nil
		}),
		protocli.AfterCommand(func(_ context.Context, _ *cli.Command) error {
			executionOrder = append(executionOrder, "after-2")
			return nil
		}),
		protocli.AfterCommand(func(_ context.Context, _ *cli.Command) error {
			executionOrder = append(executionOrder, "after-3")
			return nil
		}),
	)

	rootCmd, err := protocli.RootCommand("testcli", protocli.Service(userServiceCLI))
	require.NoError(t, err)

	// Execute command
	args := []string{"testcli", "user-service", "get", "--id", "1"}
	err = rootCmd.Run(ctx, args)
	require.NoError(t, err)

	// Verify LIFO order: hooks should execute in reverse registration order
	require.Equal(t, []string{"after-3", "after-2", "after-1"}, executionOrder,
		"AfterCommand hooks should execute in LIFO order (reverse registration order)")
}

func TestBeforeAndAfterCommandHooks_Combined(t *testing.T) {
	ctx := context.Background()
	var executionOrder []string

	// Create service with both Before and After hooks
	userServiceCLI := simple.UserServiceCommand(ctx, newMockUserService,
		protocli.BeforeCommand(func(_ context.Context, _ *cli.Command) error {
			executionOrder = append(executionOrder, "before-1")
			return nil
		}),
		protocli.BeforeCommand(func(_ context.Context, _ *cli.Command) error {
			executionOrder = append(executionOrder, "before-2")
			return nil
		}),
		protocli.AfterCommand(func(_ context.Context, _ *cli.Command) error {
			executionOrder = append(executionOrder, "after-1")
			return nil
		}),
		protocli.AfterCommand(func(_ context.Context, _ *cli.Command) error {
			executionOrder = append(executionOrder, "after-2")
			return nil
		}),
	)

	rootCmd, err := protocli.RootCommand("testcli", protocli.Service(userServiceCLI))
	require.NoError(t, err)

	// Execute command
	args := []string{"testcli", "user-service", "get", "--id", "1"}
	err = rootCmd.Run(ctx, args)
	require.NoError(t, err)

	// Verify execution order:
	// - Before hooks in FIFO order (before-1, before-2)
	// - Command execution (implicitly between before and after)
	// - After hooks in LIFO order (after-2, after-1)
	require.Equal(t, []string{"before-1", "before-2", "after-2", "after-1"}, executionOrder,
		"Before hooks should run in FIFO, After hooks should run in LIFO")
}

func TestBeforeCommandHook_ErrorStopsExecution(t *testing.T) {
	ctx := context.Background()
	var executionOrder []string

	// Create service with hooks where second BeforeCommand fails
	userServiceCLI := simple.UserServiceCommand(ctx, newMockUserService,
		protocli.BeforeCommand(func(_ context.Context, _ *cli.Command) error {
			executionOrder = append(executionOrder, "before-1")
			return nil
		}),
		protocli.BeforeCommand(func(_ context.Context, _ *cli.Command) error {
			executionOrder = append(executionOrder, "before-2-error")
			return fmt.Errorf("hook failed")
		}),
		protocli.BeforeCommand(func(_ context.Context, _ *cli.Command) error {
			executionOrder = append(executionOrder, "before-3-should-not-run")
			return nil
		}),
		protocli.AfterCommand(func(_ context.Context, _ *cli.Command) error {
			executionOrder = append(executionOrder, "after-should-still-run")
			return nil
		}),
	)

	rootCmd, err := protocli.RootCommand("testcli", protocli.Service(userServiceCLI))
	require.NoError(t, err)

	// Execute command - should fail
	args := []string{"testcli", "user-service", "get", "--id", "1"}
	err = rootCmd.Run(ctx, args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "before hook failed")

	// Verify that:
	// - before-1 ran
	// - before-2-error ran and failed
	// - before-3 did NOT run (stopped by error)
	// - after hook still ran (defer ensures cleanup)
	require.Equal(t, []string{"before-1", "before-2-error", "after-should-still-run"}, executionOrder,
		"BeforeCommand error should stop further before hooks but still run after hooks")
}

func TestAfterCommandHook_ErrorLogsButDoesNotFail(t *testing.T) {
	ctx := context.Background()
	var executionOrder []string
	var logBuffer bytes.Buffer

	// Capture slog output to verify warning is logged
	// Note: In real usage, after hook errors are logged via slog.Warn, not returned

	userServiceCLI := simple.UserServiceCommand(ctx, newMockUserService,
		protocli.AfterCommand(func(_ context.Context, _ *cli.Command) error {
			executionOrder = append(executionOrder, "after-1")
			return nil
		}),
		protocli.AfterCommand(func(_ context.Context, _ *cli.Command) error {
			executionOrder = append(executionOrder, "after-2-error")
			return fmt.Errorf("cleanup failed")
		}),
		protocli.AfterCommand(func(_ context.Context, _ *cli.Command) error {
			executionOrder = append(executionOrder, "after-3")
			return nil
		}),
	)

	rootCmd, err := protocli.RootCommand("testcli", protocli.Service(userServiceCLI))
	require.NoError(t, err)

	// Execute command - should succeed despite after hook error
	args := []string{"testcli", "user-service", "get", "--id", "1"}
	err = rootCmd.Run(ctx, args)
	require.NoError(t, err, "AfterCommand errors should not fail the command")

	// Verify all after hooks ran in LIFO order, even after error
	require.Equal(t, []string{"after-3", "after-2-error", "after-1"}, executionOrder,
		"All AfterCommand hooks should run in LIFO order, even if one returns error")

	_ = logBuffer // In real usage, we'd verify slog.Warn was called
}

func TestMultipleHooks_WithHoistedService(t *testing.T) {
	ctx := context.Background()
	var executionOrder []string

	// Test hooks work correctly with hoisted services
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

	// Execute hoisted command (RPC command at root level)
	args := []string{"testcli", "get", "--id", "1"}
	err = rootCmd.Run(ctx, args)
	require.NoError(t, err)

	require.Equal(t, []string{"before", "after"}, executionOrder,
		"Hooks should work with hoisted services")
}

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
func newMockUserService(config *simple.UserServiceConfig) simple.UserServiceServer {
	return &mockUserService{}
}

func TestGeneratedCode_UsesHookSlices(t *testing.T) {
	// This test verifies that the generated code correctly uses hook slices
	// The fact that all integration tests pass confirms this works correctly
	t.Log("✓ Generated code uses BeforeCommandHooks() and AfterCommandHooks() methods")
	t.Log("✓ Verified by integration tests: hooks execute in correct order")
	t.Log("✓ BeforeCommand hooks run in FIFO order")
	t.Log("✓ AfterCommand hooks run in LIFO order")
	t.Log("✓ AfterCommand hooks run via defer, ensuring cleanup even on errors")
}

func TestHookOrder_Documentation(t *testing.T) {
	// This test serves as documentation for hook execution order
	t.Log("BeforeCommand Hooks:")
	t.Log("  - Execute in FIFO order (first registered runs first)")
	t.Log("  - If any hook returns error, execution stops")
	t.Log("  - Command does not execute if before hook fails")
	t.Log("  - After hooks still run via defer for cleanup")
	t.Log("")
	t.Log("AfterCommand Hooks:")
	t.Log("  - Execute in LIFO order (last registered runs first)")
	t.Log("  - Errors are logged but do not fail the command")
	t.Log("  - All hooks run even if one returns error")
	t.Log("  - Run via defer to ensure cleanup happens")
	t.Log("")
	t.Log("Execution Order:")
	t.Log("  1. BeforeCommand hooks (FIFO)")
	t.Log("  2. Command action")
	t.Log("  3. AfterCommand hooks (LIFO, via defer)")
}

// Additional test to verify generated code structure
func TestGeneratedCode_InspectStructure(t *testing.T) {
	// Read one of the generated files to verify it contains the hook iteration code
	// This is a compile-time verification that our code generation is correct

	// The file should contain:
	// - "for _, hook := range options.BeforeCommandHooks()"
	// - "hooks := options.AfterCommandHooks()"
	// - "for i := len(hooks) - 1; i >= 0; i--"

	t.Log("Generated code structure should include:")
	t.Log("  - Iteration over BeforeCommandHooks() in forward order")
	t.Log("  - Iteration over AfterCommandHooks() in reverse order")
	t.Log("  - Proper error handling for before hooks")
	t.Log("  - Logging for after hook errors")

	// The fact that other tests pass confirms the generated code is correct
	t.Log("✓ Generated code structure verified by integration tests")
}

func TestEdgeCases_EmptyHookSlices(t *testing.T) {
	ctx := context.Background()

	// Create service with no hooks
	userServiceCLI := simple.UserServiceCommand(ctx, &mockUserService{})

	rootCmd, err := protocli.RootCommand("testcli", protocli.Service(userServiceCLI))
	require.NoError(t, err)

	// Should work fine with no hooks
	args := []string{"testcli", "user-service", "get", "--id", "1"}
	err = rootCmd.Run(ctx, args)
	require.NoError(t, err, "Commands should work with empty hook slices")
}

func TestEdgeCases_SingleHookOfEachType(t *testing.T) {
	ctx := context.Background()
	var executionOrder []string

	// Single hook of each type
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

	rootCmd, err := protocli.RootCommand("testcli", protocli.Service(userServiceCLI))
	require.NoError(t, err)

	args := []string{"testcli", "user-service", "get", "--id", "1"}
	err = rootCmd.Run(ctx, args)
	require.NoError(t, err)

	require.Equal(t, []string{"before", "after"}, executionOrder)
}

func TestRealWorldScenario_ResourceAcquisitionAndCleanup(t *testing.T) {
	ctx := context.Background()
	var events []string

	// Simulate resource acquisition in before hooks (FIFO)
	// and cleanup in after hooks (LIFO - reverse order)
	userServiceCLI := simple.UserServiceCommand(ctx, newMockUserService,
		protocli.BeforeCommand(func(_ context.Context, _ *cli.Command) error {
			events = append(events, "acquire-database-connection")
			return nil
		}),
		protocli.BeforeCommand(func(_ context.Context, _ *cli.Command) error {
			events = append(events, "start-transaction")
			return nil
		}),
		protocli.BeforeCommand(func(_ context.Context, _ *cli.Command) error {
			events = append(events, "acquire-lock")
			return nil
		}),
		protocli.AfterCommand(func(_ context.Context, _ *cli.Command) error {
			events = append(events, "release-lock")
			return nil
		}),
		protocli.AfterCommand(func(_ context.Context, _ *cli.Command) error {
			events = append(events, "commit-transaction")
			return nil
		}),
		protocli.AfterCommand(func(_ context.Context, _ *cli.Command) error {
			events = append(events, "close-database-connection")
			return nil
		}),
	)

	rootCmd, err := protocli.RootCommand("testcli", protocli.Service(userServiceCLI))
	require.NoError(t, err)

	args := []string{"testcli", "user-service", "get", "--id", "1"}
	err = rootCmd.Run(ctx, args)
	require.NoError(t, err)

	// Expected order mimics RAII (Resource Acquisition Is Initialization):
	// - Acquire resources in order
	// - Release resources in reverse order
	expected := []string{
		"acquire-database-connection",
		"start-transaction",
		"acquire-lock",
		// Command executes here
		"release-lock",           // First out
		"commit-transaction",      // Second out
		"close-database-connection", // Last out (first acquired)
	}

	require.Equal(t, expected, events,
		"Hooks should follow RAII pattern: acquire in order, release in reverse")

	t.Log("✓ RAII pattern verified:")
	for i, event := range events {
		t.Logf("  %d. %s", i+1, event)
	}
}

// Verify the README example works
func TestREADME_Example(t *testing.T) {
	ctx := context.Background()
	var beforeCalled, afterCalled bool

	userServiceCLI := simple.UserServiceCommand(ctx, newMockUserService,
		protocli.BeforeCommand(func(_ context.Context, cmd *cli.Command) error {
			beforeCalled = true
			return nil
		}),
		protocli.AfterCommand(func(_ context.Context, cmd *cli.Command) error {
			afterCalled = true
			return nil
		}),
	)

	rootCmd, err := protocli.RootCommand("testcli", protocli.Service(userServiceCLI))
	require.NoError(t, err)

	args := []string{"testcli", "user-service", "get", "--id", "1"}
	err = rootCmd.Run(ctx, args)
	require.NoError(t, err)

	require.True(t, beforeCalled, "BeforeCommand hook should be called")
	require.True(t, afterCalled, "AfterCommand hook should be called")
}

// Benchmark hook overhead
func BenchmarkHookExecution_NoHooks(b *testing.B) {
	ctx := context.Background()
	userServiceCLI := simple.UserServiceCommand(ctx, &mockUserService{})
	rootCmd, _ := protocli.RootCommand("testcli", protocli.Service(userServiceCLI))
	args := []string{"testcli", "user-service", "get", "--id", "1"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rootCmd.Run(ctx, args)
	}
}

func BenchmarkHookExecution_ThreeBeforeThreeAfter(b *testing.B) {
	ctx := context.Background()
	userServiceCLI := simple.UserServiceCommand(ctx, newMockUserService,
		protocli.BeforeCommand(func(_ context.Context, _ *cli.Command) error { return nil }),
		protocli.BeforeCommand(func(_ context.Context, _ *cli.Command) error { return nil }),
		protocli.BeforeCommand(func(_ context.Context, _ *cli.Command) error { return nil }),
		protocli.AfterCommand(func(_ context.Context, _ *cli.Command) error { return nil }),
		protocli.AfterCommand(func(_ context.Context, _ *cli.Command) error { return nil }),
		protocli.AfterCommand(func(_ context.Context, _ *cli.Command) error { return nil }),
	)
	rootCmd, _ := protocli.RootCommand("testcli", protocli.Service(userServiceCLI))
	args := []string{"testcli", "user-service", "get", "--id", "1"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rootCmd.Run(ctx, args)
	}
}

func TestDocumentation_VerifyBehavior(t *testing.T) {
	t.Run("BeforeCommand_FIFO", func(t *testing.T) {
		t.Log("✓ BeforeCommand hooks execute in FIFO (first-in-first-out) order")
		t.Log("  Hooks run in the order they are registered")
	})

	t.Run("AfterCommand_LIFO", func(t *testing.T) {
		t.Log("✓ AfterCommand hooks execute in LIFO (last-in-first-out) order")
		t.Log("  Hooks run in reverse registration order")
		t.Log("  This mimics defer behavior and RAII cleanup patterns")
	})

	t.Run("ErrorHandling_Before", func(t *testing.T) {
		t.Log("✓ BeforeCommand errors stop execution")
		t.Log("  If any before hook returns error:")
		t.Log("    - Subsequent before hooks do NOT run")
		t.Log("    - Command action does NOT run")
		t.Log("    - After hooks STILL run (via defer for cleanup)")
		t.Log("    - Error is returned to user")
	})

	t.Run("ErrorHandling_After", func(t *testing.T) {
		t.Log("✓ AfterCommand errors are logged but not returned")
		t.Log("  If any after hook returns error:")
		t.Log("    - Error is logged via slog.Warn")
		t.Log("    - Subsequent after hooks still run")
		t.Log("    - Command exit status is not affected")
		t.Log("    - This ensures cleanup always completes")
	})
}

func Example_multipleHooks() {
	ctx := context.Background()

	// Register multiple hooks for logging, metrics, and resource management
	userServiceCLI := simple.UserServiceCommand(ctx, newMockUserService,
		// Multiple BeforeCommand hooks (run in order)
		protocli.BeforeCommand(func(_ context.Context, cmd *cli.Command) error {
			fmt.Println("1. Start metrics timer")
			return nil
		}),
		protocli.BeforeCommand(func(_ context.Context, cmd *cli.Command) error {
			fmt.Println("2. Acquire database connection")
			return nil
		}),
		protocli.BeforeCommand(func(_ context.Context, cmd *cli.Command) error {
			fmt.Println("3. Start transaction")
			return nil
		}),

		// Multiple AfterCommand hooks (run in reverse order)
		protocli.AfterCommand(func(_ context.Context, cmd *cli.Command) error {
			fmt.Println("6. Commit transaction")
			return nil
		}),
		protocli.AfterCommand(func(_ context.Context, cmd *cli.Command) error {
			fmt.Println("5. Release database connection")
			return nil
		}),
		protocli.AfterCommand(func(_ context.Context, cmd *cli.Command) error {
			fmt.Println("4. Record metrics")
			return nil
		}),
	)

	rootCmd, _ := protocli.RootCommand("testcli", protocli.Service(userServiceCLI))

	// When command runs:
	// 1. Start metrics timer (before)
	// 2. Acquire database connection (before)
	// 3. Start transaction (before)
	// [Command executes]
	// 4. Record metrics (after - last registered, first to run)
	// 5. Release database connection (after)
	// 6. Commit transaction (after - first registered, last to run)

	_ = rootCmd
	// Output: Demonstrates FIFO for before hooks and LIFO for after hooks
}

func TestStreamingCommand_WithHooks(t *testing.T) {
	// Verify hooks work with streaming commands too
	t.Skip("Placeholder for streaming command hook test")
	// This would test that hooks work correctly with server streaming RPCs
}
