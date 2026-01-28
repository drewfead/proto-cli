package protocli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

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
	if err != nil {
		return fmt.Errorf("failed to write JSON: %w", err)
	}

	_, err = w.Write([]byte("\n"))
	return err
}

// goFormat formats proto messages using Go's default %+v formatting.
type goFormat struct{}

func (f *goFormat) Name() string {
	return "go"
}

func (f *goFormat) Format(_ context.Context, _ *cli.Command, w io.Writer, msg proto.Message) error {
	_, err := fmt.Fprintf(w, "%+v\n", msg)
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
	return writeYAML(w, data, 0)
}

func writeYAML(w io.Writer, data any, indent int) error {
	prefix := ""
	for range indent {
		prefix += "  "
	}

	switch v := data.(type) {
	case map[string]any:
		for key, val := range v {
			if subMap, ok := val.(map[string]any); ok {
				if _, err := fmt.Fprintf(w, "%s%s:\n", prefix, key); err != nil {
					return err
				}
				if err := writeYAML(w, subMap, indent+1); err != nil {
					return err
				}
			} else if subSlice, ok := val.([]any); ok {
				if _, err := fmt.Fprintf(w, "%s%s:\n", prefix, key); err != nil {
					return err
				}
				if err := writeYAML(w, subSlice, indent+1); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintf(w, "%s%s: %v\n", prefix, key, val); err != nil {
					return err
				}
			}
		}
	case []any:
		for _, item := range v {
			if _, err := fmt.Fprintf(w, "%s- ", prefix); err != nil {
				return err
			}
			if subMap, ok := item.(map[string]any); ok {
				if _, err := fmt.Fprintln(w); err != nil {
					return err
				}
				if err := writeYAML(w, subMap, indent+1); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintf(w, "%v\n", item); err != nil {
					return err
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
