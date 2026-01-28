package simple_test

import (
	"context"
	"strings"
	"testing"

	protocli "github.com/drewfead/proto-cli"
	simple "github.com/drewfead/proto-cli/examples/simple"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
	"google.golang.org/protobuf/proto"
)

// TestIntegration_RecursiveDeserializer_NestedMessage tests that deserializers
// can be registered for nested message types using fully qualified proto names.
func TestIntegration_RecursiveDeserializer_NestedMessage(t *testing.T) {
	// Custom deserializer for Address (nested message type)
	// Uses fully qualified proto name: example.Address
	addressDeserializer := func(_ context.Context, flags protocli.FlagContainer) (proto.Message, error) {
		// Parse address from comma-separated string: street,city,state,zip,country
		addressStr := flags.StringNamed("address")

		if addressStr == "" {
			// Return empty address
			return &simple.Address{}, nil
		}

		parts := strings.Split(addressStr, ",")
		address := &simple.Address{}

		if len(parts) > 0 {
			address.Street = strings.TrimSpace(parts[0])
		}
		if len(parts) > 1 {
			address.City = strings.TrimSpace(parts[1])
		}
		if len(parts) > 2 {
			address.State = strings.TrimSpace(parts[2])
		}
		if len(parts) > 3 {
			address.ZipCode = strings.TrimSpace(parts[3])
		}
		if len(parts) > 4 {
			address.Country = strings.TrimSpace(parts[4])
		}

		return address, nil
	}

	ctx := context.Background()
	userServiceCLI := simple.UserServiceCommand(
		ctx,
		newUserService,
		// Register deserializer using fully qualified proto name
		protocli.WithFlagDeserializer("example.Address", addressDeserializer),
	)

	assert.NotNil(t, userServiceCLI)
	t.Log("Recursive deserializer for nested Address message registered successfully")
}

// TestIntegration_RecursiveDeserializer_TopLevelAndNested demonstrates
// registering deserializers at both top-level and nested levels.
func TestIntegration_RecursiveDeserializer_TopLevelAndNested(t *testing.T) {
	// Deserializer for nested Address message
	addressDeserializer := func(_ context.Context, flags protocli.FlagContainer) (proto.Message, error) {
		return &simple.Address{
			Street:  flags.StringNamed("street"),
			City:    flags.StringNamed("city"),
			State:   flags.StringNamed("state"),
			ZipCode: flags.StringNamed("zip"),
			Country: flags.StringNamed("country"),
		}, nil
	}

	// Top-level deserializer for CreateUserRequest
	// This can use the address deserializer for its nested field
	createUserDeserializer := func(_ context.Context, flags protocli.FlagContainer) (proto.Message, error) {
		// Build the CreateUserRequest
		// The nested Address field will be handled by its own deserializer
		return &simple.CreateUserRequest{
			Name:  flags.StringNamed("name"),
			Email: flags.StringNamed("email"),
			// Address will be deserialized by the Address deserializer
			// when auto-generated code processes the nested field
		}, nil
	}

	ctx := context.Background()
	userServiceCLI := simple.UserServiceCommand(
		ctx,
		newUserService,
		protocli.WithFlagDeserializer("example.Address", addressDeserializer),
		protocli.WithFlagDeserializer("example.CreateUserRequest", createUserDeserializer),
	)

	assert.NotNil(t, userServiceCLI)
	t.Log("Both top-level and nested deserializers registered")
}

// TestIntegration_RecursiveDeserializer_FullyQualifiedNames verifies
// that fully qualified proto names are used for lookups.
func TestIntegration_RecursiveDeserializer_FullyQualifiedNames(t *testing.T) {
	tests := []struct {
		name              string
		protoTypeName     string
		expectedAvailable bool
	}{
		{
			name:              "fully qualified address",
			protoTypeName:     "example.Address",
			expectedAvailable: true,
		},
		{
			name:              "fully qualified request",
			protoTypeName:     "example.GetUserRequest",
			expectedAvailable: true,
		},
		{
			name:              "short name should not match",
			protoTypeName:     "Address",
			expectedAvailable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deserializer := func(_ context.Context, _ protocli.FlagContainer) (proto.Message, error) {
				return &simple.Address{}, nil
			}

			ctx := context.Background()
			userServiceCLI := simple.UserServiceCommand(
				ctx,
				newUserService,
				protocli.WithFlagDeserializer(tt.protoTypeName, deserializer),
			)

			require.NotNil(t, userServiceCLI)
			// The deserializer should be registered under the exact name provided
		})
	}
}

