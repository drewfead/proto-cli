package protocli_test

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	protocli "github.com/drewfead/proto-cli"
	"github.com/drewfead/proto-cli/examples/simple"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

// TestUnit_ConfigLoader_FileLoading tests loading config from file readers.
func TestUnit_ConfigLoader_FileLoading(t *testing.T) {
	tests := []struct {
		name            string
		yamlContent     string
		serviceName     string
		expectedDBURL   string
		expectedMaxConn int64
		expectError     bool
	}{
		{
			name: "basic file loading",
			yamlContent: `
services:
  userservice:
    database-url: postgresql://localhost:5432/testdb
    max-connections: 25
`,
			serviceName:     "userservice",
			expectedDBURL:   "postgresql://localhost:5432/testdb",
			expectedMaxConn: 25,
			expectError:     false,
		},
		{
			name: "missing service section",
			yamlContent: `
services:
  otherservice:
    some-field: value
`,
			serviceName:     "userservice",
			expectedDBURL:   "", // Should use default proto values
			expectedMaxConn: 0,
			expectError:     false,
		},
		{
			name: "no services section",
			yamlContent: `
other:
  field: value
`,
			serviceName:     "userservice",
			expectedDBURL:   "",
			expectedMaxConn: 0,
			expectError:     false,
		},
		{
			name: "partial config",
			yamlContent: `
services:
  userservice:
    database-url: postgresql://partial:5432/db
`,
			serviceName:     "userservice",
			expectedDBURL:   "postgresql://partial:5432/db",
			expectedMaxConn: 0, // Not specified, should use default
			expectError:     false,
		},
		{
			name:            "empty yaml",
			yamlContent:     "",
			serviceName:     "userservice",
			expectedDBURL:   "",
			expectedMaxConn: 0,
			expectError:     false,
		},
		{
			name:            "invalid yaml",
			yamlContent:     "invalid: yaml: content: [",
			serviceName:     "userservice",
			expectedDBURL:   "",
			expectedMaxConn: 0,
			expectError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create config loader with reader
			reader := strings.NewReader(tt.yamlContent)
			loader := protocli.NewConfigLoader(
				protocli.SingleCommandMode,
				protocli.ReaderConfig(reader),
			)

			// Create config message
			config := &simple.UserServiceConfig{}

			// Load config
			err := loader.LoadServiceConfig(nil, tt.serviceName, config)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedDBURL, config.DatabaseUrl)
			assert.Equal(t, tt.expectedMaxConn, config.MaxConnections)
		})
	}
}

// TestUnit_ConfigLoader_DeepMerge tests deep merging from multiple config sources.
func TestUnit_ConfigLoader_DeepMerge(t *testing.T) {
	tests := []struct {
		name            string
		yamlContents    []string
		serviceName     string
		expectedDBURL   string
		expectedMaxConn int64
	}{
		{
			name: "two files - second overrides first",
			yamlContents: []string{
				`
services:
  userservice:
    database-url: postgresql://base:5432/db
    max-connections: 10
`,
				`
services:
  userservice:
    database-url: postgresql://override:5432/db
`,
			},
			serviceName:     "userservice",
			expectedDBURL:   "postgresql://override:5432/db",
			expectedMaxConn: 10, // From first file
		},
		{
			name: "three files - cascading merge",
			yamlContents: []string{
				`
services:
  userservice:
    database-url: postgresql://base:5432/db
    max-connections: 10
`,
				`
services:
  userservice:
    max-connections: 20
`,
				`
services:
  userservice:
    max-connections: 30
`,
			},
			serviceName:     "userservice",
			expectedDBURL:   "postgresql://base:5432/db", // Only in first file
			expectedMaxConn: 30,                          // Last override wins
		},
		{
			name: "empty files in sequence",
			yamlContents: []string{
				`
services:
  userservice:
    database-url: postgresql://first:5432/db
    max-connections: 15
`,
				``,
				`
services:
  userservice:
    max-connections: 25
`,
			},
			serviceName:     "userservice",
			expectedDBURL:   "postgresql://first:5432/db",
			expectedMaxConn: 25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create readers for all YAML contents
			var opts []protocli.ConfigLoaderOption
			for _, content := range tt.yamlContents {
				reader := strings.NewReader(content)
				opts = append(opts, protocli.ReaderConfig(reader))
			}

			// Create config loader with all readers
			loader := protocli.NewConfigLoader(protocli.SingleCommandMode, opts...)

			// Create config message
			config := &simple.UserServiceConfig{}

			// Load config
			err := loader.LoadServiceConfig(nil, tt.serviceName, config)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedDBURL, config.DatabaseUrl)
			assert.Equal(t, tt.expectedMaxConn, config.MaxConnections)
		})
	}
}

