// Package formats provides additional output formats for proto-cli.
package formats

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	protocli "github.com/drewfead/proto-cli"
	"github.com/urfave/cli/v3"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// tableFormat renders proto messages as tab-separated columns.
//
// The first call to Format emits a header row followed by the data row.
// Subsequent calls emit data rows only, which works correctly for both
// unary and streaming RPCs.
//
// The headerSent state resets per-process, which is correct for CLI usage
// (one command per process).
type tableFormat struct {
	mu         sync.Mutex
	headerSent bool
}

func (f *tableFormat) Name() string {
	return "table"
}

func (f *tableFormat) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:  "no-header",
			Usage: "Suppress the header row in table output",
		},
	}
}

func (f *tableFormat) Format(_ context.Context, cmd *cli.Command, w io.Writer, msg proto.Message) error {
	flat, err := flattenMessage(msg)
	if err != nil {
		return err
	}

	keys := sortedKeys(flat)

	f.mu.Lock()
	needHeader := !f.headerSent && !cmd.Bool("no-header")
	f.headerSent = true
	f.mu.Unlock()

	if needHeader {
		if _, err := fmt.Fprintln(w, strings.Join(keys, "\t")); err != nil {
			return err
		}
	}

	vals := make([]string, 0, len(keys))
	for _, k := range keys {
		vals = append(vals, formatValue(flat[k]))
	}

	_, err = fmt.Fprintln(w, strings.Join(vals, "\t"))

	return err
}

// Table returns an OutputFormat that renders proto messages as tab-separated columns.
//
// Features:
//   - Flattens nested messages with dot notation (e.g. "user.name")
//   - Renders repeated fields as comma-separated values
//   - Sorted column keys for deterministic output
//   - First Format() call emits header + data; subsequent calls emit data only
//   - Supports --no-header flag to suppress the header row
//   - Tab-separated output is composable with `column -t`
func Table() protocli.OutputFormat {
	return &tableFormat{}
}

// flattenMessage converts a proto message to a flat map of string keys to values
// using dot notation for nested structures.
//
// Well-known types like Timestamp and Duration serialize to JSON scalars (strings)
// rather than objects. In that case, the message type name is used as the sole key.
func flattenMessage(msg proto.Message) (map[string]any, error) {
	marshaler := protojson.MarshalOptions{
		EmitUnpopulated: true,
	}

	jsonBytes, err := marshaler.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	// Try to unmarshal as object first; well-known types may serialize as scalars.
	var data map[string]any
	if err := json.Unmarshal(jsonBytes, &data); err != nil {
		// Not a JSON object — treat the entire value as a single column
		// keyed by the message type name.
		var scalar any
		if unmarshalErr := json.Unmarshal(jsonBytes, &scalar); unmarshalErr != nil {
			return nil, fmt.Errorf("failed to unmarshal JSON: %w", unmarshalErr)
		}

		typeName := string(msg.ProtoReflect().Descriptor().Name())

		return map[string]any{typeName: scalar}, nil
	}

	flat := make(map[string]any)
	flatten("", data, flat)

	return flat, nil
}

// flatten recursively flattens nested maps using dot-separated keys.
func flatten(prefix string, data map[string]any, out map[string]any) {
	for k, v := range data {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}

		switch val := v.(type) {
		case map[string]any:
			flatten(key, val, out)
		default:
			out[key] = v
		}
	}
}

// sortedKeys returns the keys of a map in sorted order.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

// formatValue converts a value to its string representation for table output.
// Slices are rendered as comma-separated values, nil as empty string.
func formatValue(v any) string {
	if v == nil {
		return ""
	}

	if slice, ok := v.([]any); ok {
		parts := make([]string, 0, len(slice))
		for _, item := range slice {
			parts = append(parts, fmt.Sprintf("%v", item))
		}

		return strings.Join(parts, ",")
	}

	return fmt.Sprintf("%v", v)
}
