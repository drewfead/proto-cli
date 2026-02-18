package formats_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/drewfead/proto-cli/contrib/formats"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// newTestCommand creates a minimal cli.Command for testing with optional flags set.
func newTestCommand(flags map[string]string) *cli.Command {
	cliFlags := []cli.Flag{
		&cli.BoolFlag{
			Name: "no-header",
		},
	}

	cmd := &cli.Command{
		Name:  "test",
		Flags: cliFlags,
	}

	// Build args from flags
	args := []string{"test"}
	for k, v := range flags {
		args = append(args, "--"+k+"="+v)
	}

	// Parse the command to set flag values
	_ = cmd.Run(context.Background(), args)

	return cmd
}

func TestUnit_Table_Name(t *testing.T) {
	tbl := formats.Table()
	assert.Equal(t, "table", tbl.Name())
}

func TestUnit_Table_SingleMessage(t *testing.T) {
	ctx := context.Background()
	cmd := newTestCommand(nil)
	tbl := formats.Table()

	// Timestamp serializes to a JSON string via protojson (WKT behavior),
	// so the table shows a single column with the type name as header.
	ts := &timestamppb.Timestamp{Seconds: 1705314600, Nanos: 0}

	var buf bytes.Buffer
	err := tbl.Format(ctx, cmd, &buf, ts)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 2, "should have header + data row")

	// Header should contain the type name as the column key
	assert.Contains(t, lines[0], "Timestamp")

	// Data row should contain the RFC3339 value
	assert.Contains(t, lines[1], "2024-01-15")
}

func TestUnit_Table_StreamingMultipleMessages(t *testing.T) {
	ctx := context.Background()
	cmd := newTestCommand(nil)
	tbl := formats.Table()

	// Use structpb.Struct to get JSON-object output (timestamps are WKTs and serialize as strings)
	st1, err := structpb.NewStruct(map[string]any{"name": "Alice", "age": 30})
	require.NoError(t, err)

	st2, err := structpb.NewStruct(map[string]any{"name": "Bob", "age": 25})
	require.NoError(t, err)

	var buf bytes.Buffer

	err = tbl.Format(ctx, cmd, &buf, st1)
	require.NoError(t, err)

	err = tbl.Format(ctx, cmd, &buf, st2)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 3, "should have header + 2 data rows")

	// First line is header (structpb.Struct serializes fields directly via protojson)
	assert.Contains(t, lines[0], "age")
	assert.Contains(t, lines[0], "name")

	// Data rows
	assert.Contains(t, lines[1], "Alice")
	assert.Contains(t, lines[2], "Bob")
}

func TestUnit_Table_NestedMessageFlattening(t *testing.T) {
	ctx := context.Background()
	cmd := newTestCommand(nil)
	tbl := formats.Table()

	st, err := structpb.NewStruct(map[string]any{
		"name": "Alice",
		"address": map[string]any{
			"city":  "Portland",
			"state": "OR",
		},
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = tbl.Format(ctx, cmd, &buf, st)
	require.NoError(t, err)

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.Len(t, lines, 2, "should have header + data row")

	// Header should contain flattened keys with dot notation
	header := lines[0]
	assert.Contains(t, header, "address.city")
	assert.Contains(t, header, "address.state")
	assert.Contains(t, header, "name")
}

func TestUnit_Table_NoHeader(t *testing.T) {
	ctx := context.Background()
	cmd := newTestCommand(map[string]string{"no-header": "true"})
	tbl := formats.Table()

	st, err := structpb.NewStruct(map[string]any{"name": "Alice", "age": float64(30)})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = tbl.Format(ctx, cmd, &buf, st)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 1, "should have data row only, no header")
	assert.Contains(t, lines[0], "Alice")
}

func TestUnit_Table_EmptyValues(t *testing.T) {
	ctx := context.Background()
	cmd := newTestCommand(nil)
	tbl := formats.Table()

	// Empty struct (no fields)
	st, err := structpb.NewStruct(map[string]any{})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = tbl.Format(ctx, cmd, &buf, st)
	require.NoError(t, err)

	// Empty struct produces no columns, so output may be minimal
	output := buf.String()
	assert.NotEmpty(t, output)
}

func TestUnit_Table_RepeatedFields(t *testing.T) {
	ctx := context.Background()
	cmd := newTestCommand(nil)
	tbl := formats.Table()

	st, err := structpb.NewStruct(map[string]any{
		"tags": []any{"go", "proto", "cli"},
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = tbl.Format(ctx, cmd, &buf, st)
	require.NoError(t, err)

	output := buf.String()
	// The list values should appear in the output
	assert.Contains(t, output, "tags")
}

func TestUnit_Table_DeterministicColumnOrder(t *testing.T) {
	ctx := context.Background()
	cmd := newTestCommand(nil)

	// Run multiple times to verify deterministic output
	for range 5 {
		tbl := formats.Table() // Fresh instance each time

		st, err := structpb.NewStruct(map[string]any{
			"zeta":  1,
			"alpha": 2,
			"mu":    3,
		})
		require.NoError(t, err)

		var buf bytes.Buffer
		err = tbl.Format(ctx, cmd, &buf, st)
		require.NoError(t, err)

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		require.GreaterOrEqual(t, len(lines), 1)

		// Keys should be sorted alphabetically in the header
		header := lines[0]
		alphaIdx := strings.Index(header, "alpha")
		muIdx := strings.Index(header, "mu")
		zetaIdx := strings.Index(header, "zeta")
		assert.Less(t, alphaIdx, muIdx, "alpha should come before mu")
		assert.Less(t, muIdx, zetaIdx, "mu should come before zeta")
	}
}

func TestUnit_Table_Flags(t *testing.T) {
	tbl := formats.Table()

	// Verify it implements FlagConfiguredOutputFormat
	flagged, ok := tbl.(interface{ Flags() []cli.Flag })
	require.True(t, ok, "Table should implement FlagConfiguredOutputFormat")

	flags := flagged.Flags()
	require.Len(t, flags, 1)
	assert.Equal(t, "no-header", flags[0].Names()[0])
}