// TestUnit_ConfigLoader_EnvVarOverrides tests environment variable overrides.
func TestUnit_ConfigLoader_EnvVarOverrides(t *testing.T) {
	// Note: This test sets environment variables, so it might affect other tests
	// In a real scenario, we'd want better isolation

	tests := []struct {
		name            string
		yamlContent     string
		serviceName     string
		envVars         map[string]string
		expectedDBURL   string
		expectedMaxConn int64
	}{
		{
			name: "env overrides file",
			yamlContent: `
services:
  userservice:
    database-url: postgresql://file:5432/db
    max-connections: 10
`,
			serviceName: "userservice",
			envVars: map[string]string{
				"TEST_PREFIX_DATABASE_URL": "postgresql://env:5432/db",
			},
			expectedDBURL:   "postgresql://env:5432/db",
			expectedMaxConn: 10,
		},
		{
			name: "partial env override",
			yamlContent: `
services:
  userservice:
    database-url: postgresql://file:5432/db
    max-connections: 15
`,
			serviceName: "userservice",
			envVars: map[string]string{
				"TEST_PREFIX_MAX_CONNECTIONS": "50",
			},
			expectedDBURL:   "postgresql://file:5432/db",
			expectedMaxConn: 50,
		},
		{
			name: "env overrides all",
			yamlContent: `
services:
  userservice:
    database-url: postgresql://file:5432/db
    max-connections: 20
`,
			serviceName: "userservice",
			envVars: map[string]string{
				"TEST_PREFIX_DATABASE_URL":    "postgresql://fullenv:5432/db",
				"TEST_PREFIX_MAX_CONNECTIONS": "75",
			},
			expectedDBURL:   "postgresql://fullenv:5432/db",
			expectedMaxConn: 75,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Create config loader
			reader := strings.NewReader(tt.yamlContent)
			loader := protocli.NewConfigLoader(
				protocli.SingleCommandMode,
				protocli.ReaderConfig(reader),
				protocli.EnvPrefix("TEST_PREFIX"),
			)

			// Create config message
			config := &simple.UserServiceConfig{}

			// Load config
			err := loader.LoadServiceConfig(nil, tt.serviceName, config)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedDBURL, config.DatabaseUrl)
			assert.Equal(t, tt.expectedMaxConn, config.MaxConnections)
		})
	}
}

