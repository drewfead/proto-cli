// Package deserializers provides reusable FlagDeserializer factories for well-known protobuf types.
//
// Each factory returns a protocli.FlagDeserializer that converts CLI string flags into the
// corresponding protobuf message. Empty strings produce zero-value messages.
//
// Use All() to register all deserializers at once:
//
//	svc := proto.FooServiceCommand(ctx, impl, deserializers.All()...)
package deserializers

import (
	"context"
	"fmt"
	"strings"
	"time"

	protocli "github.com/drewfead/proto-cli"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Timestamp returns a FlagDeserializer that parses RFC3339 strings into google.protobuf.Timestamp.
// An empty string produces a zero-value Timestamp.
func Timestamp() protocli.FlagDeserializer {
	return func(_ context.Context, flags protocli.FlagContainer) (proto.Message, error) {
		s := flags.String()
		if s == "" {
			return &timestamppb.Timestamp{}, nil
		}

		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return nil, fmt.Errorf("invalid RFC3339 timestamp %q: %w", s, err)
		}

		return timestamppb.New(t), nil
	}
}

// Duration returns a FlagDeserializer that parses Go duration strings (e.g. "5m30s")
// into google.protobuf.Duration. An empty string produces a zero-value Duration.
func Duration() protocli.FlagDeserializer {
	return func(_ context.Context, flags protocli.FlagContainer) (proto.Message, error) {
		s := flags.String()
		if s == "" {
			return &durationpb.Duration{}, nil
		}

		d, err := time.ParseDuration(s)
		if err != nil {
			return nil, fmt.Errorf("invalid duration %q: %w", s, err)
		}

		return durationpb.New(d), nil
	}
}

// FieldMask returns a FlagDeserializer that parses comma-separated field paths
// into google.protobuf.FieldMask. An empty string produces an empty FieldMask.
func FieldMask() protocli.FlagDeserializer {
	return func(_ context.Context, flags protocli.FlagContainer) (proto.Message, error) {
		s := flags.String()
		if s == "" {
			return &fieldmaskpb.FieldMask{}, nil
		}

		paths := strings.Split(s, ",")
		for i, p := range paths {
			paths[i] = strings.TrimSpace(p)
		}

		return &fieldmaskpb.FieldMask{Paths: paths}, nil
	}
}

// Struct returns a FlagDeserializer that parses JSON strings into google.protobuf.Struct
// using protojson. An empty string produces an empty Struct.
func Struct() protocli.FlagDeserializer {
	return func(_ context.Context, flags protocli.FlagContainer) (proto.Message, error) {
		s := flags.String()
		if s == "" {
			return &structpb.Struct{}, nil
		}

		st := &structpb.Struct{}
		if err := protojson.Unmarshal([]byte(s), st); err != nil {
			return nil, fmt.Errorf("invalid JSON for Struct %q: %w", s, err)
		}

		return st, nil
	}
}

// BoolValue returns a FlagDeserializer for google.protobuf.BoolValue.
// An empty string produces a zero-value BoolValue (false).
func BoolValue() protocli.FlagDeserializer {
	return func(_ context.Context, flags protocli.FlagContainer) (proto.Message, error) {
		s := flags.String()
		if s == "" {
			return &wrapperspb.BoolValue{}, nil
		}

		w := &wrapperspb.BoolValue{}
		if err := protojson.Unmarshal([]byte(s), w); err != nil {
			return nil, fmt.Errorf("invalid BoolValue %q: %w", s, err)
		}

		return w, nil
	}
}

// StringValue returns a FlagDeserializer for google.protobuf.StringValue.
// An empty string produces a zero-value StringValue ("").
func StringValue() protocli.FlagDeserializer {
	return func(_ context.Context, flags protocli.FlagContainer) (proto.Message, error) {
		s := flags.String()
		if s == "" {
			return &wrapperspb.StringValue{}, nil
		}

		w := &wrapperspb.StringValue{}
		if err := protojson.Unmarshal([]byte(s), w); err != nil {
			return nil, fmt.Errorf("invalid StringValue %q: %w", s, err)
		}

		return w, nil
	}
}

// Int32Value returns a FlagDeserializer for google.protobuf.Int32Value.
// An empty string produces a zero-value Int32Value (0).
func Int32Value() protocli.FlagDeserializer {
	return func(_ context.Context, flags protocli.FlagContainer) (proto.Message, error) {
		s := flags.String()
		if s == "" {
			return &wrapperspb.Int32Value{}, nil
		}

		w := &wrapperspb.Int32Value{}
		if err := protojson.Unmarshal([]byte(s), w); err != nil {
			return nil, fmt.Errorf("invalid Int32Value %q: %w", s, err)
		}

		return w, nil
	}
}

