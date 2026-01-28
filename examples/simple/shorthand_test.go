package simple_test

import (
	"context"
	"testing"

	simple "github.com/drewfead/proto-cli/examples/simple"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	cli "github.com/urfave/cli/v3"
)

// Check for Aliases field.
type aliased interface {
	Names() []string
}

// TestShorthand_FlagsPresent tests that shorthand flags are generated.
func TestShorthand_FlagsPresent(t *testing.T) {
	ctx := context.Background()

	serviceCLI := simple.UserServiceCommand(
		ctx,
		newUserService,
	)

	require.NotNil(t, serviceCLI)
	require.NotNil(t, serviceCLI.Command.Commands)

	// Get the "get" command
	var getCmd *cli.Command
	for _, cmd := range serviceCLI.Command.Commands {
		if cmd.Name == "get" {
			getCmd = cmd
			break
		}
	}

	require.NotNil(t, getCmd, "Should find 'get' command")

	// Find the id flag
	var idFlag any
	for _, flag := range getCmd.Flags {
		name := getFlagName(flag)
		if name == "id" {
			idFlag = flag
			break
		}
	}

	require.NotNil(t, idFlag, "Should find 'id' flag")

	if af, ok := idFlag.(aliased); ok {
		names := af.Names()
		// Should have both "id" and "i" (shorthand)
		assert.Contains(t, names, "id", "Should have full flag name")
		assert.Contains(t, names, "i", "Should have shorthand 'i'")
	} else {
		t.Fatal("Flag doesn't implement Names() method")
	}
}

// TestShorthand_CreateCommandFlags tests shorthands on create command.
func TestShorthand_CreateCommandFlags(t *testing.T) {
	ctx := context.Background()

	serviceCLI := simple.UserServiceCommand(
		ctx,
		newUserService,
	)

	require.NotNil(t, serviceCLI)
	require.NotNil(t, serviceCLI.Command.Commands)

	// Get the "create" command
	var createCmd *cli.Command
	for _, cmd := range serviceCLI.Command.Commands {
		if cmd.Name == "create" {
			createCmd = cmd
			break
		}
	}

	require.NotNil(t, createCmd, "Should find 'create' command")

	shorthands := map[string]string{
		"name":  "n",
		"email": "e",
	}

	for flagName, expectedShorthand := range shorthands {
		var foundFlag any
		for _, flag := range createCmd.Flags {
			name := getFlagName(flag)
			if name == flagName {
				foundFlag = flag
				break
			}
		}

		require.NotNil(t, foundFlag, "Should find '%s' flag", flagName)

		if af, ok := foundFlag.(aliased); ok {
			names := af.Names()
			assert.Contains(t, names, flagName, "Should have full flag name '%s'", flagName)
			assert.Contains(t, names, expectedShorthand, "Should have shorthand '%s' for %s", expectedShorthand, flagName)
		} else {
			t.Fatalf("Flag '%s' doesn't implement Names() method", flagName)
		}
	}
}
