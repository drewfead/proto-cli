package cliconfig

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/urfave/cli/v3"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var (
	// ErrInvalidArgument is returned when a command line argument is invalid
	ErrInvalidArgument = errors.New("invalid argument format")

	// ErrKeyValueRequired is returned when at least one key=value pair is required
	ErrKeyValueRequired = errors.New("at least one key=value pair required")

	// ErrExactlyOneKey is returned when exactly one key is required
	ErrExactlyOneKey = errors.New("exactly one key required")
)

// Commands creates the config command suite with init, set, get, and list subcommands
func Commands(manager *Manager) *cli.Command {
	return &cli.Command{
		Name:  "config",
		Usage: "manage configuration",
		Commands: []*cli.Command{
			initCommand(manager),
			setCommand(manager),
			getCommand(manager),
			listCommand(manager),
		},
	}
}

// initCommand creates the 'config init' command
func initCommand(manager *Manager) *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "initialize or edit configuration file",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "global",
				Usage: "operate on global config (~/.config/appname/config.yaml)",
			},
			&cli.BoolFlag{
				Name:  "replace",
				Usage: "replace existing config file with stub template",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			path := manager.LocalPath()
			if cmd.Bool("global") {
				path = manager.GlobalPath()
			}

			// Check if file exists
			fileExists := false
			if _, err := os.Stat(path); err == nil {
				fileExists = true
			}

			// If file exists and --replace not specified, just open for editing
			if fileExists && !cmd.Bool("replace") {
				return openEditor(ctx, path)
			}

			// Generate stub config
			stub, err := generateConfigStub(manager)
			if err != nil {
				return fmt.Errorf("failed to generate config stub: %w", err)
			}

			// Create directory if needed
			dir := filepath.Dir(path)
			if err := os.MkdirAll(dir, 0o750); err != nil {
				return fmt.Errorf("failed to create config directory: %w", err)
			}

			// Write stub to file
			if err := os.WriteFile(path, []byte(stub), 0o600); err != nil {
				return fmt.Errorf("failed to write config file: %w", err)
			}

			// Open in editor
			return openEditor(ctx, path)
		},
	}
}

// setCommand creates the 'config set' command
func setCommand(manager *Manager) *cli.Command {
	return &cli.Command{
		Name:      "set",
		Usage:     "set configuration values",
		ArgsUsage: "<key=value> [key=value...]",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "global",
				Usage: "set in global config (~/.config/appname/config.yaml)",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() == 0 {
				return ErrKeyValueRequired
			}

			path := manager.LocalPath()
			if cmd.Bool("global") {
				path = manager.GlobalPath()
			}

			// Parse key=value pairs
			keyValues := make(map[string]string)
			for i := 0; i < cmd.Args().Len(); i++ {
				arg := cmd.Args().Get(i)
				parts := strings.SplitN(arg, "=", 2)
				if len(parts) != 2 {
					return fmt.Errorf("%w: %s (expected key=value)", ErrInvalidArgument, arg)
				}
				keyValues[parts[0]] = parts[1]
			}

			// Set values
			if err := manager.SetValue(path, keyValues); err != nil {
				if errors.Is(err, ErrListNotSupported) {
					return fmt.Errorf("%w\nTo set list/array fields, edit the config file with: config init", err)
				}
				return err
			}

			_, _ = fmt.Fprintf(cmd.Writer, "Set %d value(s) in %s\n", len(keyValues), path)
			return nil
		},
	}
}

// getCommand creates the 'config get' command
func getCommand(manager *Manager) *cli.Command {
	return &cli.Command{
		Name:      "get",
		Usage:     "get configuration value",
		ArgsUsage: "<key>",
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() != 1 {
				return ErrExactlyOneKey
			}

			key := cmd.Args().First()

			// Check if this key has nested fields
			allKeys := manager.getAllKeys()
			hasNested := false
			prefix := key + "."
			for _, k := range allKeys {
				if strings.HasPrefix(k, prefix) {
					hasNested = true
					break
				}
			}

			if hasNested {
				// Return flattened representation of nested object
				for _, k := range allKeys {
					if strings.HasPrefix(k, prefix) || k == key {
						val, source, err := manager.GetValue(k)
						if err != nil {
							return err
						}
						if val != "" {
							_, _ = fmt.Fprintf(cmd.Writer, "%s: %s  # %s\n", k, val, source)
						}
					}
				}
				return nil
			}

			// Get single value
			val, source, err := manager.GetValue(key)
			if err != nil {
				if errors.Is(err, ErrInvalidKey) {
					return fmt.Errorf("%w: %s", ErrInvalidKey, key)
				}
				return err
			}

			if val == "" {
				// Key exists in schema but not set
				return nil
			}

			_, _ = fmt.Fprintf(cmd.Writer, "%s  # %s\n", val, source)
			return nil
		},
	}
}

