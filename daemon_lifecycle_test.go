package protocli_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	protocli "github.com/drewfead/proto-cli"
	simple "github.com/drewfead/proto-cli/examples/simple"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
	"google.golang.org/grpc"
)

// preventExit overrides os.Exit behavior for testing daemon commands
func preventExit(t *testing.T) {
	t.Helper()
	origExiter := cli.OsExiter
	t.Cleanup(func() { cli.OsExiter = origExiter })
	cli.OsExiter = func(_ int) {
		// Don't actually exit during tests
	}
}

// TestDaemonLifecycleHooks_StartupReadyShutdown verifies that all lifecycle hooks are called..
func TestIntegration_DaemonLifecycle_StartupReadyShutdown(t *testing.T) {
	preventExit(t)

	var (
		mu             sync.Mutex
		startupCalled  bool
		readyCalled    bool
		shutdownCalled bool
	)

	startup := func(_ context.Context, server *grpc.Server, _ *runtime.ServeMux) error {
		mu.Lock()
		startupCalled = true
		mu.Unlock()
		assert.NotNil(t, server, "gRPC server should not be nil")
		return nil
	}

	ready := func(_ context.Context) {
		mu.Lock()
		readyCalled = true
		mu.Unlock()
	}

	shutdown := func(ctx context.Context) {
		mu.Lock()
		shutdownCalled = true
		mu.Unlock()
		// Verify context has deadline (from graceful shutdown timeout)
		_, hasDeadline := ctx.Deadline()
		assert.True(t, hasDeadline, "shutdown context should have deadline")
	}

	// Create service with lifecycle hooks
	ctx := context.Background()
	userServiceCLI := simple.UserServiceCommand(ctx, newUserService)

	rootCmd, err := protocli.RootCommand("testcli",
		protocli.Service(userServiceCLI),
		protocli.OnDaemonStartup(startup),
		protocli.OnDaemonReady(ready),
		protocli.OnDaemonShutdown(shutdown),
		protocli.WithGracefulShutdownTimeout(2*time.Second),
	)
	require.NoError(t, err)

	// Start daemon in background
	go func() {
		_ = rootCmd.Run(ctx, []string{"testcli", "daemonize", "--port", "50199"})
	}()

	// Wait for server to start
	time.Sleep(500 * time.Millisecond)

	// Verify startup and ready hooks were called
	mu.Lock()
	startupWasCalled := startupCalled
	readyWasCalled := readyCalled
	mu.Unlock()
	assert.True(t, startupWasCalled, "OnDaemonStartup should be called")
	assert.True(t, readyWasCalled, "OnDaemonReady should be called")

	// Send SIGTERM to trigger shutdown
	proc, _ := os.FindProcess(os.Getpid())
	_ = proc.Signal(syscall.SIGTERM)

	// Wait for shutdown (longer than graceful shutdown timeout)
	time.Sleep(3 * time.Second)

	// Verify shutdown hook was called
	mu.Lock()
	shutdownWasCalled := shutdownCalled
	mu.Unlock()
	assert.True(t, shutdownWasCalled, "OnDaemonShutdown should be called")
}

