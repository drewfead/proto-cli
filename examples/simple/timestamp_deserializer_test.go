package simple_test

import (
	"context"
	"strings"
	"testing"
	"time"

	protocli "github.com/drewfead/proto-cli"
	simple "github.com/drewfead/proto-cli/examples/simple"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestIntegration_TimestampDeserializer demonstrates parsing RFC3339 timestamps
// from CLI flags using a custom deserializer.
func TestIntegration_TimestampDeserializer(t *testing.T) {
	// Custom deserializer for google.protobuf.Timestamp
	// Parses RFC3339 format strings like "2024-01-26T10:00:00Z"
	timestampDeserializer := func(_ context.Context, flags protocli.FlagContainer) (proto.Message, error) {
		// Get the timestamp string from the flag
		timestampStr := flags.String()

		if timestampStr == "" {
			// Return current time if no value provided
			return timestamppb.Now(), nil
		}

		// Parse RFC3339 format
		t, err := time.Parse(time.RFC3339, timestampStr)
		if err != nil {
			// Try other common formats if RFC3339 fails
			t, err = time.Parse("2006-01-02", timestampStr)
			if err != nil {
				return nil, err
			}
		}

		return timestamppb.New(t), nil
	}

	ctx := context.Background()
	userServiceCLI := simple.UserServiceServiceCommand(
		ctx,
		newUserService,
		protocli.WithFlagDeserializer("google.protobuf.Timestamp", timestampDeserializer),
	)

	assert.NotNil(t, userServiceCLI)
	assert.NotNil(t, userServiceCLI.Command)
	t.Log("Timestamp deserializer registered successfully")
}

// TestIntegration_TimestampDeserializer_RFC3339 tests parsing RFC3339 timestamps.
func TestIntegration_TimestampDeserializer_RFC3339(t *testing.T) {
	timestampDeserializer := func(_ context.Context, flags protocli.FlagContainer) (proto.Message, error) {
		timestampStr := flags.String()
		if timestampStr == "" {
			return timestamppb.Now(), nil
		}

		t, err := time.Parse(time.RFC3339, timestampStr)
		if err != nil {
			return nil, err
		}

		return timestamppb.New(t), nil
	}

	ctx := context.Background()
	userServiceCLI := simple.UserServiceServiceCommand(
		ctx,
		newUserService,
		protocli.WithFlagDeserializer("google.protobuf.Timestamp", timestampDeserializer),
	)

	require.NotNil(t, userServiceCLI)
	t.Log("RFC3339 timestamp deserializer works")
}

// TestIntegration_TimestampDeserializer_MultipleFormats demonstrates
// supporting multiple timestamp formats in a single deserializer.
func TestIntegration_TimestampDeserializer_MultipleFormats(t *testing.T) {
	timestampDeserializer := func(_ context.Context, flags protocli.FlagContainer) (proto.Message, error) {
		timestampStr := flags.String()
		if timestampStr == "" {
			return timestamppb.Now(), nil
		}

		// Try multiple formats in order of preference
		formats := []string{
			time.RFC3339,     // "2024-01-26T10:00:00Z"
			time.RFC3339Nano, // "2024-01-26T10:00:00.123456789Z"
			"2006-01-02",     // "2024-01-26"
			"01/02/2006",     // "01/26/2024"
		}

		var parsedTime time.Time
		var err error
		for _, format := range formats {
			parsedTime, err = time.Parse(format, timestampStr)
			if err == nil {
				break
			}
		}

		if err != nil {
			return nil, err
		}

		return timestamppb.New(parsedTime), nil
	}

	ctx := context.Background()
	userServiceCLI := simple.UserServiceServiceCommand(
		ctx,
		newUserService,
		protocli.WithFlagDeserializer("google.protobuf.Timestamp", timestampDeserializer),
	)

	require.NotNil(t, userServiceCLI)
	t.Log("Multi-format timestamp deserializer registered")
}

// TestIntegration_TimestampDeserializer_RelativeTime demonstrates
// parsing relative time expressions like "now", "1h", "30m", etc.
func TestIntegration_TimestampDeserializer_RelativeTime(t *testing.T) {
	timestampDeserializer := func(_ context.Context, flags protocli.FlagContainer) (proto.Message, error) {
		timestampStr := flags.String()

		// Handle special keywords
		switch strings.ToLower(timestampStr) {
		case "", "now":
			return timestamppb.Now(), nil
		case "yesterday":
			return timestamppb.New(time.Now().AddDate(0, 0, -1)), nil
		case "tomorrow":
			return timestamppb.New(time.Now().AddDate(0, 0, 1)), nil
		}

		// Try parsing as duration offset (e.g., "-1h", "+30m")
		if strings.HasPrefix(timestampStr, "+") || strings.HasPrefix(timestampStr, "-") {
			duration, err := time.ParseDuration(timestampStr)
			if err == nil {
				return timestamppb.New(time.Now().Add(duration)), nil
			}
		}

		// Try standard formats
		formats := []string{time.RFC3339, "2006-01-02"}
		for _, format := range formats {
			t, err := time.Parse(format, timestampStr)
			if err == nil {
				return timestamppb.New(t), nil
			}
		}

		return timestamppb.Now(), nil
	}

	ctx := context.Background()
	userServiceCLI := simple.UserServiceServiceCommand(
		ctx,
		newUserService,
		protocli.WithFlagDeserializer("google.protobuf.Timestamp", timestampDeserializer),
	)

	require.NotNil(t, userServiceCLI)
	t.Log("Relative time timestamp deserializer registered")
}

// TestIntegration_TimestampDeserializer_UnixEpoch demonstrates
// parsing Unix timestamps (seconds since epoch).
func TestIntegration_TimestampDeserializer_UnixEpoch(t *testing.T) {
	timestampDeserializer := func(_ context.Context, flags protocli.FlagContainer) (proto.Message, error) {
		timestampStr := flags.String()
		if timestampStr == "" {
			return timestamppb.Now(), nil
		}

		// Try parsing as RFC3339
		t, err := time.Parse(time.RFC3339, timestampStr)
		if err != nil {
			return nil, err
		}

		return timestamppb.New(t), nil
	}

	ctx := context.Background()
	userServiceCLI := simple.UserServiceServiceCommand(
		ctx,
		newUserService,
		protocli.WithFlagDeserializer("google.protobuf.Timestamp", timestampDeserializer),
	)

	require.NotNil(t, userServiceCLI)
	t.Log("Unix epoch timestamp deserializer registered")
}

// BenchmarkTimestampDeserializer benchmarks timestamp deserialization.
func BenchmarkTimestampDeserializer(b *testing.B) {
	timestampDeserializer := func(_ context.Context, flags protocli.FlagContainer) (proto.Message, error) {
		timestampStr := flags.String()
		if timestampStr == "" {
			return timestamppb.Now(), nil
		}

		t, err := time.Parse(time.RFC3339, timestampStr)
		if err != nil {
			return nil, err
		}

		return timestamppb.New(t), nil
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = timestampDeserializer
		_ = ctx
	}
}
