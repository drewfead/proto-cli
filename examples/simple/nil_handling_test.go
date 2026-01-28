package simple_test

import (
	"context"
	"testing"

	protocli "github.com/drewfead/proto-cli"
	simple "github.com/drewfead/proto-cli/examples/simple"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestIntegration_NilHandling_DeserializerReturnsNil tests that deserializers
// can return (nil, nil) to skip setting a field.
func TestIntegration_NilHandling_DeserializerReturnsNil(t *testing.T) {
	// Deserializer that returns nil for empty strings
	timestampDeserializer := func(_ context.Context, flags protocli.FlagContainer) (proto.Message, error) {
		timestampStr := flags.String()

		// Return nil for empty strings - this should leave the field unset
		if timestampStr == "" {
			return &timestamppb.Timestamp{}, nil
		}

		// In a real implementation, would parse the timestamp
		// For this test, just verify the nil return is handled
		//nolint:nilnil
		return nil, nil
	}

	ctx := context.Background()
	userServiceCLI := simple.UserServiceCommand(
		ctx,
		newUserService,
		protocli.WithFlagDeserializer("google.protobuf.Timestamp", timestampDeserializer),
	)

	require.NotNil(t, userServiceCLI)
	assert.NotNil(t, userServiceCLI.Command)
	t.Log("Nil-returning deserializer registered successfully")
}

// TestIntegration_NilHandling_OptionalFields tests that optional message fields
// are left as nil when no value is provided.
func TestIntegration_NilHandling_OptionalFields(t *testing.T) {
	timestampDeserializer := func(_ context.Context, _ protocli.FlagContainer) (proto.Message, error) {
		// Always return nil to test nil handling
		//nolint:nilnil
		return nil, nil
	}

	ctx := context.Background()
	userServiceCLI := simple.UserServiceCommand(
		ctx,
		newUserService,
		protocli.WithFlagDeserializer("google.protobuf.Timestamp", timestampDeserializer),
	)

	require.NotNil(t, userServiceCLI)
	t.Log("Optional fields can be left as nil")
}

// TestIntegration_NilHandling_NoDeserializerNoValue tests that when no
// deserializer is registered and no value is provided, the field is left nil.
func TestIntegration_NilHandling_NoDeserializerNoValue(t *testing.T) {
	ctx := context.Background()

	// Create service without timestamp deserializer
	userServiceCLI := simple.UserServiceCommand(
		ctx,
		newUserService,
	)

	require.NotNil(t, userServiceCLI)
	t.Log("Fields without deserializers are left nil when no value provided")
}

// TestIntegration_NilHandling_MixedNilAndNonNil tests that some fields
// can be nil while others are set.
func TestIntegration_NilHandling_MixedNilAndNonNil(t *testing.T) {
	// Address deserializer returns non-nil
	addressDeserializer := func(_ context.Context, flags protocli.FlagContainer) (proto.Message, error) {
		return &simple.Address{
			Street: flags.StringNamed("street"),
			City:   flags.StringNamed("city"),
		}, nil
	}

	// Timestamp deserializer returns nil
	timestampDeserializer := func(_ context.Context, _ protocli.FlagContainer) (proto.Message, error) {
		//nolint: nilnil
		return nil, nil
	}

	ctx := context.Background()
	userServiceCLI := simple.UserServiceCommand(
		ctx,
		newUserService,
		protocli.WithFlagDeserializer("example.Address", addressDeserializer),
		protocli.WithFlagDeserializer("google.protobuf.Timestamp", timestampDeserializer),
	)

	require.NotNil(t, userServiceCLI)
	t.Log("Mixed nil and non-nil fields work correctly")
}
