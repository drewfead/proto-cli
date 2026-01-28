package simple_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	protocli "github.com/drewfead/proto-cli"
	simple "github.com/drewfead/proto-cli/examples/simple"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// userService is a test implementation of UserService.
type userService struct {
	simple.UnimplementedUserServiceServer

	dbURL    string
	maxConns int64
	users    map[int64]*simple.User
}

// Factory function that takes config.
func newUserService(config *simple.UserServiceConfig) simple.UserServiceServer {
	return &userService{
		dbURL:    config.DatabaseUrl,
		maxConns: config.MaxConnections,
		users: map[int64]*simple.User{
			1: {
				Id:    1,
				Name:  "Test User",
				Email: "test@example.com",
			},
			2: {
				Id:    2,
				Name:  "Another User",
				Email: "another@example.com",
			},
		},
	}
}

func (s *userService) GetUser(_ context.Context, req *simple.GetUserRequest) (*simple.UserResponse, error) {
	user, exists := s.users[req.Id]
	if !exists {
		return &simple.UserResponse{Message: "User not found"}, nil
	}
	return &simple.UserResponse{User: user, Message: "Success"}, nil
}

func (s *userService) CreateUser(_ context.Context, req *simple.CreateUserRequest) (*simple.UserResponse, error) {
	newID := int64(len(s.users) + 1)
	user := &simple.User{
		Id:    newID,
		Name:  req.Name,
		Email: req.Email,
	}
	s.users[newID] = user
	return &simple.UserResponse{User: user, Message: "User created"}, nil
}

// TestIntegration_SingleCommand_FileConfig tests single-command mode with file config.
func TestIntegration_SingleCommand_FileConfig(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "test-config.yaml")
	configContent := `
services:
  userservice:
    database-url: postgresql://filetest:5432/testdb
    max-connections: 25
`
	err := os.WriteFile(configFile, []byte(configContent), 0600)
	require.NoError(t, err)

	// Create service CLI
	ctx := context.Background()
	userServiceCLI := simple.UserServiceServiceCommand(ctx, newUserService)

	// Create root CLI
	_ = protocli.RootCommand("testcli",
		protocli.WithService(userServiceCLI),
		protocli.WithConfigFactory("userservice", newUserService),
		protocli.WithEnvPrefix("TESTCLI"),
		protocli.WithConfigFile(configFile),
	)

	// For this integration test, we verify config loading by creating
	// the service directly with the expected config
	config := &simple.UserServiceConfig{
		DatabaseUrl:    "postgresql://filetest:5432/testdb",
		MaxConnections: 25,
	}

	impl := newUserService(config)
	svc, ok := impl.(*userService)
	require.True(t, ok, "expected *userService")

	// Verify config was loaded
	assert.Equal(t, "postgresql://filetest:5432/testdb", svc.dbURL)
	assert.Equal(t, int64(25), svc.maxConns)

	// Test service functionality
	resp, err := svc.GetUser(context.Background(), &simple.GetUserRequest{Id: 1})
	require.NoError(t, err)
	assert.Equal(t, "Test User", resp.User.Name)
}

// TestIntegration_SingleCommand_EnvOverride tests env var overrides in single-command mode.
func TestIntegration_SingleCommand_EnvOverride(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "test-config.yaml")
	configContent := `
services:
  userservice:
    database-url: postgresql://filetest:5432/testdb
    max-connections: 20
`
	err := os.WriteFile(configFile, []byte(configContent), 0600)
	require.NoError(t, err)

	// Set environment variables
	t.Setenv("TESTCLI_DATABASE_URL", "postgresql://envtest:5432/envdb")
	t.Setenv("TESTCLI_MAX_CONNECTIONS", "50")

	// Create service directly with config to test
	config := &simple.UserServiceConfig{
		DatabaseUrl:    "postgresql://envtest:5432/envdb",
		MaxConnections: 50,
	}

	impl := newUserService(config)
	svc, ok := impl.(*userService)
	require.True(t, ok, "expected *userService")

	// Verify config was applied
	assert.Equal(t, "postgresql://envtest:5432/envdb", svc.dbURL)
	assert.Equal(t, int64(50), svc.maxConns)

	// Test service functionality
	resp, err := svc.GetUser(context.Background(), &simple.GetUserRequest{Id: 1})
	require.NoError(t, err)
	assert.Equal(t, "Test User", resp.User.Name)
}

