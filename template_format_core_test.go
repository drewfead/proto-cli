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
)

// TestUnit_TemplateFormat_BasicRendering demonstrates the recommended pattern with custom accessors
func TestUnit_TemplateFormat_BasicRendering(t *testing.T) {
	// Register custom accessor for cleaner templates
	funcMap := template.FuncMap{
		"user": func(resp *simple.UserResponse) *simple.User {
			return resp.GetUser()
		},
	}

	templates := map[string]string{
		"example.UserResponse": `Name: {{(user .).GetName}}
Email: {{(user .).GetEmail}}
ID: {{(user .).GetId}}`,
	}

	format, err := protocli.TemplateFormat("user-info", templates, funcMap)
	require.NoError(t, err)
	require.Equal(t, "user-info", format.Name())

	// Create test message
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

// TestUnit_TemplateFormat_MissingMessageType tests error when template not found for message type
func TestUnit_TemplateFormat_MissingMessageType(t *testing.T) {
	templates := map[string]string{
		"example.OtherMessage": `{{$f := protoFields .}}{{$f.field}}`,
	}

	format, err := protocli.TemplateFormat("test", templates)
	require.NoError(t, err)

	msg := &simple.UserResponse{
		User: &simple.User{Id: 1, Name: "Test"},
	}

	var buf bytes.Buffer
	err = format.Format(context.Background(), &cli.Command{}, &buf, msg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no template registered for message type")
	require.Contains(t, err.Error(), "example.UserResponse")
	require.Contains(t, err.Error(), "example.OtherMessage")
}

// TestUnit_TemplateFormat_InvalidTemplate tests error on invalid template syntax
func TestUnit_TemplateFormat_InvalidTemplate(t *testing.T) {
	templates := map[string]string{
		"example.UserResponse": `{{.unclosed`,
	}

	_, err := protocli.TemplateFormat("test", templates)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to parse template")
}

// TestUnit_MustTemplateFormat_Success tests MustTemplateFormat with valid template
func TestUnit_MustTemplateFormat_Success(t *testing.T) {
	templates := map[string]string{
		"example.UserResponse": `{{$f := protoFields .}}{{$f.user.name}}`,
	}

	// Should not panic
	require.NotPanics(t, func() {
		format := protocli.MustTemplateFormat("test", templates)
		require.NotNil(t, format)
		require.Equal(t, "test", format.Name())
	})
}

// TestUnit_MustTemplateFormat_Panics tests MustTemplateFormat panics on error
func TestUnit_MustTemplateFormat_Panics(t *testing.T) {
	templates := map[string]string{
		"example.UserResponse": `{{.unclosed`,
	}

	require.Panics(t, func() {
		_ = protocli.MustTemplateFormat("test", templates)
	})
}

// Example_templateFormat demonstrates template format usage
func Example_templateFormat() {
	// Define templates for message types
	templates := map[string]string{
		"example.UserResponse": `{{$f := protoFields .}}User: {{$f.user.name}} (ID: {{$f.user.id}})
Email: {{$f.user.email}}
Status: {{if $f.user.verified}}Verified{{else}}Unverified{{end}}`,
	}

	// Create the format
	format, err := protocli.TemplateFormat("user-detail", templates)
	if err != nil {
		panic(err)
	}

	// Use in CLI
	_ = format // protocli.WithOutputFormats(format)

	// Output:
}
