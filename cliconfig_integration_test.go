package protocli_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	protocli "github.com/drewfead/proto-cli"
	"github.com/drewfead/proto-cli/examples/simple"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
	"google.golang.org/grpc"
)

// TestIntegration_ConfigManagement_EndToEnd tests the full config management lifecycle:
// init, set, get, list, and actual config loading
func TestIntegration_ConfigManagement_EndToEnd(t *testing.T) {
	// Create temp directory for config files
	tempDir := t.TempDir()
	localConfigPath := filepath.Join(tempDir, ".testapp", "config.yaml")

	// Create a test service with config (empty proto - no defaults)
	configProto := &simple.UserServiceConfig{}

	serviceCLI := &protocli.ServiceCLI{
		Command: &cli.Command{
			Name:  "userservice",
			Usage: "User service commands",
			Commands: []*cli.Command{
				{
					Name:  "get-user",
					Usage: "Get a user",
					Action: func(ctx context.Context, cmd *cli.Command) error {
						return nil
					},
				},
			},
		},
		ServiceName:       "userservice",
		ConfigMessageType: "example.UserServiceConfig",
		ConfigPrototype:   configProto,
		FactoryOrImpl: func(cfg *simple.UserServiceConfig) *simple.UnimplementedUserServiceServer {
			// Store config in context or somewhere we can verify it
			t.Logf("Factory called with config: DatabaseUrl=%s, MaxConnections=%d",
				cfg.DatabaseUrl, cfg.MaxConnections)
			return &simple.UnimplementedUserServiceServer{}
		},
		RegisterFunc: func(s *grpc.Server, impl any) {
			// No-op for this test
		},
	}

	// Create root command with config management
	rootCmd, err := protocli.RootCommand("testapp",
		protocli.Service(serviceCLI),
		protocli.WithConfigManagementCommands(configProto, "testapp", "userservice"),
		protocli.WithLocalConfigPath(localConfigPath),
		protocli.WithConfigFile(localConfigPath),
	)
	require.NoError(t, err)

	// Helper to run commands
	runCommand := func(args ...string) (string, error) {
		var buf bytes.Buffer
		rootCmd.Writer = &buf

		// Reset command state
		for _, cmd := range rootCmd.Commands {
			cmd.Writer = &buf
			setWriterRecursive(cmd, &buf)
		}

		err := rootCmd.Run(context.Background(), append([]string{"testapp"}, args...))
		return buf.String(), err
	}

	// Step 1: Verify config command exists
	output, err := runCommand("--help")
	require.NoError(t, err)
	require.Contains(t, output, "config")

	// Step 2: Create initial config with config set
	output, err = runCommand("config", "set",
		"databaseUrl=postgres://localhost/testdb",
		"maxConnections=25")
	require.NoError(t, err)
	require.Contains(t, output, "Set 2 value(s)")
	require.Contains(t, output, localConfigPath)

	// Verify file was created
	require.FileExists(t, localConfigPath)

	// Step 3: Get individual config values
	output, err = runCommand("config", "get", "databaseUrl")
	require.NoError(t, err)
	require.Contains(t, output, "postgres://localhost/testdb")
	require.Contains(t, output, localConfigPath)

	output, err = runCommand("config", "get", "maxConnections")
	require.NoError(t, err)
	require.Contains(t, output, "25")
	require.Contains(t, output, localConfigPath)

	// Step 4: List all config values
	output, err = runCommand("config", "list")
	require.NoError(t, err)
	require.Contains(t, output, "databaseUrl: postgres://localhost/testdb")
	require.Contains(t, output, "maxConnections: 25")
	require.Contains(t, output, localConfigPath)

	// Step 5: Update a config value
	output, err = runCommand("config", "set", "maxConnections=50")
	require.NoError(t, err)
	require.Contains(t, output, "Set 1 value(s)")

	// Verify the update
	output, err = runCommand("config", "get", "maxConnections")
	require.NoError(t, err)
	require.Contains(t, output, "50")

	// Step 6: Verify config can be read back correctly after update
	output, err = runCommand("config", "get", "databaseUrl")
	require.NoError(t, err)
	require.Contains(t, output, "postgres://localhost/testdb")
	require.Contains(t, output, localConfigPath)

	output, err = runCommand("config", "get", "maxConnections")
	require.NoError(t, err)
	require.Contains(t, output, "50")

	// Step 7: Test invalid key
	_, err = runCommand("config", "get", "nonexistent.key")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid config key")

	// Step 8: Test setting array field (should fail with helpful message)
	_, err = runCommand("config", "set", "some.list.0=value")
	require.Error(t, err)
}