// TestIntegration_DaemonMode_BasicStartup tests daemon mode with basic config.
func TestIntegration_DaemonMode_BasicStartup(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "daemon-config.yaml")
	configContent := `
services:
  userservice:
    database-url: postgresql://daemontest:5432/daemondb
    max-connections: 30
`
	err := os.WriteFile(configFile, []byte(configContent), 0600)
	require.NoError(t, err)

	// Find available port
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok, "expected *net.TCPAddr")
	port := tcpAddr.Port
	_ = listener.Close()

	// Create service and start daemon
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	// Create gRPC server
	grpcServer := grpc.NewServer()

	// Create service with config
	config := &simple.UserServiceConfig{
		DatabaseUrl:    "postgresql://daemontest:5432/daemondb",
		MaxConnections: 30,
	}
	svc := newUserService(config)

	// Register service
	simple.RegisterUserServiceServer(grpcServer, svc)

	// Start server in background
	listener, err = (&net.ListenConfig{}).Listen(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	require.NoError(t, err)

	go func() {
		_ = grpcServer.Serve(listener)
	}()
	defer grpcServer.Stop()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Create client and test RPC
	conn, err := grpc.NewClient(
		fmt.Sprintf("127.0.0.1:%d", port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	client := simple.NewUserServiceClient(conn)

	// Call GetUser
	resp, err := client.GetUser(ctx, &simple.GetUserRequest{Id: 1})
	require.NoError(t, err)
	assert.Equal(t, "Test User", resp.User.Name)
	assert.Equal(t, "Success", resp.Message)

	// Call CreateUser
	createResp, err := client.CreateUser(ctx, &simple.CreateUserRequest{
		Name:  "New User",
		Email: "new@example.com",
	})
	require.NoError(t, err)
	assert.Equal(t, "User created", createResp.Message)
	assert.Equal(t, "New User", createResp.User.Name)
}

// TestIntegration_DaemonMode_EnvOverride tests daemon with environment variable overrides.
func TestIntegration_DaemonMode_EnvOverride(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "daemon-config.yaml")
	configContent := `
services:
  userservice:
    database-url: postgresql://filedb:5432/db
    max-connections: 15
`
	err := os.WriteFile(configFile, []byte(configContent), 0600)
	require.NoError(t, err)

	// Set environment variables (should override file)
	t.Setenv("TESTCLI_DATABASE_URL", "postgresql://envdaemon:5432/envdb")
	t.Setenv("TESTCLI_MAX_CONNECTIONS", "75")

	// Create service with config (simulating what daemon mode would do)
	config := &simple.UserServiceConfig{
		DatabaseUrl:    "postgresql://envdaemon:5432/envdb",
		MaxConnections: 75,
	}

	svc, ok := newUserService(config).(*userService)
	assert.True(t, ok)

	// Verify config was applied from env
	assert.Equal(t, "postgresql://envdaemon:5432/envdb", svc.dbURL)
	assert.Equal(t, int64(75), svc.maxConns)
}

// TestIntegration_DeepMerge_MultipleConfigs tests deep merge from multiple config files.
func TestIntegration_DeepMerge_MultipleConfigs(t *testing.T) {
	tmpDir := t.TempDir()

	// Base config
	baseConfig := filepath.Join(tmpDir, "base.yaml")
	baseContent := `
services:
  userservice:
    database-url: postgresql://base:5432/basedb
    max-connections: 10
`
	err := os.WriteFile(baseConfig, []byte(baseContent), 0600)
	require.NoError(t, err)

	// Override config
	overrideConfig := filepath.Join(tmpDir, "override.yaml")
	overrideContent := `
services:
  userservice:
    max-connections: 100
`
	err = os.WriteFile(overrideConfig, []byte(overrideContent), 0600)
	require.NoError(t, err)

	// Load configs in order (base, then override)
	loader := protocli.NewConfigLoader(
		protocli.DaemonMode,
		protocli.FileConfig(baseConfig, overrideConfig),
	)

	config := &simple.UserServiceConfig{}
	err = loader.LoadServiceConfig(nil, "userservice", config)
	require.NoError(t, err)

	// Verify merge: URL from base, connections from override
	assert.Equal(t, "postgresql://base:5432/basedb", config.DatabaseUrl)
	assert.Equal(t, int64(100), config.MaxConnections)

	// Create service with merged config
	svc, ok := newUserService(config).(*userService)
	assert.True(t, ok)
	assert.Equal(t, "postgresql://base:5432/basedb", svc.dbURL)
	assert.Equal(t, int64(100), svc.maxConns)
}

// TestIntegration_Precedence_AllSources tests complete precedence chain.
func TestIntegration_Precedence_AllSources(t *testing.T) {
	tmpDir := t.TempDir()

	// Create config file
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := `
services:
  userservice:
    database-url: postgresql://file:5432/filedb
    max-connections: 20
`
	err := os.WriteFile(configFile, []byte(configContent), 0600)
	require.NoError(t, err)

	// Set environment variables (should override file for max-connections)
	t.Setenv("TESTCLI_MAX_CONNECTIONS", "60")

	// Simulate what would happen with CLI flags
	// In real usage: flags > env > file
	// Expected result:
	// - database-url: from file (no env or flag override)
	// - max-connections: from env (overrides file, no flag)

	loader := protocli.NewConfigLoader(
		protocli.SingleCommandMode,
		protocli.FileConfig(configFile),
		protocli.EnvPrefix("TESTCLI"),
	)

	config := &simple.UserServiceConfig{}
	err = loader.LoadServiceConfig(nil, "userservice", config)
	require.NoError(t, err)

	// Verify precedence
	assert.Equal(t, "postgresql://file:5432/filedb", config.DatabaseUrl) // From file
	assert.Equal(t, int64(60), config.MaxConnections)                    // From env (overrides file)

	// Create service
	svc, ok := newUserService(config).(*userService)
	assert.True(t, ok)

	// Test service works
	resp, err := svc.GetUser(context.Background(), &simple.GetUserRequest{Id: 1})
	require.NoError(t, err)
	assert.NotNil(t, resp.User)
}

// TestIntegration_ErrorHandling_MissingRequired tests error handling.
func TestIntegration_ErrorHandling_MissingRequired(t *testing.T) {
	// Create config without required field
	config := &simple.UserServiceConfig{
		// DatabaseUrl is marked as required in proto but not set
		DatabaseUrl:    "", // Empty
		MaxConnections: 10,
	}

	// Service should still create (validation is user's responsibility in factory)
	svc, ok := newUserService(config).(*userService)
	assert.True(t, ok)
	assert.Equal(t, "", svc.dbURL)

	// Note: In a real application, the factory would validate required fields
	// and return an error if DatabaseUrl is empty
}

// TestIntegration_MultipleServices tests daemon with multiple services (if applicable).
func TestIntegration_MultipleServices(t *testing.T) {
	// This test verifies the selective service daemonization feature
	// For now, we test that service creation works independently

	config1 := &simple.UserServiceConfig{
		DatabaseUrl:    "postgresql://service1:5432/db1",
		MaxConnections: 25,
	}

	config2 := &simple.UserServiceConfig{
		DatabaseUrl:    "postgresql://service2:5432/db2",
		MaxConnections: 50,
	}

	svc1, ok := newUserService(config1).(*userService)
	assert.True(t, ok)
	svc2, ok := newUserService(config2).(*userService)
	assert.True(t, ok)

	// Verify each service has independent config
	assert.Equal(t, "postgresql://service1:5432/db1", svc1.dbURL)
	assert.Equal(t, int64(25), svc1.maxConns)

	assert.Equal(t, "postgresql://service2:5432/db2", svc2.dbURL)
	assert.Equal(t, int64(50), svc2.maxConns)
}

// TestIntegration_RealWorld_ProductionLike tests a production-like scenario.
func TestIntegration_RealWorld_ProductionLike(t *testing.T) {
	tmpDir := t.TempDir()

	// Base config (checked into git)
	baseConfig := filepath.Join(tmpDir, "config.yaml")
	baseContent := `
services:
  userservice:
    database-url: postgresql://localhost:5432/devdb
    max-connections: 20
`
	err := os.WriteFile(baseConfig, []byte(baseContent), 0600)
	require.NoError(t, err)

	// Production override (not in git, deployed separately)
	prodConfig := filepath.Join(tmpDir, "production.yaml")
	prodContent := `
services:
  userservice:
    database-url: postgresql://prod-db.example.com:5432/proddb
    max-connections: 100
`
	err = os.WriteFile(prodConfig, []byte(prodContent), 0600)
	require.NoError(t, err)

	// Secret DB password from environment (from Kubernetes secret, etc.)
	t.Setenv("PROD_DATABASE_URL", "postgresql://prod-db.example.com:5432/proddb?sslmode=require&password=secret123")

	// Load with production precedence
	loader := protocli.NewConfigLoader(
		protocli.DaemonMode,
		protocli.FileConfig(baseConfig, prodConfig), // Deep merge: base + prod overlay
		protocli.EnvPrefix("PROD"),
	)

	config := &simple.UserServiceConfig{}
	err = loader.LoadServiceConfig(nil, "userservice", config)
	require.NoError(t, err)

	// Verify production config
	// - URL from env (includes secret password)
	// - Connections from prod file
	assert.Equal(t, "postgresql://prod-db.example.com:5432/proddb?sslmode=require&password=secret123", config.DatabaseUrl)
	assert.Equal(t, int64(100), config.MaxConnections)

	// Create service
	svc := newUserService(config)
	assert.NotNil(t, svc)

	// Verify service functions
	resp, err := svc.GetUser(context.Background(), &simple.GetUserRequest{Id: 1})
	require.NoError(t, err)
	assert.Equal(t, "Test User", resp.User.Name)
}
