package simple_test

import (
	"context"
	"testing"

	simple "github.com/drewfead/proto-cli/examples/simple"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	cli "github.com/urfave/cli/v3"
)

// TestNaming_ServiceKebabCase tests that service names use kebab-case by default.
func TestNaming_ServiceKebabCase(t *testing.T) {
	ctx := context.Background()

	serviceCLI := simple.UserServiceServiceCommand(
		ctx,
		newUserService,
	)

	require.NotNil(t, serviceCLI)
	require.NotNil(t, serviceCLI.Command)

	// Verify service name uses kebab-case: UserService -> user-service
	assert.Equal(t, "user-service", serviceCLI.Command.Name)
	assert.Equal(t, "user-service", serviceCLI.ServiceName)
}

// TestNaming_CommandOverride tests that command names can be overridden.
func TestNaming_CommandOverride(t *testing.T) {
	ctx := context.Background()

	serviceCLI := simple.UserServiceServiceCommand(
		ctx,
		newUserService,
	)

	require.NotNil(t, serviceCLI)
	require.NotNil(t, serviceCLI.Command.Commands)

	// Verify GetUser command uses annotation: ["user", "get"] -> "get"
	var getCmd, createCmd *cli.Command
	for _, cmd := range serviceCLI.Command.Commands {
		if cmd.Name == "get" {
			getCmd = cmd
		}
		if cmd.Name == "create" {
			createCmd = cmd
		}
	}

	require.NotNil(t, getCmd, "Should find 'get' command from annotation")
	require.NotNil(t, createCmd, "Should find 'create' command from annotation")

	// Verify descriptions from annotations
	assert.Equal(t, "Retrieve a user by ID", getCmd.Usage)
	assert.Equal(t, "Create a new user", createCmd.Usage)
}

// TestNaming_FlagKebabCase tests that flag names use kebab-case by default.
func TestNaming_FlagKebabCase(t *testing.T) {
	ctx := context.Background()

	serviceCLI := simple.UserServiceServiceCommand(
		ctx,
		newUserService,
	)

	require.NotNil(t, serviceCLI)
	require.NotNil(t, serviceCLI.Command.Commands)

	// Get the create command
	var createCmd *cli.Command
	for _, cmd := range serviceCLI.Command.Commands {
		if cmd.Name == "create" {
			createCmd = cmd
			break
		}
	}

	require.NotNil(t, createCmd)

	// Check that message field flags use kebab-case
	var hasRegistrationDate, hasAddress bool
	for _, flag := range createCmd.Flags {
		name := getFlagName(flag)
		if name == "registration-date" {
			hasRegistrationDate = true
		}
		if name == "address" {
			hasAddress = true
		}
	}

	assert.True(t, hasRegistrationDate, "Should have 'registration-date' flag (kebab-case for RegistrationDate)")
	assert.True(t, hasAddress, "Should have 'address' flag")
}

// TestNaming_FlagOverride tests that flag names can be overridden with annotations.
func TestNaming_FlagOverride(t *testing.T) {
	ctx := context.Background()

	serviceCLI := simple.UserServiceServiceCommand(
		ctx,
		newUserService,
	)

	require.NotNil(t, serviceCLI)
	require.NotNil(t, serviceCLI.Command.Commands)

	// Get the create command
	var createCmd *cli.Command
	for _, cmd := range serviceCLI.Command.Commands {
		if cmd.Name == "create" {
			createCmd = cmd
			break
		}
	}

	require.NotNil(t, createCmd)

	// Verify custom flag names from annotations exist
	var hasName, hasEmail bool
	for _, flag := range createCmd.Flags {
		name := getFlagName(flag)
		if name == "name" {
			hasName = true
		}
		if name == "email" {
			hasEmail = true
		}
	}

	// These are explicitly set in the proto with [(cli.flag) = {name: "name"}]
	assert.True(t, hasName, "Should have 'name' flag from annotation")
	assert.True(t, hasEmail, "Should have 'email' flag from annotation")
}

// Helper to extract flag name from cli.Flag interface.
func getFlagName(flag any) string {
	if n, ok := flag.(aliased); ok {
		names := n.Names()
		if len(names) > 0 {
			return names[0]
		}
	}

	return ""
}