// listCommand creates the 'config list' command
func listCommand(manager *Manager) *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "list all configuration values",
		Action: func(_ context.Context, cmd *cli.Command) error {
			values, err := manager.ListAll()
			if err != nil {
				return err
			}

			// Get all keys and sort them for consistent output
			keys := make([]string, 0, len(values))
			for k := range values {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			// Print in flat format with source annotations
			for _, key := range keys {
				v := values[key]
				if v.Value != "" {
					_, _ = fmt.Fprintf(cmd.Writer, "%s: %s  # %s\n", key, v.Value, v.Source)
				} else {
					_, _ = fmt.Fprintf(cmd.Writer, "%s:   # %s (not set)\n", key, v.Source)
				}
			}

			return nil
		},
	}
}

// openEditor opens the specified file in the user's preferred editor
func openEditor(ctx context.Context, path string) error {
	// Check for editor in order: VISUAL, EDITOR, fallback to vi
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}

	cmd := exec.CommandContext(ctx, editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// generateConfigStub generates a commented YAML config stub from the proto schema
func generateConfigStub(manager *Manager) (string, error) {
	var sb strings.Builder

	sb.WriteString("# Configuration file\n")
	sb.WriteString("# Edit values below and save\n\n")

	indent := 0
	if manager.serviceName != "" {
		sb.WriteString("services:\n")
		sb.WriteString("  ")
		sb.WriteString(manager.serviceName)
		sb.WriteString(":\n")
		indent = 2
	}

	if err := writeFieldsAsYAML(&sb, manager.configMsg.ProtoReflect(), indent); err != nil {
		return "", err
	}

	return sb.String(), nil
}

// writeFieldsAsYAML writes proto message fields as commented YAML
func writeFieldsAsYAML(sb *strings.Builder, msg protoreflect.Message, indent int) error {
	fields := msg.Descriptor().Fields()

	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		fieldName := fd.JSONName()

		// Write comment with field type
		comment := getFieldComment(fd)
		if comment != "" {
			for j := 0; j < indent; j++ {
				sb.WriteString("  ")
			}
			sb.WriteString("# ")
			sb.WriteString(comment)
			sb.WriteString("\n")
		}

		// Write field
		for j := 0; j < indent; j++ {
			sb.WriteString("  ")
		}

		if fd.Kind() == protoreflect.MessageKind && !fd.IsList() && !fd.IsMap() {
			// Nested message
			sb.WriteString(fieldName)
			sb.WriteString(":\n")
			nestedMsg := msg.NewField(fd).Message()
			if err := writeFieldsAsYAML(sb, nestedMsg, indent+1); err != nil {
				return err
			}
		} else {
			// Scalar, list, or map field
			sb.WriteString(fieldName)
			sb.WriteString(": ")
			sb.WriteString(getFieldPlaceholder(fd))
			sb.WriteString("\n")
		}

		// Add blank line between top-level fields
		if indent == 0 && i < fields.Len()-1 {
			sb.WriteString("\n")
		}
	}

	return nil
}

// getFieldComment returns a comment for a field based on its type
func getFieldComment(fd protoreflect.FieldDescriptor) string {
	// Note: Future enhancement - get from proto annotations (description field)
	// For now, just return the field type
	kind := fd.Kind().String()
	if fd.IsList() {
		return fmt.Sprintf("List of %s", kind)
	}
	if fd.IsMap() {
		return "Map field"
	}
	return ""
}

// getFieldPlaceholder returns a placeholder value for a field
func getFieldPlaceholder(fd protoreflect.FieldDescriptor) string {
	// Note: Future enhancement - get from proto annotations (placeholder field)
	// For now, use type-based defaults

	if fd.IsList() {
		return "[]"
	}

	if fd.IsMap() {
		return "{}"
	}

	switch fd.Kind() {
	case protoreflect.BoolKind:
		return "false"
	case protoreflect.StringKind:
		return `""`
	case protoreflect.Int32Kind, protoreflect.Int64Kind,
		protoreflect.Uint32Kind, protoreflect.Uint64Kind,
		protoreflect.Sint32Kind, protoreflect.Sint64Kind,
		protoreflect.Fixed32Kind, protoreflect.Fixed64Kind,
		protoreflect.Sfixed32Kind, protoreflect.Sfixed64Kind:
		return "0"
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return "0.0"
	case protoreflect.BytesKind:
		return `""`
	case protoreflect.EnumKind:
		// Get first enum value
		enumDesc := fd.Enum()
		if enumDesc.Values().Len() > 0 {
			return fmt.Sprintf(`"%s"`, enumDesc.Values().Get(0).Name())
		}
		return `""`
	case protoreflect.MessageKind, protoreflect.GroupKind:
		// These should be handled by the caller
		return `""`
	default:
		return `""`
	}
}