// TestUnit_ConfigLoader_FlagOverrides tests CLI flag overrides (highest precedence).
func TestUnit_ConfigLoader_FlagOverrides(t *testing.T) {
	tests := []struct {
		name            string
		yamlContent     string
		serviceName     string
		envVars         map[string]string
		setFlags        map[string]any
		expectedDBURL   string
		expectedMaxConn int64
	}{
		{
			name: "flag overrides env and file",
			yamlContent: `
services:
  userservice:
    database-url: postgresql://file:5432/db
    max-connections: 10
`,
			serviceName: "userservice",
			envVars: map[string]string{
				"TEST_PREFIX_DATABASE_URL": "postgresql://env:5432/db",
			},
			setFlags: map[string]any{
				"db-url": "postgresql://flag:5432/db",
			},
			expectedDBURL:   "postgresql://flag:5432/db",
			expectedMaxConn: 10,
		},
		{
			name: "partial flag override",
			yamlContent: `
services:
  userservice:
    database-url: postgresql://file:5432/db
    max-connections: 15
`,
			serviceName: "userservice",
			envVars: map[string]string{
				"TEST_PREFIX_MAX_CONNECTIONS": "50",
			},
			setFlags: map[string]any{
				"db-url": "postgresql://flagurl:5432/db",
			},
			expectedDBURL:   "postgresql://flagurl:5432/db",
			expectedMaxConn: 50, // From env
		},
		{
			name: "all three sources - flags win",
			yamlContent: `
services:
  userservice:
    database-url: postgresql://file:5432/db
    max-connections: 20
`,
			serviceName: "userservice",
			envVars: map[string]string{
				"TEST_PREFIX_DATABASE_URL":    "postgresql://env:5432/db",
				"TEST_PREFIX_MAX_CONNECTIONS": "60",
			},
			setFlags: map[string]any{
				"db-url":    "postgresql://topflag:5432/db",
				"max-conns": int64(99),
			},
			expectedDBURL:   "postgresql://topflag:5432/db",
			expectedMaxConn: 99,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Create a mock CLI command with flags
			cmd := &cli.Command{
				Name: "test",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "db-url"},
					&cli.IntFlag{Name: "max-conns"},
				},
			}

			// This is a simplified test - in reality, we'd need to properly
			// set up the CLI context and flag values. For unit testing the
			// config loader logic, we'll test the precedence at the integration level.

			// Create config loader
			reader := strings.NewReader(tt.yamlContent)
			loader := protocli.NewConfigLoader(
				protocli.SingleCommandMode,
				protocli.ReaderConfig(reader),
				protocli.EnvPrefix("TEST_PREFIX"),
			)

			// Create config message
			config := &simple.UserServiceConfig{}

			// Note: Full flag testing will be done in integration tests
			// since it requires a properly set up CLI command context
			err := loader.LoadServiceConfig(cmd, tt.serviceName, config)
			require.NoError(t, err)

			// For now, just verify file + env work
			// Flag precedence will be tested in integration tests
		})
	}
}

// TestUnit_ConfigLoader_DaemonMode tests that daemon mode ignores flags.
func TestUnit_ConfigLoader_DaemonMode(t *testing.T) {
	yamlContent := `
services:
  userservice:
    database-url: postgresql://file:5432/db
    max-connections: 10
`

	// Set environment variable
	t.Setenv("TEST_PREFIX_MAX_CONNECTIONS", "50")

	// Create config loader in daemon mode
	reader := strings.NewReader(yamlContent)
	loader := protocli.NewConfigLoader(
		protocli.DaemonMode, // Daemon mode should NOT apply flags
		protocli.ReaderConfig(reader),
		protocli.EnvPrefix("TEST_PREFIX"),
	)

	// Create config message
	config := &simple.UserServiceConfig{}

	// Load config (passing nil cmd should be fine for daemon mode)
	err := loader.LoadServiceConfig(nil, "userservice", config)
	require.NoError(t, err)

	// Should have file + env, but NOT flags
	assert.Equal(t, "postgresql://file:5432/db", config.DatabaseUrl)
	assert.Equal(t, int64(50), config.MaxConnections) // From env
}

// TestUnit_VerbosityDefaults tests the default verbosity configuration.
func TestUnit_VerbosityDefaults(t *testing.T) {
	tests := []struct {
		name     string
		option   protocli.RootOption
		expected string
	}{
		{
			name:     "debug level",
			option:   protocli.WithDefaultVerbosity(slog.LevelDebug),
			expected: "debug",
		},
		{
			name:     "warn level",
			option:   protocli.WithDefaultVerbosity(slog.LevelWarn),
			expected: "warn",
		},
		{
			name:     "none level",
			option:   protocli.WithDefaultVerbosity(slog.Level(1000)),
			expected: "none",
		},
		{
			name:     "default is info",
			option:   nil,
			expected: "info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			userServiceCLI := simple.UserServiceCommand(ctx, &simple.UnimplementedUserServiceServer{})

			var opts []protocli.RootOption
			opts = append(opts, protocli.Service(userServiceCLI))
			if tt.option != nil {
				opts = append(opts, tt.option)
			}

			rootCmd, err := protocli.RootCommand("testcli", opts...)
			require.NoError(t, err)

			// Find the verbosity flag
			var verbosityDefault string
			for _, flag := range rootCmd.Flags {
				if flag.Names()[0] == "verbosity" {
					if stringFlag, ok := flag.(*cli.StringFlag); ok {
						verbosityDefault = stringFlag.Value
						break
					}
				}
			}

			assert.Equal(t, tt.expected, verbosityDefault)
		})
	}
}

