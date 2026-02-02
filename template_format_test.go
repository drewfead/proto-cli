package protocli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"text/template"

	protocli "github.com/drewfead/proto-cli"
	"github.com/drewfead/proto-cli/examples/simple"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestUnit_TemplateFormat_BasicRendering(t *testing.T) {
	templates := map[string]string{
		"example.UserResponse": `Name: {{.user.name}}
Email: {{.user.email}}
ID: {{.user.id}}`,
	}

	format, err := protocli.TemplateFormat("user-info", templates)
	require.NoError(t, err)
	require.Equal(t, "user-info", format.Name())

	// Create test message
	msg := &simple.UserResponse{
		User: &simple.User{
			Id:    123,
			Name:  "Alice Smith",
			Email: "alice@example.com",
		},
	}

	// Render
	var buf bytes.Buffer
	err = format.Format(context.Background(), &cli.Command{}, &buf, msg)
	require.NoError(t, err)

	expected := `Name: Alice Smith
Email: alice@example.com
ID: 123`
	require.Equal(t, expected, buf.String())
}

func TestUnit_TemplateFormat_ConditionalLogic(t *testing.T) {
	templates := map[string]string{
		"example.UserResponse": `{{.user.name}}{{if .user.address}} (has address){{else}} (no address){{end}}`,
	}

	format, err := protocli.TemplateFormat("user-status", templates)
	require.NoError(t, err)

	// Test with address
	withAddress := &simple.UserResponse{
		User: &simple.User{
			Id:   1,
			Name: "Bob",
			Address: &simple.Address{
				Street: "123 Main St",
				City:   "Springfield",
			},
		},
	}

	var buf bytes.Buffer
	err = format.Format(context.Background(), &cli.Command{}, &buf, withAddress)
	require.NoError(t, err)
	require.Equal(t, "Bob (has address)", buf.String())

	// Test without address
	withoutAddress := &simple.UserResponse{
		User: &simple.User{
			Id:   2,
			Name: "Charlie",
		},
	}

	buf.Reset()
	err = format.Format(context.Background(), &cli.Command{}, &buf, withoutAddress)
	require.NoError(t, err)
	require.Equal(t, "Charlie (no address)", buf.String())
}

func TestUnit_TemplateFormat_Conditionals(t *testing.T) {
	templates := map[string]string{
		"example.CreateUserRequest": `Name: {{.name}}
Email: {{.email}}{{if .phoneNumber}}
Phone: {{.phoneNumber}}{{end}}{{if .nickname}}
Nickname: {{.nickname}}{{end}}`,
	}

	format, err := protocli.TemplateFormat("user-detail", templates)
	require.NoError(t, err)

	msg := &simple.CreateUserRequest{
		Name:        "John Doe",
		Email:       "john@example.com",
		PhoneNumber: "555-1234",
		Nickname:    strPtr("Johnny"),
	}

	var buf bytes.Buffer
	err = format.Format(context.Background(), &cli.Command{}, &buf, msg)
	require.NoError(t, err)

	output := buf.String()
	require.Contains(t, output, "Name: John Doe")
	require.Contains(t, output, "Email: john@example.com")
	require.Contains(t, output, "Phone: 555-1234")
	require.Contains(t, output, "Nickname: Johnny")
}

func TestUnit_TemplateFormat_CustomFunctions(t *testing.T) {
	templates := map[string]string{
		"example.UserResponse": `{{upper .user.name}} - {{lower .user.email}}`,
	}

	funcMap := template.FuncMap{
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
	}

	format, err := protocli.TemplateFormat("user-case", templates, funcMap)
	require.NoError(t, err)

	msg := &simple.UserResponse{
		User: &simple.User{
			Id:    1,
			Name:  "Alice Smith",
			Email: "ALICE@EXAMPLE.COM",
		},
	}

	var buf bytes.Buffer
	err = format.Format(context.Background(), &cli.Command{}, &buf, msg)
	require.NoError(t, err)
	require.Equal(t, "ALICE SMITH - alice@example.com", buf.String())
}

