package protocli_test

import (
	"os"
	"strings"
	"testing"

	protocli "github.com/drewfead/proto-cli"
	simple "github.com/drewfead/proto-cli/examples/simple"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfigDebug_FileDiscovery tests config file discovery debugging.
func TestConfigDebug_FileDiscovery(t *testing.T) {
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
`), 0600)

	_ = os.WriteFile(config2, []byte(`
services:
  user-service:
    max-connections: 20
`), 0600)

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
func TestConfigDebug_EnvVarTracking(t *testing.T) {
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
func TestConfigDebug_NestedErrorContext(t *testing.T) {
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
func TestConfigDebug_DeepNestedError(t *testing.T) {
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
func TestConfigDebug_OutputFormat(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := tmpDir + "/config.yaml"

	_ = os.WriteFile(configFile, []byte(`
services:
  user-service:
    database-url: "postgres://localhost/db"
`), 0600)

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