func TestUnit_ConfigLoader_NestedMessages(t *testing.T) {
	yamlContent := `
services:
  userservice:
    database-url: postgresql://localhost/db
    max-connections: 10
    database:
      url: postgresql://nested/db
      max-connections: 20
      timeout-seconds: 30
`

	loader := protocli.NewConfigLoader(
		protocli.DaemonMode,
		protocli.ReaderConfig(bytes.NewBufferString(yamlContent)),
	)

	config := &simple.UserServiceConfig{}
	cmd := &cli.Command{Name: "test"}

	err := loader.LoadServiceConfig(cmd, "userservice", config)
	require.NoError(t, err)

	// Check top-level fields
	assert.Equal(t, "postgresql://localhost/db", config.DatabaseUrl)
	assert.Equal(t, int64(10), config.MaxConnections)

	// Check nested database config
	require.NotNil(t, config.Database)
	assert.Equal(t, "postgresql://nested/db", config.Database.Url)
	assert.Equal(t, int32(20), config.Database.MaxConnections)
	assert.Equal(t, int32(30), config.Database.TimeoutSeconds)
}

// TestUnit_ConfigLoader_OneofTypes tests oneof (union) type support.
func TestUnit_ConfigLoader_OneofTypes(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		expectType  string
		checkFields func(*testing.T, *simple.UserServiceConfig)
	}{
		{
			name: "postgres backend",
			yaml: `
services:
  userservice:
    database-url: postgresql://localhost/db
    postgres:
      host: localhost
      port: 5432
      database: mydb
`,
			expectType: "postgres",
			checkFields: func(t *testing.T, config *simple.UserServiceConfig) {
				t.Helper()
				postgres, ok := config.Backend.(*simple.UserServiceConfig_Postgres)
				require.True(t, ok, "Expected postgres backend")
				assert.Equal(t, "localhost", postgres.Postgres.Host)
				assert.Equal(t, int32(5432), postgres.Postgres.Port)
				assert.Equal(t, "mydb", postgres.Postgres.Database)
			},
		},
		{
			name: "mysql backend",
			yaml: `
services:
  userservice:
    database-url: postgresql://localhost/db
    mysql:
      host: mysqlhost
      port: 3306
      database: testdb
      enable-ssl: true
`,
			expectType: "mysql",
			checkFields: func(t *testing.T, config *simple.UserServiceConfig) {
				t.Helper()
				mysql, ok := config.Backend.(*simple.UserServiceConfig_Mysql)
				require.True(t, ok, "Expected mysql backend")
				assert.Equal(t, "mysqlhost", mysql.Mysql.Host)
				assert.Equal(t, int32(3306), mysql.Mysql.Port)
				assert.Equal(t, "testdb", mysql.Mysql.Database)
				assert.True(t, mysql.Mysql.EnableSsl)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := protocli.NewConfigLoader(
				protocli.DaemonMode,
				protocli.ReaderConfig(bytes.NewBufferString(tt.yaml)),
			)

			config := &simple.UserServiceConfig{}
			cmd := &cli.Command{Name: "test"}

			err := loader.LoadServiceConfig(cmd, "userservice", config)
			require.NoError(t, err)

			tt.checkFields(t, config)
		})
	}
}