func TestUnit_TemplateFormat_MissingMessageType(t *testing.T) {
	templates := map[string]string{
		"example.OtherMessage": `{{.field}}`,
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

func TestUnit_TemplateFormat_InvalidTemplate(t *testing.T) {
	templates := map[string]string{
		"example.UserResponse": `{{.user.name`,
	}

	_, err := protocli.TemplateFormat("invalid", templates)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to parse template")
}

func TestUnit_TemplateFormat_TableFormat(t *testing.T) {
	templates := map[string]string{
		"example.UserResponse": `┌────────────────────────────────────────┐
│ User Information                       │
├────────────────────────────────────────┤
│ ID:    {{printf "%-31d" .user.id}} │
│ Name:  {{printf "%-31s" .user.name}} │
│ Email: {{printf "%-31s" .user.email}} │
└────────────────────────────────────────┘`,
	}

	format, err := protocli.TemplateFormat("user-table", templates)
	require.NoError(t, err)

	msg := &simple.UserResponse{
		User: &simple.User{
			Id:    42,
			Name:  "John Doe",
			Email: "john@example.com",
		},
	}

	var buf bytes.Buffer
	err = format.Format(context.Background(), &cli.Command{}, &buf, msg)
	require.NoError(t, err)

	output := buf.String()
	require.Contains(t, output, "John Doe")
	require.Contains(t, output, "john@example.com")
	require.Contains(t, output, "│ ID:")
}

func TestUnit_TemplateFormat_OptionalFields(t *testing.T) {
	templates := map[string]string{
		"example.CreateUserRequest": `Name: {{.name}}{{if .nickname}}
Nickname: {{.nickname}}{{end}}{{if .age}}
Age: {{.age}}{{end}}`,
	}

	format, err := protocli.TemplateFormat("user-optional", templates)
	require.NoError(t, err)

	// Test with optional fields set
	withOptional := &simple.CreateUserRequest{
		Name:     "Alice",
		Email:    "alice@example.com",
		Nickname: strPtr("Ace"),
		Age:      int32Ptr(30),
	}

	var buf bytes.Buffer
	err = format.Format(context.Background(), &cli.Command{}, &buf, withOptional)
	require.NoError(t, err)
	require.Contains(t, buf.String(), "Nickname: Ace")
	require.Contains(t, buf.String(), "Age: 30")

	// Test without optional fields
	withoutOptional := &simple.CreateUserRequest{
		Name:  "Bob",
		Email: "bob@example.com",
	}

	buf.Reset()
	err = format.Format(context.Background(), &cli.Command{}, &buf, withoutOptional)
	require.NoError(t, err)
	require.NotContains(t, buf.String(), "Nickname:")
	require.NotContains(t, buf.String(), "Age:")
}

func TestUnit_TemplateFormat_OptionalEnumField(t *testing.T) {
	templates := map[string]string{
		"example.CreateUserRequest": `Name: {{.name}}{{if .logLevel}}
LogLevel: {{.logLevel}}{{end}}`,
	}

	format, err := protocli.TemplateFormat("user-enum", templates)
	require.NoError(t, err)

	// Test with optional enum field set
	logLevel := simple.LogLevel_DEBUG
	withEnum := &simple.CreateUserRequest{
		Name:     "Alice",
		Email:    "alice@example.com",
		LogLevel: &logLevel,
	}

	var buf bytes.Buffer
	err = format.Format(context.Background(), &cli.Command{}, &buf, withEnum)
	require.NoError(t, err)
	require.Contains(t, buf.String(), "LogLevel: DEBUG") // Enum is printed as name

	// Test without optional enum field
	withoutEnum := &simple.CreateUserRequest{
		Name:  "Bob",
		Email: "bob@example.com",
	}

	buf.Reset()
	err = format.Format(context.Background(), &cli.Command{}, &buf, withoutEnum)
	require.NoError(t, err)
	require.NotContains(t, buf.String(), "LogLevel:")
}

func TestUnit_MustTemplateFormat_Success(t *testing.T) {
	templates := map[string]string{
		"example.UserResponse": `{{.user.name}}`,
	}

	// Should not panic
	format := protocli.MustTemplateFormat("test", templates)
	require.NotNil(t, format)
	require.Equal(t, "test", format.Name())
}

func TestUnit_MustTemplateFormat_Panics(t *testing.T) {
	templates := map[string]string{
		"example.UserResponse": `{{.user.name`,
	}

	require.Panics(t, func() {
		protocli.MustTemplateFormat("invalid", templates)
	})
}

func TestUnit_TemplateFormat_MultipleFuncMaps(t *testing.T) {
	templates := map[string]string{
		"example.UserResponse": `{{upper .user.name}} - {{reverse .user.email}}`,
	}

	funcMap1 := template.FuncMap{
		"upper": strings.ToUpper,
	}

	funcMap2 := template.FuncMap{
		"reverse": func(s string) string {
			runes := []rune(s)
			for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
				runes[i], runes[j] = runes[j], runes[i]
			}
			return string(runes)
		},
	}

	format, err := protocli.TemplateFormat("multi-func", templates, funcMap1, funcMap2)
	require.NoError(t, err)

	msg := &simple.UserResponse{
		User: &simple.User{
			Id:    1,
			Name:  "alice",
			Email: "test@example.com",
		},
	}

	var buf bytes.Buffer
	err = format.Format(context.Background(), &cli.Command{}, &buf, msg)
	require.NoError(t, err)
	require.Equal(t, "ALICE - moc.elpmaxe@tset", buf.String())
}

func TestUnit_TemplateFormat_CompactFormat(t *testing.T) {
	templates := map[string]string{
		"example.UserResponse": `{{.user.name}} <{{.user.email}}>`,
	}

	format, err := protocli.TemplateFormat("compact", templates)
	require.NoError(t, err)

	msg := &simple.UserResponse{
		User: &simple.User{
			Id:    1,
			Name:  "Jane Doe",
			Email: "jane@example.com",
		},
	}

	var buf bytes.Buffer
	err = format.Format(context.Background(), &cli.Command{}, &buf, msg)
	require.NoError(t, err)
	require.Equal(t, "Jane Doe <jane@example.com>", buf.String())
}

func TestUnit_TemplateFormat_CSVLikeOutput(t *testing.T) {
	templates := map[string]string{
		"example.UserResponse": `{{.user.id}},{{.user.name}},{{.user.email}}`,
	}

	format, err := protocli.TemplateFormat("csv", templates)
	require.NoError(t, err)

	msg := &simple.UserResponse{
		User: &simple.User{
			Id:    100,
			Name:  "Test User",
			Email: "test@example.com",
		},
	}

	var buf bytes.Buffer
	err = format.Format(context.Background(), &cli.Command{}, &buf, msg)
	require.NoError(t, err)
	require.Equal(t, "100,Test User,test@example.com", buf.String())
}

func TestUnit_TemplateFormat_NestedFields(t *testing.T) {
	templates := map[string]string{
		"example.UserResponse": `User: {{.user.name}}{{if .user.address}}
Address:
  Street: {{.user.address.street}}
  City: {{.user.address.city}}
  State: {{.user.address.state}}
  Zip: {{.user.address.zipCode}}{{end}}`,
	}

	format, err := protocli.TemplateFormat("nested", templates)
	require.NoError(t, err)

	msg := &simple.UserResponse{
		User: &simple.User{
			Id:   1,
			Name: "Alice",
			Address: &simple.Address{
				Street:  "123 Main St",
				City:    "Springfield",
				State:   "IL",
				ZipCode: "62701",
			},
		},
	}

	var buf bytes.Buffer
	err = format.Format(context.Background(), &cli.Command{}, &buf, msg)
	require.NoError(t, err)

	output := buf.String()
	require.Contains(t, output, "User: Alice")
	require.Contains(t, output, "Street: 123 Main St")
	require.Contains(t, output, "City: Springfield")
}

// Helper functions
func strPtr(s string) *string {
	return &s
}

func int32Ptr(i int32) *int32 {
	return &i
}

// Example usage documentation test
func Example_templateFormat() {
	// Define templates for message types
	templates := map[string]string{
		"example.UserResponse": `User: {{.user.name}} (ID: {{.user.id}})
Email: {{.user.email}}
Status: {{if .user.verified}}Verified{{else}}Unverified{{end}}`,
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
