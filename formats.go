package protocli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"text/template"

	"github.com/urfave/cli/v3"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// jsonFormat formats proto messages as JSON.
type jsonFormat struct{}

func (f *jsonFormat) Name() string {
	return "json"
}

func (f *jsonFormat) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:  "pretty",
			Usage: "Pretty-print JSON output with indentation",
		},
	}
}

func (f *jsonFormat) Format(_ context.Context, cmd *cli.Command, w io.Writer, msg proto.Message) error {
	marshaler := protojson.MarshalOptions{
		EmitUnpopulated: true,
	}

	if cmd.Bool("pretty") {
		marshaler.Indent = "  "
	}

	jsonBytes, err := marshaler.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	_, err = w.Write(jsonBytes)
	return err
}

// goFormat formats proto messages using Go's default %+v formatting.
type goFormat struct{}

func (f *goFormat) Name() string {
	return "go"
}

func (f *goFormat) Format(_ context.Context, _ *cli.Command, w io.Writer, msg proto.Message) error {
	_, err := fmt.Fprintf(w, "%+v", msg)
	return err
}

// yamlFormat formats proto messages as YAML.
type yamlFormat struct{}

func (f *yamlFormat) Name() string {
	return "yaml"
}

func (f *yamlFormat) Format(_ context.Context, _ *cli.Command, w io.Writer, msg proto.Message) error {
	// Convert to JSON first, then to YAML-like format
	marshaler := protojson.MarshalOptions{
		EmitUnpopulated: true,
		Indent:          "  ",
	}

	jsonBytes, err := marshaler.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// Parse JSON to map for YAML-style output
	var data map[string]any
	if err := json.Unmarshal(jsonBytes, &data); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	// Simple YAML-like output (for full YAML support, use gopkg.in/yaml.v3)
	// Use internal function that tracks last item to avoid trailing newline
	return writeYAMLMap(w, data, 0)
}