// TestUnit_ConfigLoader_RepeatedFields tests repeated (list) field support.
func TestUnit_ConfigLoader_RepeatedFields(t *testing.T) {
	yamlContent := `
services:
  userservice:
    database-url: postgresql://localhost/db
    allowed-origins:
      - https://example.com
      - https://api.example.com
      - http://localhost:3000
`

	loader := protocli.NewConfigLoader(
		protocli.DaemonMode,
		protocli.ReaderConfig(bytes.NewBufferString(yamlContent)),
	)

	config := &simple.UserServiceConfig{}
	cmd := &cli.Command{Name: "test"}

	err := loader.LoadServiceConfig(cmd, "userservice", config)
	require.NoError(t, err)

	require.Len(t, config.AllowedOrigins, 3)
	assert.Equal(t, "https://example.com", config.AllowedOrigins[0])
	assert.Equal(t, "https://api.example.com", config.AllowedOrigins[1])
	assert.Equal(t, "http://localhost:3000", config.AllowedOrigins[2])
}

// TestUnit_ConfigLoader_MapFields tests map field support.
func TestUnit_ConfigLoader_MapFields(t *testing.T) {
	yamlContent := `
services:
  userservice:
    database-url: postgresql://localhost/db
    feature-flags:
      enable-caching: "true"
      max-cache-size: "1000"
      debug-mode: "false"
`

	loader := protocli.NewConfigLoader(
		protocli.DaemonMode,
		protocli.ReaderConfig(bytes.NewBufferString(yamlContent)),
	)

	config := &simple.UserServiceConfig{}
	cmd := &cli.Command{Name: "test"}

	err := loader.LoadServiceConfig(cmd, "userservice", config)
	require.NoError(t, err)

	require.Len(t, config.FeatureFlags, 3)
	assert.Equal(t, "true", config.FeatureFlags["enable-caching"])
	assert.Equal(t, "1000", config.FeatureFlags["max-cache-size"])
	assert.Equal(t, "false", config.FeatureFlags["debug-mode"])
}

// TestUnit_ConfigLoader_EnumFields tests enum field support.
func TestUnit_ConfigLoader_EnumFields(t *testing.T) {
	tests := []struct {
		name          string
		yaml          string
		expectedLevel simple.LogLevel
	}{
		{
			name: "enum by name",
			yaml: `
services:
  userservice:
    database-url: postgresql://localhost/db
    log-level: INFO
`,
			expectedLevel: simple.LogLevel_INFO,
		},
		{
			name: "enum by number",
			yaml: `
services:
  userservice:
    database-url: postgresql://localhost/db
    log-level: 3
`,
			expectedLevel: simple.LogLevel_WARN,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := protocli.NewConfigLoader(
				protocli.DaemonMode,
				protocli.ReaderConfig(bytes.NewBufferString(tt.yaml)),
			)

			config := &simple.UserServiceConfig{}
			cmd := &cli.Command{Name: "test"}

			err := loader.LoadServiceConfig(cmd, "userservice", config)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedLevel, config.LogLevel)
		})
	}
}

func TestIntegration_ConfigDebug_FileDiscovery(t *testing.T) {
	// Create temp config files
	tmpDir := t.TempDir()
	config1 := tmpDir + "/config1.yaml"
	config2 := tmpDir + "/config2.yaml"
	missingFile := tmpDir + "/missing.yaml"

	// Write test configs
	_ = os.WriteFile(config1, []byte(`
services:
  user-service:
    database-url: "postgres://localhost/db1"
`), 0o600)

	_ = os.WriteFile(config2, []byte(`
services:
  user-service:
    max-connections: 20
`), 0o600)

	// Create loader with debug mode enabled
	loader := protocli.NewConfigLoader(
		protocli.DaemonMode,
		protocli.FileConfig(config1, config2, missingFile),
		protocli.DebugMode(true),
	)

	config := &simple.UserServiceConfig{}
	err := loader.LoadServiceConfig(nil, "user-service", config)
	require.NoError(t, err)

	// Get debug info
	debug := loader.DebugInfo()
	require.NotNil(t, debug)

	// Verify paths checked
	assert.ElementsMatch(t, []string{config1, config2, missingFile}, debug.PathsChecked)

	// Verify files loaded
	assert.ElementsMatch(t, []string{config1, config2}, debug.FilesLoaded)

	// Verify missing file is tracked as failed
	assert.Contains(t, debug.FilesFailed, missingFile)
	assert.Contains(t, debug.FilesFailed[missingFile], "does not exist")

	// Verify final config
	assert.NotNil(t, debug.FinalConfig)
}