// TestDaemonLifecycleHooks_StartupError verifies that startup error prevents daemon from starting..
func TestIntegration_DaemonLifecycle_StartupError(t *testing.T) {
	startupWithError := func(_ context.Context, _ *grpc.Server, _ *runtime.ServeMux) error {
		return fmt.Errorf("%w: startup validation failed", assert.AnError)
	}

	ctx := context.Background()
	userServiceCLI := simple.UserServiceCommand(ctx, newUserService)

	rootCmd, err := protocli.RootCommand("testcli",
		protocli.Service(userServiceCLI),
		protocli.OnDaemonStartup(startupWithError),
	)
	require.NoError(t, err)

	// Daemon should fail to start
	err = rootCmd.Run(ctx, []string{"testcli", "daemonize", "--port", "50200"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "startup validation failed")
}

// TestDaemonLifecycleHooks_MultipleHooks verifies multiple hooks run in correct order..
func TestIntegration_DaemonLifecycle_MultipleHooks(t *testing.T) {
	preventExit(t)

	var (
		mu        sync.Mutex
		callOrder []string
	)

	startup1 := func(_ context.Context, _ *grpc.Server, _ *runtime.ServeMux) error {
		mu.Lock()
		callOrder = append(callOrder, "startup1")
		mu.Unlock()
		return nil
	}

	startup2 := func(_ context.Context, _ *grpc.Server, _ *runtime.ServeMux) error {
		mu.Lock()
		callOrder = append(callOrder, "startup2")
		mu.Unlock()
		return nil
	}

	ready1 := func(_ context.Context) {
		mu.Lock()
		callOrder = append(callOrder, "ready1")
		mu.Unlock()
	}

	ready2 := func(_ context.Context) {
		mu.Lock()
		callOrder = append(callOrder, "ready2")
		mu.Unlock()
	}

	shutdown1 := func(_ context.Context) {
		mu.Lock()
		callOrder = append(callOrder, "shutdown1")
		mu.Unlock()
	}

	shutdown2 := func(_ context.Context) {
		mu.Lock()
		callOrder = append(callOrder, "shutdown2")
		mu.Unlock()
	}

	ctx := context.Background()
	userServiceCLI := simple.UserServiceCommand(ctx, newUserService)

	rootCmd, err := protocli.RootCommand("testcli",
		protocli.Service(userServiceCLI),
		protocli.OnDaemonStartup(startup1),
		protocli.OnDaemonStartup(startup2),
		protocli.OnDaemonReady(ready1),
		protocli.OnDaemonReady(ready2),
		protocli.OnDaemonShutdown(shutdown1),
		protocli.OnDaemonShutdown(shutdown2),
		protocli.WithGracefulShutdownTimeout(2*time.Second),
	)
	require.NoError(t, err)

	// Start daemon in background
	go func() {
		_ = rootCmd.Run(ctx, []string{"testcli", "daemonize", "--port", "50201"})
	}()

	// Wait for server to start
	time.Sleep(500 * time.Millisecond)

	// Send SIGTERM
	proc, _ := os.FindProcess(os.Getpid())
	_ = proc.Signal(syscall.SIGTERM)

	// Wait for shutdown (longer than graceful shutdown timeout)
	time.Sleep(3 * time.Second)

	// Verify order:
	// - Startup hooks: registration order
	// - Ready hooks: registration order
	// - Shutdown hooks: REVERSE registration order
	expectedOrder := []string{
		"startup1",
		"startup2",
		"ready1",
		"ready2",
		"shutdown2", // Reverse order
		"shutdown1",
	}

	mu.Lock()
	actualOrder := make([]string, len(callOrder))
	copy(actualOrder, callOrder)
	mu.Unlock()

	assert.Equal(t, expectedOrder, actualOrder, "Hooks should run in correct order")
}

// TestDaemonLifecycleHooks_GracefulShutdownTimeout verifies timeout behavior.
func TestIntegration_DaemonLifecycle_GracefulShutdownTimeout(t *testing.T) {
	preventExit(t)

	shutdownStarted := make(chan time.Time, 1)
	shutdownCompleted := make(chan time.Time, 1)

	shutdown := func(_ context.Context) {
		shutdownStarted <- time.Now()
		// Simulate slow shutdown
		time.Sleep(500 * time.Millisecond)
		shutdownCompleted <- time.Now()
	}

	ctx := context.Background()
	userServiceCLI := simple.UserServiceCommand(ctx, newUserService)

	rootCmd, err := protocli.RootCommand("testcli",
		protocli.Service(userServiceCLI),
		protocli.OnDaemonShutdown(shutdown),
		protocli.WithGracefulShutdownTimeout(1*time.Second),
	)
	require.NoError(t, err)

	// Start daemon in background
	go func() {
		_ = rootCmd.Run(ctx, []string{"testcli", "daemonize", "--port", "50202"})
	}()

	// Wait for server to start
	time.Sleep(500 * time.Millisecond)

	// Send SIGTERM
	proc, _ := os.FindProcess(os.Getpid())
	_ = proc.Signal(syscall.SIGTERM)

	// Wait for shutdown (longer than graceful shutdown timeout)
	time.Sleep(3 * time.Second)

	// Verify shutdown hook was called
	select {
	case start := <-shutdownStarted:
		t.Logf("Shutdown started at %v", start)
	default:
		t.Error("Shutdown hook was not called")
	}
}

// TestDaemonLifecycleHooks_AccessToServerInStartup verifies startup hook can configure server.
func TestIntegration_DaemonLifecycle_AccessToServerInStartup(t *testing.T) {
	preventExit(t)

	var (
		mu               sync.Mutex
		serverConfigured bool
	)

	startup := func(_ context.Context, server *grpc.Server, _ *runtime.ServeMux) error {
		// Startup hook has access to gRPC server before it starts
		// This allows configuring server, adding interceptors, etc.
		assert.NotNil(t, server)

		// Example: Could register additional interceptors, configure server, etc.
		mu.Lock()
		serverConfigured = true
		mu.Unlock()
		return nil
	}

	ctx := context.Background()
	userServiceCLI := simple.UserServiceCommand(ctx, newUserService)

	rootCmd, err := protocli.RootCommand("testcli",
		protocli.Service(userServiceCLI),
		protocli.OnDaemonStartup(startup),
	)
	require.NoError(t, err)

	// Start daemon in background
	go func() {
		_ = rootCmd.Run(ctx, []string{"testcli", "daemonize", "--port", "50203"})
	}()

	// Wait for server to start
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	configured := serverConfigured
	mu.Unlock()
	assert.True(t, configured, "Server should be configured in startup hook")

	// Cleanup
	proc, _ := os.FindProcess(os.Getpid())
	_ = proc.Signal(syscall.SIGTERM)
	time.Sleep(3 * time.Second)
}

// Helper: userService implementation for tests.
type testUserService struct {
	simple.UnimplementedUserServiceServer
}

func newUserService(_ *simple.UserServiceConfig) simple.UserServiceServer {
	return &testUserService{}
}
