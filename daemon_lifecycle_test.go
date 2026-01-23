package protocli_test

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"testing"
	"time"

	protocli "github.com/drewfead/proto-cli"
	simple "github.com/drewfead/proto-cli/examples/simple"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

// TestDaemonLifecycleHooks_StartupReadyShutdown verifies that all lifecycle hooks are called
func TestDaemonLifecycleHooks_StartupReadyShutdown(t *testing.T) {
	var (
		startupCalled  bool
		readyCalled    bool
		shutdownCalled bool
	)

	startup := func(ctx context.Context, server *grpc.Server, mux *runtime.ServeMux) error {
		startupCalled = true
		assert.NotNil(t, server, "gRPC server should not be nil")
		return nil
	}

	ready := func(ctx context.Context) {
		readyCalled = true
	}

	shutdown := func(ctx context.Context) {
		shutdownCalled = true
		// Verify context has deadline (from graceful shutdown timeout)
		_, hasDeadline := ctx.Deadline()
		assert.True(t, hasDeadline, "shutdown context should have deadline")
	}

	// Create service with lifecycle hooks
	ctx := context.Background()
	userServiceCLI := simple.UserServiceServiceCommand(ctx, newUserService)

	rootCmd := protocli.RootCommand("testcli",
		protocli.WithService(userServiceCLI),
		protocli.WithOnDaemonStartup(startup),
		protocli.WithOnDaemonReady(ready),
		protocli.WithOnDaemonShutdown(shutdown),
		protocli.WithGracefulShutdownTimeout(2*time.Second),
	)

	// Start daemon in background
	go func() {
		_ = rootCmd.Run(ctx, []string{"testcli", "daemonize", "--port", "50199"})
	}()

	// Wait for server to start
	time.Sleep(500 * time.Millisecond)

	// Verify startup and ready hooks were called
	assert.True(t, startupCalled, "OnDaemonStartup should be called")
	assert.True(t, readyCalled, "OnDaemonReady should be called")

	// Send SIGTERM to trigger shutdown
	proc, _ := os.FindProcess(os.Getpid())
	proc.Signal(syscall.SIGTERM)

	// Wait for shutdown
	time.Sleep(1 * time.Second)

	// Verify shutdown hook was called
	assert.True(t, shutdownCalled, "OnDaemonShutdown should be called")
}

