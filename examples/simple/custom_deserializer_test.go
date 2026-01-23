package simple_test

import (
	"context"
	"testing"

	protocli "github.com/drewfead/proto-cli"
	simple "github.com/drewfead/proto-cli/examples/simple"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
	"google.golang.org/protobuf/proto"
)

// TestIntegration_CustomFlagDeserializer demonstrates using custom flag deserializers
// to transform CLI flags into complex proto messages
func TestIntegration_CustomFlagDeserializer(t *testing.T) {
	// Custom deserializer that builds GetUserRequest from a user-id flag
	// This example shows how you can add custom parsing logic, validation, etc.
	customDeserializer := func(ctx context.Context, cmd *cli.Command) (proto.Message, error) {
		userId := cmd.Int("id")

		// Custom logic: validate user ID
		if userId <= 0 {
			userId = 1 // Default to user 1
		}

		// Build the request with custom logic
		return &simple.GetUserRequest{
			Id: int64(userId),
		}, nil
	}

	// Create service with custom deserializer
	ctx := context.Background()
	userServiceCLI := simple.UserServiceServiceCommand(
		ctx,
		newUserService,
		protocli.WithFlagDeserializer("GetUserRequest", customDeserializer),
		protocli.WithOutputFormats(
			protocli.JSON(),
		),
	)

	// Verify the service CLI was created
	assert.NotNil(t, userServiceCLI)
	assert.NotNil(t, userServiceCLI.Command)

	// The custom deserializer is now registered and will be used
	// when the getuser command is executed
	t.Log("Custom flag deserializer registered successfully")
}

// TestIntegration_CustomDeserializer_ComplexTransformation demonstrates more complex use cases
func TestIntegration_CustomDeserializer_ComplexTransformation(t *testing.T) {
	// Example: Parse a comma-separated list or JSON string into a proto message
	complexDeserializer := func(ctx context.Context, cmd *cli.Command) (proto.Message, error) {
		name := cmd.String("name")
		email := cmd.String("email")

		// Could add validation here
		if name == "" {
			name = "Default User"
		}
		if email == "" {
			email = "default@example.com"
		}

		return &simple.CreateUserRequest{
			Name:  name,
			Email: email,
		}, nil
	}

	ctx := context.Background()
	userServiceCLI := simple.UserServiceServiceCommand(
		ctx,
		newUserService,
		protocli.WithFlagDeserializer("CreateUserRequest", complexDeserializer),
	)

	assert.NotNil(t, userServiceCLI)
	t.Log("Complex deserializer registered successfully")
}

// TestIntegration_CustomDeserializer_WithValidation shows validation in deserializer
func TestIntegration_CustomDeserializer_WithValidation(t *testing.T) {
	tests := []struct {
		name        string
		deserializer protocli.FlagDeserializer
		expectValid bool
	}{
		{
			name: "valid input",
			deserializer: func(ctx context.Context, cmd *cli.Command) (proto.Message, error) {
				userId := cmd.Int("id")
				if userId <= 0 {
					return nil, assert.AnError
				}
				return &simple.GetUserRequest{Id: int64(userId)}, nil
			},
			expectValid: true,
		},
		{
			name: "with default values",
			deserializer: func(ctx context.Context, cmd *cli.Command) (proto.Message, error) {
				userId := cmd.Int("id")
				if userId <= 0 {
					userId = 1 // Apply default
				}
				return &simple.GetUserRequest{Id: int64(userId)}, nil
			},
			expectValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			userServiceCLI := simple.UserServiceServiceCommand(
				ctx,
				newUserService,
				protocli.WithFlagDeserializer("GetUserRequest", tt.deserializer),
			)

			require.NotNil(t, userServiceCLI)
			if tt.expectValid {
				assert.NotNil(t, userServiceCLI.Command)
			}
		})
	}
}

// Example use case: Parsing JSON from a flag into a complex message
func TestIntegration_CustomDeserializer_JSONParsing(t *testing.T) {
	jsonDeserializer := func(ctx context.Context, cmd *cli.Command) (proto.Message, error) {
		// In a real scenario, you might have a --json flag
		// that contains the entire request as JSON
		// Example: --json '{"name":"John","email":"john@example.com"}'

		// For this test, we'll just show the pattern
		name := cmd.String("name")
		email := cmd.String("email")

		// Could parse JSON here:
		// var req CreateUserRequest
		// json.Unmarshal([]byte(cmd.String("json")), &req)

		return &simple.CreateUserRequest{
			Name:  name,
			Email: email,
		}, nil
	}

	ctx := context.Background()
	userServiceCLI := simple.UserServiceServiceCommand(
		ctx,
		newUserService,
		protocli.WithFlagDeserializer("CreateUserRequest", jsonDeserializer),
	)

	assert.NotNil(t, userServiceCLI)
	t.Log("JSON deserializer pattern demonstrated")
}

// Example: Multiple deserializers for different message types
func TestIntegration_MultipleCustomDeserializers(t *testing.T) {
	getUserDeserializer := func(ctx context.Context, cmd *cli.Command) (proto.Message, error) {
		return &simple.GetUserRequest{
			Id: int64(cmd.Int("id")),
		}, nil
	}

	createUserDeserializer := func(ctx context.Context, cmd *cli.Command) (proto.Message, error) {
		return &simple.CreateUserRequest{
			Name:  cmd.String("name"),
			Email: cmd.String("email"),
		}, nil
	}

	ctx := context.Background()
	userServiceCLI := simple.UserServiceServiceCommand(
		ctx,
		newUserService,
		protocli.WithFlagDeserializer("GetUserRequest", getUserDeserializer),
		protocli.WithFlagDeserializer("CreateUserRequest", createUserDeserializer),
	)

	assert.NotNil(t, userServiceCLI)
	assert.NotNil(t, userServiceCLI.Command)
	t.Log("Multiple deserializers registered successfully")
}

// Benchmarks

func BenchmarkCustomDeserializer(b *testing.B) {
	deserializer := func(ctx context.Context, cmd *cli.Command) (proto.Message, error) {
		return &simple.GetUserRequest{
			Id: int64(cmd.Int("id")),
		}, nil
	}

	ctx := context.Background()
	cmd := &cli.Command{
		Name: "test",
		Flags: []cli.Flag{
			&cli.IntFlag{Name: "id", Value: 123},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = deserializer(ctx, cmd)
	}
}

func BenchmarkAutoGeneratedParsing(b *testing.B) {
	// Simulates auto-generated flag parsing
	cmd := &cli.Command{
		Name: "test",
		Flags: []cli.Flag{
			&cli.IntFlag{Name: "id", Value: 123},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := &simple.GetUserRequest{
			Id: int64(cmd.Int("id")),
		}
		_ = req
	}
}
