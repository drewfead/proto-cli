package protocli_test

import (
	"context"
	"strings"
	"testing"

	protocli "github.com/drewfead/proto-cli"
	"github.com/drewfead/proto-cli/examples/simple"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHoistedService_FlatCommandStructure tests that hoisted services have commands at root level.
func TestHoistedService_FlatCommandStructure(t *testing.T) {
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
func TestHoistedService_NamingCollision(t *testing.T) {
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
func TestHoistedService_DaemonizeCollision(t *testing.T) {
	// This test would require a service with an RPC named "daemonize" to properly test
	// For now, we'll document that the collision detection exists at root.go:150
	t.Skip("Would need a service with a 'daemonize' RPC to test this collision")
}