// writeMapFields writes a map's key-value pairs with proper YAML formatting
func writeMapFields(w io.Writer, data map[string]any, prefix string, indent int) error {
	// Get keys in a slice so we can track the last one
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}

	for i, key := range keys {
		val := data[key]
		isLast := i == len(keys)-1

		if subMap, ok := val.(map[string]any); ok {
			if _, err := fmt.Fprintf(w, "%s%s:\n", prefix, key); err != nil {
				return err
			}
			if err := writeYAMLValue(w, subMap, indent+1, false); err != nil {
				return err
			}
			// Add newline after nested structure unless it's the last field
			if !isLast {
				if _, err := fmt.Fprint(w, "\n"); err != nil {
					return err
				}
			}
		} else if subSlice, ok := val.([]any); ok {
			if _, err := fmt.Fprintf(w, "%s%s:\n", prefix, key); err != nil {
				return err
			}
			if err := writeYAMLValue(w, subSlice, indent+1, false); err != nil {
				return err
			}
			// Add newline after nested structure unless it's the last field
			if !isLast {
				if _, err := fmt.Fprint(w, "\n"); err != nil {
					return err
				}
			}
		} else {
			if isLast {
				if _, err := fmt.Fprintf(w, "%s%s: %v", prefix, key, val); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintf(w, "%s%s: %v\n", prefix, key, val); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func writeYAMLMap(w io.Writer, data map[string]any, indent int) error {
	prefix := ""
	for range indent {
		prefix += "  "
	}
	return writeMapFields(w, data, prefix, indent)
}

func writeYAMLValue(w io.Writer, data any, indent int, _ bool) error {
	prefix := ""
	for range indent {
		prefix += "  "
	}

	switch v := data.(type) {
	case map[string]any:
		return writeMapFields(w, v, prefix, indent)
	case []any:
		for i, item := range v {
			itemIsLast := i == len(v)-1
			if _, err := fmt.Fprintf(w, "%s- ", prefix); err != nil {
				return err
			}
			if subMap, ok := item.(map[string]any); ok {
				if _, err := fmt.Fprintln(w); err != nil {
					return err
				}
				if err := writeYAMLValue(w, subMap, indent+1, false); err != nil {
					return err
				}
				if !itemIsLast {
					if _, err := fmt.Fprint(w, "\n"); err != nil {
						return err
					}
				}
			} else {
				if itemIsLast {
					if _, err := fmt.Fprintf(w, "%v", item); err != nil {
						return err
					}
				} else {
					if _, err := fmt.Fprintf(w, "%v\n", item); err != nil {
						return err
					}
				}
			}
		}
	default:
		if _, err := fmt.Fprintf(w, "%s%v\n", prefix, v); err != nil {
			return err
		}
	}

	return nil
}

// Factory functions for built-in formats

// JSON returns a new JSON output format with optional --pretty flag.
func JSON() OutputFormat {
	return &jsonFormat{}
}

// YAML returns a new YAML output format.
func YAML() OutputFormat {
	return &yamlFormat{}
}

// Go returns a new Go-style output format (uses %+v).
func Go() OutputFormat {
	return &goFormat{}
}

// templateFormat renders proto messages using Go text templates.
// Templates are keyed by fully qualified message type name.
type templateFormat struct {
	name      string
	templates map[string]*template.Template // Parsed templates keyed by message type name
	funcMap   template.FuncMap              // Custom template functions
}

func (f *templateFormat) Name() string {
	return f.name
}

func (f *templateFormat) Format(_ context.Context, _ *cli.Command, w io.Writer, msg proto.Message) error {
	// Get the fully qualified message type name
	msgType := string(msg.ProtoReflect().Descriptor().FullName())

	// Look up the template for this message type
	tmpl, ok := f.templates[msgType]
	if !ok {
		return fmt.Errorf("no template registered for message type %s (available: %v)", msgType, f.availableTypes())
	}

	// Convert proto message to map for easier template access
	data, err := protoToMap(msg)
	if err != nil {
		return fmt.Errorf("failed to convert proto message to map: %w", err)
	}

	// Execute the template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to execute template for %s: %w", msgType, err)
	}

	// Write the result
	_, err = w.Write(buf.Bytes())
	return err
}

// availableTypes returns a list of available message types for error messages
func (f *templateFormat) availableTypes() []string {
	types := make([]string, 0, len(f.templates))
	for t := range f.templates {
		types = append(types, t)
	}
	return types
}

// protoToMap converts a proto message to a map[string]any for template rendering.
// This makes it easier to access fields in templates using dot notation.
func protoToMap(msg proto.Message) (map[string]any, error) {
	// Convert to JSON first
	marshaler := protojson.MarshalOptions{
		EmitUnpopulated: true,
	}
	jsonBytes, err := marshaler.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal to JSON: %w", err)
	}

	// Parse JSON to map
	var data map[string]any
	if err := json.Unmarshal(jsonBytes, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	return data, nil
}

// TemplateFormat creates an output format that renders proto messages using Go text templates.
//
// Templates are specified as a map from fully qualified message type name to template string.
// The message type name format is "package.MessageName" (e.g., "example.UserResponse").
//
// Templates have access to all message fields as a map[string]any, making field access
// straightforward using Go template syntax like {{.FieldName}}.
//
// Optional function maps can be provided to add custom template functions.
//
// Example:
//
//	templates := map[string]string{
//	    "example.UserResponse": `User: {{.user.name}} ({{.user.email}})
//	ID: {{.user.id}}
//	{{if .user.verified}}✓ Verified{{else}}✗ Not verified{{end}}`,
//	}
//
//	format := protocli.TemplateFormat("user-table", templates)
//
// With custom functions:
//
//	funcMap := template.FuncMap{
//	    "upper": strings.ToUpper,
//	    "date": func(ts string) string {
//	        // Custom date formatting
//	        return formattedDate
//	    },
//	}
//
//	format := protocli.TemplateFormat("custom", templates, funcMap)
func TemplateFormat(name string, templates map[string]string, funcMaps ...template.FuncMap) (OutputFormat, error) {
	// Merge all function maps
	funcMap := template.FuncMap{}
	for _, fm := range funcMaps {
		for k, v := range fm {
			funcMap[k] = v
		}
	}

	// Parse all templates
	parsed := make(map[string]*template.Template)
	for msgType, tmplStr := range templates {
		tmpl, err := template.New(msgType).Funcs(funcMap).Parse(tmplStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse template for %s: %w", msgType, err)
		}
		parsed[msgType] = tmpl
	}

	return &templateFormat{
		name:      name,
		templates: parsed,
		funcMap:   funcMap,
	}, nil
}

// MustTemplateFormat is like TemplateFormat but panics on error.
// Useful for package-level initialization where template errors should be caught at startup.
//
// Example:
//
//	var userFormat = protocli.MustTemplateFormat("user", map[string]string{
//	    "example.UserResponse": `{{.user.name}} <{{.user.email}}>`,
//	})
func MustTemplateFormat(name string, templates map[string]string, funcMaps ...template.FuncMap) OutputFormat {
	format, err := TemplateFormat(name, templates, funcMaps...)
	if err != nil {
		panic(fmt.Sprintf("failed to create template format: %v", err))
	}
	return format
}
