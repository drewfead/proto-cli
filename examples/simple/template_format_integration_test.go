package simple_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"text/template"

	protocli "github.com/drewfead/proto-cli"
	"github.com/drewfead/proto-cli/examples/simple"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

// setupTestCLI overrides os.Exit behavior for testing
func setupTestCLI(t *testing.T) *int {
	t.Helper()
	origExiter := cli.OsExiter
	t.Cleanup(func() { cli.OsExiter = origExiter })

	var exitCode int
	cli.OsExiter = func(code int) {
		exitCode = code
	}

	return &exitCode
}

// setWriterOnAllCommands sets the writer on all commands and subcommands
// This is needed because urfave/cli v3 doesn't propagate Writer to subcommands
func setWriterOnAllCommands(cmd *cli.Command, w io.Writer) {
	cmd.Writer = w
	for _, subcmd := range cmd.Commands {
		setWriterOnAllCommands(subcmd, w)
	}
}

// TestTemplateFormat_Integration demonstrates using template formats in a real CLI
func TestTemplateFormat_Integration(t *testing.T) {
	exitCode := setupTestCLI(t)
	ctx := context.Background()

	// Define custom templates for different message types
	templates := map[string]string{
		"example.UserResponse": `┌─────────────────────────────┐
│ User Details                │
├─────────────────────────────┤
│ Name:  {{.user.name}}
│ Email: {{.user.email}}
│ ID:    {{.user.id}}
└─────────────────────────────┘`,
	}

	// Create template format
	userTableFormat, err := protocli.TemplateFormat("user-table", templates)
	require.NoError(t, err)

	// Create service with template format
	userServiceCLI := simple.UserServiceCommand(ctx, newUserService,
		protocli.WithOutputFormats(userTableFormat, protocli.JSON()),
	)

	// Create root command
	rootCmd, err := protocli.RootCommand("testcli",
		protocli.Service(userServiceCLI),
	)
	require.NoError(t, err)

	// Execute command with template format
	var stdout bytes.Buffer
	setWriterOnAllCommands(rootCmd, &stdout)

	args := []string{"testcli", "user-service", "get", "--id", "1", "--format", "user-table"}
	err = rootCmd.Run(ctx, args)
	require.NoError(t, err)
	require.Equal(t, 0, *exitCode, "command should exit with code 0")

	// Verify output
	output := stdout.String()
	require.Contains(t, output, "User Details")
	require.Contains(t, output, "Name:  Test User")
	require.Contains(t, output, "Email: test@example.com")
	require.Contains(t, output, "ID:    1")
}

// TestTemplateFormat_CompactFormat demonstrates a compact one-line format
func TestTemplateFormat_CompactFormat(t *testing.T) {
	_ = setupTestCLI(t)
	ctx := context.Background()

	templates := map[string]string{
		"example.UserResponse": `{{.user.name}} <{{.user.email}}> (ID: {{.user.id}})`,
	}

	compactFormat, err := protocli.TemplateFormat("compact", templates)
	require.NoError(t, err)

	userServiceCLI := simple.UserServiceCommand(ctx, newUserService,
		protocli.WithOutputFormats(compactFormat),
	)

	rootCmd, err := protocli.RootCommand("testcli",
		protocli.Service(userServiceCLI),
	)
	require.NoError(t, err)

	var stdout bytes.Buffer
	setWriterOnAllCommands(rootCmd, &stdout)

	args := []string{"testcli", "user-service", "get", "--id", "1", "--format", "compact"}
	err = rootCmd.Run(ctx, args)
	require.NoError(t, err)

	require.Equal(t, "Test User <test@example.com> (ID: 1)", strings.TrimSpace(stdout.String()))
}

// TestTemplateFormat_WithCustomFunctions demonstrates using custom template functions
func TestTemplateFormat_WithCustomFunctions(t *testing.T) {
	_ = setupTestCLI(t)
	ctx := context.Background()

	templates := map[string]string{
		"example.UserResponse": `{{upper .user.name}} | {{lower .user.email}}`,
	}

	// Add custom template functions
	funcMap := template.FuncMap{
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
	}

	customFormat, err := protocli.TemplateFormat("custom", templates, funcMap)
	require.NoError(t, err)

	userServiceCLI := simple.UserServiceCommand(ctx, newUserService,
		protocli.WithOutputFormats(customFormat),
	)

	rootCmd, err := protocli.RootCommand("testcli",
		protocli.Service(userServiceCLI),
	)
	require.NoError(t, err)

	var stdout bytes.Buffer
	setWriterOnAllCommands(rootCmd, &stdout)

	args := []string{"testcli", "user-service", "get", "--id", "1", "--format", "custom"}
	err = rootCmd.Run(ctx, args)
	require.NoError(t, err)

	require.Equal(t, "TEST USER | test@example.com", strings.TrimSpace(stdout.String()))
}

