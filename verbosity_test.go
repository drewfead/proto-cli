package protocli_test

import (
	"context"
	"log/slog"
	"testing"

	protocli "github.com/drewfead/proto-cli"
	"github.com/drewfead/proto-cli/examples/simple"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

// TestDefaultVerbosity_DebugLevel tests setting debug as default verbosity level.
func TestDefaultVerbosity_DebugLevel(t *testing.T) {
	ctx := context.Background()

	// Create a service CLI
	userServiceCLI := simple.UserServiceCommand(ctx, &simple.UnimplementedUserServiceServer{})

	// Create root command with debug as default verbosity
	rootCmd, err := protocli.RootCommand("testcli",
		protocli.Service(userServiceCLI),
		protocli.WithDefaultVerbosity(slog.LevelDebug),
	)
	require.NoError(t, err)

	// Find the verbosity flag and check its default value
	var verbosityDefault string
	for _, flag := range rootCmd.Flags {
		if flag.Names()[0] == "verbosity" {
			// urfave/cli v3 uses StringFlag with Value field
			if stringFlag, ok := flag.(*cli.StringFlag); ok {
				verbosityDefault = stringFlag.Value
				break
			}
		}
	}

	assert.Equal(t, "debug", verbosityDefault, "default verbosity should be 'debug'")
}

// TestDefaultVerbosity_WarnLevel tests setting warn as default verbosity level.
func TestDefaultVerbosity_WarnLevel(t *testing.T) {
	ctx := context.Background()

	// Create a service CLI
	userServiceCLI := simple.UserServiceCommand(ctx, &simple.UnimplementedUserServiceServer{})

	// Create root command with warn as default verbosity
	rootCmd, err := protocli.RootCommand("testcli",
		protocli.Service(userServiceCLI),
		protocli.WithDefaultVerbosity(slog.LevelWarn),
	)
	require.NoError(t, err)

	// Find the verbosity flag and check its default value
	var verbosityDefault string
	for _, flag := range rootCmd.Flags {
		if flag.Names()[0] == "verbosity" {
			if stringFlag, ok := flag.(*cli.StringFlag); ok {
				verbosityDefault = stringFlag.Value
				break
			}
		}
	}

	assert.Equal(t, "warn", verbosityDefault, "default verbosity should be 'warn'")
}

// TestDefaultVerbosity_NoneLevel tests disabling logging by default.
func TestDefaultVerbosity_NoneLevel(t *testing.T) {
	ctx := context.Background()

	// Create a service CLI
	userServiceCLI := simple.UserServiceCommand(ctx, &simple.UnimplementedUserServiceServer{})

	// Create root command with logging disabled (level >= 1000)
	rootCmd, err := protocli.RootCommand("testcli",
		protocli.Service(userServiceCLI),
		protocli.WithDefaultVerbosity(slog.Level(1000)),
	)
	require.NoError(t, err)

	// Find the verbosity flag and check its default value
	var verbosityDefault string
	for _, flag := range rootCmd.Flags {
		if flag.Names()[0] == "verbosity" {
			if stringFlag, ok := flag.(*cli.StringFlag); ok {
				verbosityDefault = stringFlag.Value
				break
			}
		}
	}

	assert.Equal(t, "none", verbosityDefault, "default verbosity should be 'none'")
}

// TestDefaultVerbosity_DefaultIsInfo tests that default verbosity is "info" when not specified.
func TestDefaultVerbosity_DefaultIsInfo(t *testing.T) {
	ctx := context.Background()

	// Create a service CLI
	userServiceCLI := simple.UserServiceCommand(ctx, &simple.UnimplementedUserServiceServer{})

	// Create root command WITHOUT custom default verbosity
	rootCmd, err := protocli.RootCommand("testcli",
		protocli.Service(userServiceCLI),
	)
	require.NoError(t, err)

	// Find the verbosity flag and check its default value
	var verbosityDefault string
	for _, flag := range rootCmd.Flags {
		if flag.Names()[0] == "verbosity" {
			if stringFlag, ok := flag.(*cli.StringFlag); ok {
				verbosityDefault = stringFlag.Value
				break
			}
		}
	}

	assert.Equal(t, "info", verbosityDefault, "default verbosity should be 'info' when not specified")
}