// Int64Value returns a FlagDeserializer for google.protobuf.Int64Value.
// An empty string produces a zero-value Int64Value (0).
func Int64Value() protocli.FlagDeserializer {
	return func(_ context.Context, flags protocli.FlagContainer) (proto.Message, error) {
		s := flags.String()
		if s == "" {
			return &wrapperspb.Int64Value{}, nil
		}

		w := &wrapperspb.Int64Value{}
		if err := protojson.Unmarshal([]byte(s), w); err != nil {
			return nil, fmt.Errorf("invalid Int64Value %q: %w", s, err)
		}

		return w, nil
	}
}

// UInt32Value returns a FlagDeserializer for google.protobuf.UInt32Value.
// An empty string produces a zero-value UInt32Value (0).
func UInt32Value() protocli.FlagDeserializer {
	return func(_ context.Context, flags protocli.FlagContainer) (proto.Message, error) {
		s := flags.String()
		if s == "" {
			return &wrapperspb.UInt32Value{}, nil
		}

		w := &wrapperspb.UInt32Value{}
		if err := protojson.Unmarshal([]byte(s), w); err != nil {
			return nil, fmt.Errorf("invalid UInt32Value %q: %w", s, err)
		}

		return w, nil
	}
}

// UInt64Value returns a FlagDeserializer for google.protobuf.UInt64Value.
// An empty string produces a zero-value UInt64Value (0).
func UInt64Value() protocli.FlagDeserializer {
	return func(_ context.Context, flags protocli.FlagContainer) (proto.Message, error) {
		s := flags.String()
		if s == "" {
			return &wrapperspb.UInt64Value{}, nil
		}

		w := &wrapperspb.UInt64Value{}
		if err := protojson.Unmarshal([]byte(s), w); err != nil {
			return nil, fmt.Errorf("invalid UInt64Value %q: %w", s, err)
		}

		return w, nil
	}
}

// FloatValue returns a FlagDeserializer for google.protobuf.FloatValue.
// An empty string produces a zero-value FloatValue (0).
func FloatValue() protocli.FlagDeserializer {
	return func(_ context.Context, flags protocli.FlagContainer) (proto.Message, error) {
		s := flags.String()
		if s == "" {
			return &wrapperspb.FloatValue{}, nil
		}

		w := &wrapperspb.FloatValue{}
		if err := protojson.Unmarshal([]byte(s), w); err != nil {
			return nil, fmt.Errorf("invalid FloatValue %q: %w", s, err)
		}

		return w, nil
	}
}

// DoubleValue returns a FlagDeserializer for google.protobuf.DoubleValue.
// An empty string produces a zero-value DoubleValue (0).
func DoubleValue() protocli.FlagDeserializer {
	return func(_ context.Context, flags protocli.FlagContainer) (proto.Message, error) {
		s := flags.String()
		if s == "" {
			return &wrapperspb.DoubleValue{}, nil
		}

		w := &wrapperspb.DoubleValue{}
		if err := protojson.Unmarshal([]byte(s), w); err != nil {
			return nil, fmt.Errorf("invalid DoubleValue %q: %w", s, err)
		}

		return w, nil
	}
}

// All returns ServiceOptions that register deserializers for all well-known types.
// The returned slice can be spread directly into generated ServiceCommand calls:
//
//	svc := proto.FooServiceCommand(ctx, impl, deserializers.All()...)
func All() []protocli.ServiceOption {
	return []protocli.ServiceOption{
		protocli.WithFlagDeserializer("google.protobuf.Timestamp", Timestamp()),
		protocli.WithFlagDeserializer("google.protobuf.Duration", Duration()),
		protocli.WithFlagDeserializer("google.protobuf.FieldMask", FieldMask()),
		protocli.WithFlagDeserializer("google.protobuf.Struct", Struct()),
		protocli.WithFlagDeserializer("google.protobuf.BoolValue", BoolValue()),
		protocli.WithFlagDeserializer("google.protobuf.StringValue", StringValue()),
		protocli.WithFlagDeserializer("google.protobuf.Int32Value", Int32Value()),
		protocli.WithFlagDeserializer("google.protobuf.Int64Value", Int64Value()),
		protocli.WithFlagDeserializer("google.protobuf.UInt32Value", UInt32Value()),
		protocli.WithFlagDeserializer("google.protobuf.UInt64Value", UInt64Value()),
		protocli.WithFlagDeserializer("google.protobuf.FloatValue", FloatValue()),
		protocli.WithFlagDeserializer("google.protobuf.DoubleValue", DoubleValue()),
	}
}