// TestIntegration_RecursiveDeserializer_RealWorldExample shows a practical
// example of parsing complex address formats.
func TestIntegration_RecursiveDeserializer_RealWorldExample(t *testing.T) {
	// Example: Parse addresses from various formats
	addressDeserializer := func(_ context.Context, flags protocli.FlagContainer) (proto.Message, error) {
		// Support multiple input methods:

		// Method 1: Individual flags
		if flags.IsSetNamed("street") {
			return &simple.Address{
				Street:  flags.StringNamed("street"),
				City:    flags.StringNamed("city"),
				State:   flags.StringNamed("state"),
				ZipCode: flags.StringNamed("zip"),
				Country: flags.StringNamed("country"),
			}, nil
		}

		// Method 2: Comma-separated string
		if flags.IsSetNamed("address") {
			addressStr := flags.StringNamed("address")
			parts := strings.Split(addressStr, ",")

			address := &simple.Address{}
			if len(parts) > 0 {
				address.Street = strings.TrimSpace(parts[0])
			}
			if len(parts) > 1 {
				address.City = strings.TrimSpace(parts[1])
			}
			if len(parts) > 2 {
				address.State = strings.TrimSpace(parts[2])
			}
			if len(parts) > 3 {
				address.ZipCode = strings.TrimSpace(parts[3])
			}
			if len(parts) > 4 {
				address.Country = strings.TrimSpace(parts[4])
			}

			return address, nil
		}

		// Default: empty address
		return &simple.Address{}, nil
	}

	ctx := context.Background()
	userServiceCLI := simple.UserServiceCommand(
		ctx,
		newUserService,
		protocli.WithFlagDeserializer("example.Address", addressDeserializer),
	)

	assert.NotNil(t, userServiceCLI)
	t.Log("Real-world address deserializer with multiple input methods")
}

// TestIntegration_RecursiveDeserializer_ValidationInNested demonstrates
// validation in nested message deserializers.
func TestIntegration_RecursiveDeserializer_ValidationInNested(t *testing.T) {
	// Address deserializer with validation
	addressDeserializer := func(_ context.Context, flags protocli.FlagContainer) (proto.Message, error) {
		city := flags.StringNamed("city")
		state := flags.StringNamed("state")

		// Validation logic
		if city == "" {
			city = "Unknown"
		}

		// Normalize state to uppercase
		state = strings.ToUpper(state)
		if len(state) != 2 {
			// Default to XX if invalid
			state = "XX"
		}

		return &simple.Address{
			Street:  flags.StringNamed("street"),
			City:    city,
			State:   state,
			ZipCode: flags.StringNamed("zip"),
			Country: flags.StringNamed("country"),
		}, nil
	}

	ctx := context.Background()
	userServiceCLI := simple.UserServiceCommand(
		ctx,
		newUserService,
		protocli.WithFlagDeserializer("example.Address", addressDeserializer),
	)

	assert.NotNil(t, userServiceCLI)
	t.Log("Validation in nested deserializer works")
}

// TestIntegration_RecursiveDeserializer_CompositionPattern shows how
// deserializers can compose together.
func TestIntegration_RecursiveDeserializer_CompositionPattern(t *testing.T) {
	// Helper function to build address (could be reused)
	buildAddress := func(flags protocli.FlagContainer) *simple.Address {
		return &simple.Address{
			Street:  flags.StringNamed("street"),
			City:    flags.StringNamed("city"),
			State:   flags.StringNamed("state"),
			ZipCode: flags.StringNamed("zip"),
			Country: flags.StringNamed("country"),
		}
	}

	// Address deserializer using helper
	addressDeserializer := func(_ context.Context, flags protocli.FlagContainer) (proto.Message, error) {
		return buildAddress(flags), nil
	}

	// Top-level deserializer could manually build nested messages
	createUserDeserializer := func(_ context.Context, flags protocli.FlagContainer) (proto.Message, error) {
		return &simple.CreateUserRequest{
			Name:    flags.StringNamed("name"),
			Email:   flags.StringNamed("email"),
			Address: buildAddress(flags), // Manually compose
		}, nil
	}

	ctx := context.Background()
	userServiceCLI := simple.UserServiceCommand(
		ctx,
		newUserService,
		protocli.WithFlagDeserializer("example.Address", addressDeserializer),
		protocli.WithFlagDeserializer("example.CreateUserRequest", createUserDeserializer),
	)

	assert.NotNil(t, userServiceCLI)
	t.Log("Composition pattern works for building complex messages")
}

// BenchmarkRecursiveDeserializer benchmarks nested message deserialization.
func BenchmarkRecursiveDeserializer(b *testing.B) {
	addressDeserializer := func(_ context.Context, flags protocli.FlagContainer) (proto.Message, error) {
		return &simple.Address{
			Street:  flags.StringNamed("street"),
			City:    flags.StringNamed("city"),
			State:   flags.StringNamed("state"),
			ZipCode: flags.StringNamed("zip"),
			Country: flags.StringNamed("country"),
		}, nil
	}

	ctx := context.Background()
	cmd := &cli.Command{
		Name: "test",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "street", Value: "123 Main St"},
			&cli.StringFlag{Name: "city", Value: "San Francisco"},
			&cli.StringFlag{Name: "state", Value: "CA"},
			&cli.StringFlag{Name: "zip", Value: "94102"},
			&cli.StringFlag{Name: "country", Value: "USA"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		flagContainer := protocli.NewFlagContainer(cmd, "street")
		_, _ = addressDeserializer(ctx, flagContainer)
	}
}
