package protocli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"text/template"

	"github.com/urfave/cli/v3"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// ErrNoTemplate is returned when no template is registered for a message type.
var ErrNoTemplate = errors.New("no template registered for message type")

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
	// Get keys in a sorted slice for deterministic output
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

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
		return fmt.Errorf("%w: %s (available: %v)", ErrNoTemplate, msgType, f.availableTypes())
	}

	// Pass the proto message directly to templates
	// Custom template functions receive actual proto types
	// Templates can use the 'field' helper for field access or 'json' for JSON conversion
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, msg); err != nil {
		return fmt.Errorf("failed to execute template for %s: %w", msgType, err)
	}

	// Write the result
	_, err := w.Write(buf.Bytes())
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

// DefaultTemplateFunctions returns the default set of template functions.
// These functions are available in all templates unless overridden.
func DefaultTemplateFunctions() template.FuncMap {
	return template.FuncMap{
		// protoField accesses a field from a proto message by JSON name using reflection
		// Returns the actual proto type for that field
		// Usage: {{protoField . "fieldName"}}
		"protoField": func(msg any, fieldName string) any {
			if m, ok := msg.(proto.Message); ok {
				return getProtoField(m, fieldName)
			}
			return nil
		},

		// protoJSON converts a proto message to JSON string using protojson
		// Usage: {{protoJSON .}}
		"protoJSON": func(msg any) string {
			if m, ok := msg.(proto.Message); ok {
				marshaler := protojson.MarshalOptions{
					EmitUnpopulated: false,
				}
				jsonBytes, err := marshaler.Marshal(m)
				if err != nil {
					return fmt.Sprintf("error: %v", err)
				}
				return string(jsonBytes)
			}
			return fmt.Sprintf("%v", msg)
		},

		// protoJSONIndent converts a proto message to indented JSON string
		// Usage: {{protoJSONIndent .}}
		"protoJSONIndent": func(msg any) string {
			if m, ok := msg.(proto.Message); ok {
				marshaler := protojson.MarshalOptions{
					EmitUnpopulated: false,
					Indent:          "  ",
				}
				jsonBytes, err := marshaler.Marshal(m)
				if err != nil {
					return fmt.Sprintf("error: %v", err)
				}
				return string(jsonBytes)
			}
			return fmt.Sprintf("%v", msg)
		},

		// protoFields converts a proto message to a map for dot-chain field access
		// The proto message is converted via JSON, so proto types become JSON types
		// (e.g., timestamps become strings). Use this for easy template access patterns.
		// Usage: {{$fields := protoFields .}}{{$fields.user.name}}
		"protoFields": func(msg any) map[string]any {
			if m, ok := msg.(proto.Message); ok {
				marshaler := protojson.MarshalOptions{
					EmitUnpopulated: false,
				}
				jsonBytes, err := marshaler.Marshal(m)
				if err != nil {
					return nil
				}

				var result map[string]any
				if err := json.Unmarshal(jsonBytes, &result); err != nil {
					return nil
				}
				return result
			}
			return nil
		},
	}
}

// getProtoField extracts a field value from a proto message by JSON name.
// Returns the actual proto type (proto messages are returned as-is, not converted).
func getProtoField(msg proto.Message, fieldName string) any {
	m := msg.ProtoReflect()
	fields := m.Descriptor().Fields()

	// Find field by JSON name
	var fd protoreflect.FieldDescriptor
	for i := 0; i < fields.Len(); i++ {
		f := fields.Get(i)
		if f.JSONName() == fieldName {
			fd = f
			break
		}
	}

	if fd == nil {
		return nil
	}

	if !m.Has(fd) {
		return nil
	}

	v := m.Get(fd)

	// For message fields, return the actual proto message
	if fd.Kind() == protoreflect.MessageKind {
		return v.Message().Interface()
	}

	// For lists, return as slice (preserving proto messages)
	if fd.IsList() {
		list := v.List()
		slice := make([]any, list.Len())
		for i := 0; i < list.Len(); i++ {
			item := list.Get(i)
			if fd.Kind() == protoreflect.MessageKind {
				slice[i] = item.Message().Interface()
			} else {
				slice[i] = item.Interface()
			}
		}
		return slice
	}

	// For maps, return as map (preserving proto messages in values)
	if fd.IsMap() {
		result := make(map[string]any)
		v.Map().Range(func(k protoreflect.MapKey, v protoreflect.Value) bool {
			keyStr := fmt.Sprint(k.Interface())
			if fd.MapValue().Kind() == protoreflect.MessageKind {
				result[keyStr] = v.Message().Interface()
			} else {
				result[keyStr] = v.Interface()
			}
			return true
		})
		return result
	}

	// For scalars, return the Go value
	return v.Interface()
}

// TemplateFormat creates an output format that renders proto messages using Go text templates.
//
// Templates are specified as a map from fully qualified message type name to template string.
// The message type name format is "package.MessageName" (e.g., "example.UserResponse").
//
// Templates receive the actual proto message directly. Custom template functions receive
// actual proto types (e.g., *timestamppb.Timestamp), not JSON strings or maps.
//
// Default template functions available:
//   - protoField: access message fields by JSON name, preserving proto types
//   - protoJSON: convert proto message to JSON string
//   - protoJSONIndent: convert proto message to indented JSON string
//   - protoFields: convert proto message to map for dot-chain field access
//
// Field access patterns:
//  1. Use protoFields for easy dot-chain access (proto types become JSON types):
//     {{$fields := protoFields .}}{{$fields.user.name}}
//  2. Use protoField helper to preserve proto types: {{protoField . "fieldName"}}
//  3. Register custom accessor functions for your message types (recommended for complex templates)
//
// Optional function maps can be provided to add custom template functions.
// Functions are merged in order: defaults, global registry, then provided funcMaps.
//
// Example with protoFields (simplest):
//
//	templates := map[string]string{
//	    "example.UserResponse": `{{$f := protoFields .}}User: {{$f.user.name}}
//	Email: {{$f.user.email}}
//	ID: {{$f.user.id}}`,
//	}
//
//	format := protocli.TemplateFormat("user-table", templates)
//
// Example with custom accessor (best for proto types):
//
//	// Register type-specific accessor functions globally
//	protocli.TemplateFunctions().Register("user", func(resp *simple.UserResponse) *simple.User {
//	    return resp.GetUser()
//	})
//
//	protocli.TemplateFunctions().Register("formatTime", func(ts *timestamppb.Timestamp) string {
//	    if ts == nil || !ts.IsValid() {
//	        return "N/A"
//	    }
//	    return ts.AsTime().Format("2006-01-02")
//	})
//
//	templates := map[string]string{
//	    "example.UserResponse": `User: {{(user .).GetName}}
//	Created: {{formatTime (user .).GetCreatedAt}}`,
//	}
//
//	format := protocli.TemplateFormat("user-table", templates)
func TemplateFormat(name string, templates map[string]string, funcMaps ...template.FuncMap) (OutputFormat, error) {
	// Start with default functions
	funcMap := DefaultTemplateFunctions()

	// Merge global registry
	for k, v := range globalTemplateFunctionRegistry.Functions() {
		funcMap[k] = v
	}

	// Merge provided function maps (later maps override earlier ones)
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
