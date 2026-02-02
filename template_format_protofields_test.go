package protocli_test

import (
	"bytes"
	"context"
	"testing"
	"text/template"

	protocli "github.com/drewfead/proto-cli"
	"github.com/drewfead/proto-cli/examples/simple"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestUnit_TemplateFormat_ProtoFields demonstrates using protoFields for easy dot-chain access
func TestUnit_TemplateFormat_ProtoFields(t *testing.T) {
	templates := map[string]string{
		"example.UserResponse": `{{$f := protoFields .}}Name: {{$f.user.name}}
Email: {{$f.user.email}}
ID: {{$f.user.id}}`,
	}

	format, err := protocli.TemplateFormat("user-info", templates)
	require.NoError(t, err)

	msg := &simple.UserResponse{
		User: &simple.User{
			Id:    123,
			Name:  "Alice",
			Email: "alice@example.com",
		},
	}

	var buf bytes.Buffer
	err = format.Format(context.Background(), &cli.Command{}, &buf, msg)
	require.NoError(t, err)

	output := buf.String()
	require.Contains(t, output, "Name: Alice")
	require.Contains(t, output, "Email: alice@example.com")
	require.Contains(t, output, "ID: 123")
}

// TestUnit_TemplateFormat_ProtoFieldsWithConditionals shows conditionals work with protoFields
func TestUnit_TemplateFormat_ProtoFieldsWithConditionals(t *testing.T) {
	templates := map[string]string{
		"example.UserResponse": `{{$f := protoFields .}}{{$f.user.name}}{{if $f.user.address}} (has address){{else}} (no address){{end}}`,
	}

	format, err := protocli.TemplateFormat("user-check", templates)
	require.NoError(t, err)

	// Test with address
	msgWithAddress := &simple.UserResponse{
		User: &simple.User{
			Id:   123,
			Name: "Alice",
			Address: &simple.Address{
				Street: "123 Main St",
			},
		},
	}

	var buf bytes.Buffer
	err = format.Format(context.Background(), &cli.Command{}, &buf, msgWithAddress)
	require.NoError(t, err)
	require.Contains(t, buf.String(), "Alice (has address)")

	// Test without address
	msgWithoutAddress := &simple.UserResponse{
		User: &simple.User{
			Id:   456,
			Name: "Bob",
		},
	}

	buf.Reset()
	err = format.Format(context.Background(), &cli.Command{}, &buf, msgWithoutAddress)
	require.NoError(t, err)
	require.Contains(t, buf.String(), "Bob (no address)")
}

// TestUnit_TemplateFormat_MixedProtoAndFields shows using both proto types and fields
func TestUnit_TemplateFormat_MixedProtoAndFields(t *testing.T) {
	// Register custom function that works with proto types
	funcMap := template.FuncMap{
		"user": func(resp *simple.UserResponse) *simple.User {
			return resp.GetUser()
		},
		"formatTime": func(ts *timestamppb.Timestamp) string {
			if ts == nil || !ts.IsValid() {
				return "N/A"
			}
			return ts.AsTime().UTC().Format("2006-01-02")
		},
	}

	templates := map[string]string{
		// Mix proto accessor for timestamp (to use proto methods)
		// and protoFields for simple field access
		"example.UserResponse": `{{$f := protoFields .}}User: {{$f.user.name}}
Created: {{formatTime (user .).GetCreatedAt}}`,
	}

	format, err := protocli.TemplateFormat("user-mixed", templates, funcMap)
	require.NoError(t, err)

	createdAt := &timestamppb.Timestamp{
		Seconds: 1609459200, // 2021-01-01
		Nanos:   0,
	}

	msg := &simple.UserResponse{
		User: &simple.User{
			Id:        123,
			Name:      "Alice",
			Email:     "alice@example.com",
			CreatedAt: createdAt,
		},
	}

	var buf bytes.Buffer
	err = format.Format(context.Background(), &cli.Command{}, &buf, msg)
	require.NoError(t, err)

	output := buf.String()
	require.Contains(t, output, "User: Alice")
	require.Contains(t, output, "Created: 2021-01-01")
}

// TestUnit_TemplateFormat_ProtoJSONFunctions tests the protoJSON helper functions
func TestUnit_TemplateFormat_ProtoJSONFunctions(t *testing.T) {
	t.Run("protoJSON", func(t *testing.T) {
		templates := map[string]string{
			"example.UserResponse": `{{protoJSON .}}`,
		}

		format, err := protocli.TemplateFormat("json", templates)
		require.NoError(t, err)

		msg := &simple.UserResponse{
			User: &simple.User{
				Id:    123,
				Name:  "Alice",
				Email: "alice@example.com",
			},
		}

		var buf bytes.Buffer
		err = format.Format(context.Background(), &cli.Command{}, &buf, msg)
		require.NoError(t, err)

		output := buf.String()
		require.Contains(t, output, `"name":"Alice"`)
		require.Contains(t, output, `"id":"123"`)
	})

	t.Run("protoJSONIndent", func(t *testing.T) {
		templates := map[string]string{
			"example.UserResponse": `{{protoJSONIndent .}}`,
		}

		format, err := protocli.TemplateFormat("json-indent", templates)
		require.NoError(t, err)

		msg := &simple.UserResponse{
			User: &simple.User{
				Id:    123,
				Name:  "Alice",
				Email: "alice@example.com",
			},
		}

		var buf bytes.Buffer
		err = format.Format(context.Background(), &cli.Command{}, &buf, msg)
		require.NoError(t, err)

		output := buf.String()
		// Check for indented JSON (protojson may use different spacing)
		require.Contains(t, output, "\"name\"")
		require.Contains(t, output, "\"Alice\"")
		require.Contains(t, output, "\"id\"")
		require.Contains(t, output, "\"123\"")
		// Verify it's actually indented
		require.Contains(t, output, "\n  ")
	})
}

// TestUnit_TemplateFormat_GlobalRegistry tests the global template function registry
func TestUnit_TemplateFormat_GlobalRegistry(t *testing.T) {
	// Register a custom function globally
	protocli.TemplateFunctions().Register("exclaim", func(s string) string {
		return s + "!"
	})

	templates := map[string]string{
		"example.UserResponse": `{{$f := protoFields .}}{{exclaim $f.user.name}}`,
	}

	format, err := protocli.TemplateFormat("exclaim", templates)
	require.NoError(t, err)

	msg := &simple.UserResponse{
		User: &simple.User{
			Id:    123,
			Name:  "Alice",
			Email: "alice@example.com",
		},
	}

	var buf bytes.Buffer
	err = format.Format(context.Background(), &cli.Command{}, &buf, msg)
	require.NoError(t, err)

	require.Equal(t, "Alice!", buf.String())
}