// TestIntegration_ConfigManagement_GlobalVsLocal tests global and local config precedence
func TestIntegration_ConfigManagement_GlobalVsLocal(t *testing.T) {
	tempDir := t.TempDir()
	globalConfigPath := filepath.Join(tempDir, "global", "config.yaml")
	localConfigPath := filepath.Join(tempDir, "local", ".testapp", "config.yaml")

	configProto := &simple.UserServiceConfig{}

	serviceCLI := &protocli.ServiceCLI{
		Command: &cli.Command{
			Name:  "userservice",
			Usage: "User service commands",
		},
		ServiceName:       "userservice",
		ConfigMessageType: "example.UserServiceConfig",
		ConfigPrototype:   configProto,
		FactoryOrImpl:     &simple.UnimplementedUserServiceServer{},
		RegisterFunc:      func(s *grpc.Server, impl any) {},
	}

	rootCmd, err := protocli.RootCommand("testapp",
		protocli.Service(serviceCLI),
		protocli.WithConfigManagementCommands(configProto, "testapp", "userservice"),
		protocli.WithGlobalConfigPath(globalConfigPath),
		protocli.WithLocalConfigPath(localConfigPath),
	)
	require.NoError(t, err)

	runCommand := func(args ...string) (string, error) {
		var buf bytes.Buffer
		rootCmd.Writer = &buf
		for _, cmd := range rootCmd.Commands {
			cmd.Writer = &buf
			setWriterRecursive(cmd, &buf)
		}
		err := rootCmd.Run(context.Background(), append([]string{"testapp"}, args...))
		return buf.String(), err
	}

	// Set global config
	output, err := runCommand("config", "set", "--global",
		"databaseUrl=global-db",
		"maxConnections=100")
	require.NoError(t, err)
	require.Contains(t, output, globalConfigPath)

	// Set local config (override one value)
	output, err = runCommand("config", "set",
		"databaseUrl=local-db")
	require.NoError(t, err)
	require.Contains(t, output, localConfigPath)

	// Verify local overrides global
	output, err = runCommand("config", "get", "databaseUrl")
	require.NoError(t, err)
	require.Contains(t, output, "local-db")
	require.Contains(t, output, localConfigPath)

	// Verify global value used when not in local
	output, err = runCommand("config", "get", "maxConnections")
	require.NoError(t, err)
	require.Contains(t, output, "100")
	require.Contains(t, output, globalConfigPath)

	// List should show both
	output, err = runCommand("config", "list")
	require.NoError(t, err)

	// Check for values and their sources
	lines := strings.Split(output, "\n")
	var dbLine, maxConnLine string
	for _, line := range lines {
		if strings.Contains(line, "databaseUrl:") && strings.Contains(line, "local-db") {
			dbLine = line
		}
		if strings.Contains(line, "maxConnections:") && strings.Contains(line, "100") {
			maxConnLine = line
		}
	}

	require.Contains(t, dbLine, "local-db")
	require.Contains(t, dbLine, localConfigPath)
	require.Contains(t, maxConnLine, "100")
	require.Contains(t, maxConnLine, globalConfigPath)
}

// TestIntegration_ConfigManagement_WithoutConfig tests that config commands are not added
// when WithConfigManagementCommands is not called
func TestIntegration_ConfigManagement_WithoutConfig(t *testing.T) {
	configProto := &simple.UserServiceConfig{}
	serviceCLI := &protocli.ServiceCLI{
		Command: &cli.Command{
			Name:  "userservice",
			Usage: "User service commands",
		},
		ServiceName:       "userservice",
		ConfigMessageType: "example.UserServiceConfig",
		ConfigPrototype:   configProto,
		FactoryOrImpl:     &simple.UnimplementedUserServiceServer{},
		RegisterFunc:      func(s *grpc.Server, impl any) {},
	}

	// Create root command WITHOUT config management
	rootCmd, err := protocli.RootCommand("testapp",
		protocli.Service(serviceCLI),
	)
	require.NoError(t, err)

	var buf bytes.Buffer
	rootCmd.Writer = &buf
	err = rootCmd.Run(context.Background(), []string{"testapp", "--help"})
	require.NoError(t, err)

	output := buf.String()
	// Check that "config" command doesn't appear in COMMANDS section
	// (The word "config" may appear in --config flag, so be specific)
	require.NotContains(t, output, "COMMANDS:\n   config")
}

// setWriterRecursive sets the writer on all subcommands recursively
func setWriterRecursive(cmd *cli.Command, w *bytes.Buffer) {
	cmd.Writer = w
	for _, subCmd := range cmd.Commands {
		setWriterRecursive(subCmd, w)
	}
}
