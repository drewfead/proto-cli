// Package protocli provides framework features for generated gRPC CLI commands.
//
// This package is automatically imported by generated CLI code and provides
// lifecycle hooks, output formatting, configuration options, and utilities for CLI applications.
//
// # Options
//
// The Option type provides a functional options pattern for configuring CLI behavior:
//
//	cmd := proto.BuildUserServiceCLI(ctx, impl,
//	    protocli.BeforeCommand(func(ctx context.Context, cmd *cli.Command) error {
//	        log.Printf("Running command: %s", cmd.Name)
//	        return nil
//	    }),
//	    protocli.AfterCommand(func(ctx context.Context, cmd *cli.Command) error {
//	        log.Printf("Completed command: %s", cmd.Name)
//	        return nil
//	    }),
//	    protocli.WithOutputFormats(
//	        protocli.JSON(),
//	        protocli.YAML(),
//	    ),
//	)
//
// # Lifecycle Hooks
//
//   - BeforeCommand: Runs before each command execution
//   - AfterCommand: Runs after each command execution
//
// Hooks receive the context and command, allowing for logging, validation,
// metrics collection, or other cross-cutting concerns.
//
// # Output Formats
//
// The CLI automatically supports --format and --output flags for all commands:
//
//   - --format: Specifies output format (go, json, yaml, or custom)
//   - --output: Specifies output file (- or empty for stdout)
//
// Built-in formats (use factory functions to create them):
//
//   - protocli.Go(): Default Go %+v formatting (automatically used if no formats registered)
//   - protocli.JSON(): JSON output with optional --pretty flag
//   - protocli.YAML(): YAML-style output
//
// If no formats are explicitly registered via WithOutputFormats, the Go format is used
// as the default. Custom formats can be registered and will define additional flags
// (e.g., --pretty for JSON) that are automatically added to all commands.
//
// Example custom format without additional flags:
//
//	type CSVFormat struct{}
//
//	func (f *CSVFormat) Name() string { return "csv" }
//
//	func (f *CSVFormat) Format(ctx context.Context, cmd *cli.Command, w io.Writer, msg proto.Message) error {
//	    // Format as CSV...
//	    return nil
//	}
//
// Example custom format with additional flags (implements FlagConfiguredOutputFormat):
//
//	type CSVFormat struct{}
//
//	func (f *CSVFormat) Name() string { return "csv" }
//
//	func (f *CSVFormat) Flags() []cli.Flag {
//	    return []cli.Flag{
//	        &cli.StringFlag{Name: "delimiter", Value: ",", Usage: "CSV delimiter"},
//	    }
//	}
//
//	func (f *CSVFormat) Format(ctx context.Context, cmd *cli.Command, w io.Writer, msg proto.Message) error {
//	    delimiter := cmd.String("delimiter")
//	    // Format as CSV...
//	    return nil
//	}
//
// The Flags() method is optional - implement it only if your format needs custom flags.
// The generated CLI code checks for the FlagConfiguredOutputFormat interface at runtime.
//
// # Template-Based Formats
//
// For simpler cases, use TemplateFormat to create formats using Go text templates:
//
//	templates := map[string]string{
//	    "example.UserResponse": `User: {{.user.name}} ({{.user.email}})`,
//	    "example.CreateUserRequest": `Creating: {{.name}}`,
//	}
//	format, err := protocli.TemplateFormat("user-compact", templates)
//
// Templates support:
//   - Field access: {{.fieldName}}
//   - Conditionals: {{if .field}}...{{end}}
//   - Loops: {{range .list}}...{{end}}
//   - Custom functions via template.FuncMap
//   - Nested fields: {{.user.address.city}}
//
// Message types are identified by fully qualified name (e.g., "example.UserResponse").
package protocli