// TestTemplateFormat_CSVFormat demonstrates CSV-like output
func TestTemplateFormat_CSVFormat(t *testing.T) {
	_ = setupTestCLI(t)
	ctx := context.Background()

	templates := map[string]string{
		"example.UserResponse": `{{.user.id}},{{.user.name}},{{.user.email}}`,
	}

	csvFormat, err := protocli.TemplateFormat("csv", templates)
	require.NoError(t, err)

	userServiceCLI := simple.UserServiceCommand(ctx, newUserService,
		protocli.WithOutputFormats(csvFormat),
	)

	rootCmd, err := protocli.RootCommand("testcli",
		protocli.Service(userServiceCLI),
	)
	require.NoError(t, err)

	var stdout bytes.Buffer
	setWriterOnAllCommands(rootCmd, &stdout)

	args := []string{"testcli", "user-service", "get", "--id", "1", "--format", "csv"}
	err = rootCmd.Run(ctx, args)
	require.NoError(t, err)

	require.Equal(t, "1,Test User,test@example.com", strings.TrimSpace(stdout.String()))
}

// TestTemplateFormat_ConditionalOutput demonstrates conditional rendering
func TestTemplateFormat_ConditionalOutput(t *testing.T) {
	_ = setupTestCLI(t)
	ctx := context.Background()

	templates := map[string]string{
		"example.UserResponse": `User: {{.user.name}}{{if .user.address}}
Address: {{.user.address.street}}, {{.user.address.city}}, {{.user.address.state}} {{.user.address.zipCode}}{{else}}
Address: Not provided{{end}}`,
	}

	detailFormat, err := protocli.TemplateFormat("detail", templates)
	require.NoError(t, err)

	userServiceCLI := simple.UserServiceCommand(ctx, newUserService,
		protocli.WithOutputFormats(detailFormat),
	)

	rootCmd, err := protocli.RootCommand("testcli",
		protocli.Service(userServiceCLI),
	)
	require.NoError(t, err)

	var stdout bytes.Buffer
	setWriterOnAllCommands(rootCmd, &stdout)

	args := []string{"testcli", "user-service", "get", "--id", "1", "--format", "detail"}
	err = rootCmd.Run(ctx, args)
	require.NoError(t, err)

	output := stdout.String()
	require.Contains(t, output, "User: Test User")
	// Test User doesn't have an address in our mock, so we should see "Not provided"
	require.Contains(t, output, "Address: Not provided")
}

// TestTemplateFormat_MultipleMessageTypes demonstrates handling different message types
func TestTemplateFormat_MultipleMessageTypes(t *testing.T) {
	_ = setupTestCLI(t)
	ctx := context.Background()

	templates := map[string]string{
		"example.UserResponse":        `[USER] {{.user.name}} ({{.user.email}})`,
		"example.CreateUserRequest":   `[CREATE] {{.name}} - {{.email}}`,
		"example.HealthCheckResponse": `[HEALTH] Status: {{.status}}`,
	}

	multiFormat, err := protocli.TemplateFormat("multi", templates)
	require.NoError(t, err)

	// Test with UserResponse
	userServiceCLI := simple.UserServiceCommand(ctx, newUserService,
		protocli.WithOutputFormats(multiFormat),
	)

	rootCmd, err := protocli.RootCommand("testcli",
		protocli.Service(userServiceCLI),
	)
	require.NoError(t, err)

	var stdout bytes.Buffer
	setWriterOnAllCommands(rootCmd, &stdout)

	args := []string{"testcli", "user-service", "get", "--id", "1", "--format", "multi"}
	err = rootCmd.Run(ctx, args)
	require.NoError(t, err)

	require.Equal(t, "[USER] Test User (test@example.com)", strings.TrimSpace(stdout.String()))
}

// TestTemplateFormat_MustTemplateFormat demonstrates panic on invalid template
func TestTemplateFormat_MustTemplateFormat(t *testing.T) {
	// Valid template - should not panic
	require.NotPanics(t, func() {
		templates := map[string]string{
			"example.UserResponse": `{{.user.name}}`,
		}
		_ = protocli.MustTemplateFormat("valid", templates)
	})

	// Invalid template - should panic
	require.Panics(t, func() {
		templates := map[string]string{
			"example.UserResponse": `{{.user.name`,
		}
		_ = protocli.MustTemplateFormat("invalid", templates)
	})
}

func TestTemplateFormat_PackageLevelFormat(t *testing.T) {
	_ = setupTestCLI(t)
	ctx := context.Background()

	// Example demonstrating package-level format initialization
	userCompactFormat := protocli.MustTemplateFormat("user-compact", map[string]string{
		"example.UserResponse": `{{.user.name}} <{{.user.email}}>`,
	})

	userServiceCLI := simple.UserServiceCommand(ctx, newUserService,
		protocli.WithOutputFormats(userCompactFormat),
	)

	rootCmd, err := protocli.RootCommand("testcli",
		protocli.Service(userServiceCLI),
	)
	require.NoError(t, err)

	var stdout bytes.Buffer
	setWriterOnAllCommands(rootCmd, &stdout)

	args := []string{"testcli", "user-service", "get", "--id", "1", "--format", "user-compact"}
	err = rootCmd.Run(ctx, args)
	require.NoError(t, err)

	require.Equal(t, "Test User <test@example.com>", strings.TrimSpace(stdout.String()))
}