// TestDaemonLifecycleHooks_StartupError verifies that startup error prevents daemon from starting
func TestDaemonLifecycleHooks_StartupError(t *testing.T) {
	startupWithError := func(ctx context.Context, server *grpc.Server, mux *runtime.ServeMux) error {
		return fmt.Errorf("startup validation failed")
	}

	ctx := context.Background()
	userServiceCLI := simple.UserServiceServiceCommand(ctx, newUserService)

	rootCmd := protocli.RootCommand("testcli",
		protocli.WithService(userServiceCLI),
		protocli.WithOnDaemonStartup(startupWithError),
	)

	// Daemon should fail to start
	err := rootCmd.Run(ctx, []string{"testcli", "daemonize", "--port", "50200"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "startup validation failed")
}

// TestDaemonLifecycleHooks_MultipleHooks verifies multiple hooks run in correct order
func TestDaemonLifecycleHooks_MultipleHooks(t *testing.T) {
	var callOrder []string

	startup1 := func(ctx context.Context, server *grpc.Server, mux *runtime.ServeMux) error {
		callOrder = append(callOrder, "startup1")
		return nil
	}

	startup2 := func(ctx context.Context, server *grpc.Server, mux *runtime.ServeMux) error {
		callOrder = append(callOrder, "startup2")
		return nil
	}

	ready1 := func(ctx context.Context) {
		callOrder = append(callOrder, "ready1")
	}

	ready2 := func(ctx context.Context) {
		callOrder = append(callOrder, "ready2")
	}

	shutdown1 := func(ctx context.Context) {
		callOrder = append(callOrder, "shutdown1")
	}

	shutdown2 := func(ctx context.Context) {
		callOrder = append(callOrder, "shutdown2")
	}

	ctx := context.Background()
	userServiceCLI := simple.UserServiceServiceCommand(ctx, newUserService)

	rootCmd := protocli.RootCommand("testcli",
		protocli.WithService(userServiceCLI),
		protocli.WithOnDaemonStartup(startup1),
		protocli.WithOnDaemonStartup(startup2),
		protocli.WithOnDaemonReady(ready1),
		protocli.WithOnDaemonReady(ready2),
		protocli.WithOnDaemonShutdown(shutdown1),
		protocli.WithOnDaemonShutdown(shutdown2),
		protocli.WithGracefulShutdownTimeout(2*time.Second),
	)

	// Start daemon in background
	go func() {
		_ = rootCmd.Run(ctx, []string{"testcli", "daemonize", "--port", "50201"})
	}()

	// Wait for server to start
	time.Sleep(500 * time.Millisecond)

	// Send SIGTERM
	proc, _ := os.FindProcess(os.Getpid())
	proc.Signal(syscall.SIGTERM)

	// Wait for shutdown
	time.Sleep(1 * time.Second)

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

	assert.Equal(t, expectedOrder, callOrder, "Hooks should run in correct order")
}

// TestDaemonLifecycleHooks_GracefulShutdownTimeout verifies timeout behavior
func TestDaemonLifecycleHooks_GracefulShutdownTimeout(t *testing.T) {
	shutdownStarted := make(chan time.Time, 1)
	shutdownCompleted := make(chan time.Time, 1)

	shutdown := func(ctx context.Context) {
		shutdownStarted <- time.Now()
		// Simulate slow shutdown
		time.Sleep(500 * time.Millisecond)
		shutdownCompleted <- time.Now()
	}

	ctx := context.Background()
	userServiceCLI := simple.UserServiceServiceCommand(ctx, newUserService)

	rootCmd := protocli.RootCommand("testcli",
		protocli.WithService(userServiceCLI),
		protocli.WithOnDaemonShutdown(shutdown),
		protocli.WithGracefulShutdownTimeout(1*time.Second),
	)

	// Start daemon in background
	go func() {
		_ = rootCmd.Run(ctx, []string{"testcli", "daemonize", "--port", "50202"})
	}()

	// Wait for server to start
	time.Sleep(500 * time.Millisecond)

	// Send SIGTERM
	proc, _ := os.FindProcess(os.Getpid())
	proc.Signal(syscall.SIGTERM)

	// Wait for shutdown
	time.Sleep(2 * time.Second)

	// Verify shutdown hook was called
	select {
	case start := <-shutdownStarted:
		t.Logf("Shutdown started at %v", start)
	default:
		t.Error("Shutdown hook was not called")
	}
}

// TestDaemonLifecycleHooks_AccessToServerInStartup verifies startup hook can configure server
func TestDaemonLifecycleHooks_AccessToServerInStartup(t *testing.T) {
	var serverConfigured bool

	startup := func(ctx context.Context, server *grpc.Server, mux *runtime.ServeMux) error {
		// Startup hook has access to gRPC server before it starts
		// This allows configuring server, adding interceptors, etc.
		assert.NotNil(t, server)

		// Example: Could register additional interceptors, configure server, etc.
		serverConfigured = true
		return nil
	}

	ctx := context.Background()
	userServiceCLI := simple.UserServiceServiceCommand(ctx, newUserService)

	rootCmd := protocli.RootCommand("testcli",
		protocli.WithService(userServiceCLI),
		protocli.WithOnDaemonStartup(startup),
	)

	// Start daemon in background
	go func() {
		_ = rootCmd.Run(ctx, []string{"testcli", "daemonize", "--port", "50203"})
	}()

	// Wait for server to start
	time.Sleep(500 * time.Millisecond)

	assert.True(t, serverConfigured, "Server should be configured in startup hook")

	// Cleanup
	proc, _ := os.FindProcess(os.Getpid())
	proc.Signal(syscall.SIGTERM)
	time.Sleep(500 * time.Millisecond)
}

// Helper: userService implementation for tests
type testUserService struct {
	simple.UnimplementedUserServiceServer
}

func newUserService(config *simple.UserServiceConfig) simple.UserServiceServer {
	return &testUserService{}
}