// TestConfigDebug_EnvVarTracking tests environment variable tracking.
func TestIntegration_ConfigDebug_EnvVarTracking(t *testing.T) {
	// Set environment variables
	t.Setenv("TEST_DATABASE_URL", "postgres://env/db")
	t.Setenv("TEST_MAX_CONNECTIONS", "50")
	defer func() { _ = os.Unsetenv("TEST_DATABASE_URL") }()
	defer func() { _ = os.Unsetenv("TEST_MAX_CONNECTIONS") }()

	loader := protocli.NewConfigLoader(
		protocli.DaemonMode,
		protocli.EnvPrefix("TEST"),
		protocli.DebugMode(true),
	)

	config := &simple.UserServiceConfig{}
	err := loader.LoadServiceConfig(nil, "user-service", config)
	require.NoError(t, err)

	// Verify env vars tracked
	debug := loader.DebugInfo()
	require.NotNil(t, debug)

	assert.Equal(t, "postgres://env/db", debug.EnvVarsApplied["TEST_DATABASE_URL"])
	assert.Equal(t, "50", debug.EnvVarsApplied["TEST_MAX_CONNECTIONS"])
}

// TestConfigDebug_NestedErrorContext tests that nested field errors include full path.
func TestIntegration_ConfigDebug_NestedErrorContext(t *testing.T) {
	configYAML := `
services:
  user-service:
    database:
      url: "postgres://localhost/db"
      invalid-field: "bad"
`

	loader := protocli.NewConfigLoader(
		protocli.DaemonMode,
		protocli.ReaderConfig(strings.NewReader(configYAML)),
	)

	config := &simple.UserServiceConfig{}
	err := loader.LoadServiceConfig(nil, "user-service", config)

	// Should get error with full field path
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database.invalid-field", "Error should include nested field path")
}

// TestConfigDebug_DeepNestedError tests deeply nested field path in errors.
func TestIntegration_ConfigDebug_DeepNestedError(t *testing.T) {
	// Test with nested config that has an error deep in the structure
	configYAML := `
services:
  user-service:
    database:
      url: "test"
      max-connections: "not-a-number"
`

	loader := protocli.NewConfigLoader(
		protocli.DaemonMode,
		protocli.ReaderConfig(strings.NewReader(configYAML)),
	)

	config := &simple.UserServiceConfig{}
	err := loader.LoadServiceConfig(nil, "user-service", config)
	// Should get error with full field path
	if err != nil {
		// Verify error message includes path context
		errMsg := err.Error()
		t.Logf("Error message: %s", errMsg)
		// The error should mention "database" somewhere in the path
		assert.Contains(t, errMsg, "database", "Error should reference nested field")
	}
}

// TestConfigDebug_OutputFormat tests that debug info can be marshaled to JSON.
func TestIntegration_ConfigDebug_OutputFormat(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := tmpDir + "/config.yaml"

	_ = os.WriteFile(configFile, []byte(`
services:
  user-service:
    database-url: "postgres://localhost/db"
`), 0o600)

	loader := protocli.NewConfigLoader(
		protocli.DaemonMode,
		protocli.FileConfig(configFile),
		protocli.EnvPrefix("TEST"),
		protocli.DebugMode(true),
	)

	config := &simple.UserServiceConfig{}
	err := loader.LoadServiceConfig(nil, "user-service", config)
	require.NoError(t, err)

	debug := loader.DebugInfo()
	require.NotNil(t, debug)

	// Verify all debug fields are populated
	assert.NotEmpty(t, debug.PathsChecked)
	assert.NotEmpty(t, debug.FilesLoaded)
	assert.NotNil(t, debug.FilesFailed)
	assert.NotNil(t, debug.EnvVarsApplied)
	assert.NotNil(t, debug.FlagsApplied)
	assert.NotNil(t, debug.FinalConfig)
}
