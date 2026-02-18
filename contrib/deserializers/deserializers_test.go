package deserializers_test

import (
	"context"
	"testing"

	protocli "github.com/drewfead/proto-cli"
	"github.com/drewfead/proto-cli/contrib/deserializers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// mockFlagContainer implements protocli.FlagContainer for testing.
type mockFlagContainer struct {
	stringVal string
	isSet     bool
}

func (m *mockFlagContainer) String() string                     { return m.stringVal }
func (m *mockFlagContainer) Int() int                           { return 0 }
func (m *mockFlagContainer) Int64() int64                       { return 0 }
func (m *mockFlagContainer) Uint() uint                         { return 0 }
func (m *mockFlagContainer) Uint64() uint64                     { return 0 }
func (m *mockFlagContainer) Bool() bool                         { return false }
func (m *mockFlagContainer) Float() float64                     { return 0 }
func (m *mockFlagContainer) StringSlice() []string              { return nil }
func (m *mockFlagContainer) IsSet() bool                        { return m.isSet }
func (m *mockFlagContainer) StringNamed(_ string) string        { return "" }
func (m *mockFlagContainer) IntNamed(_ string) int              { return 0 }
func (m *mockFlagContainer) Int64Named(_ string) int64          { return 0 }
func (m *mockFlagContainer) BoolNamed(_ string) bool            { return false }
func (m *mockFlagContainer) FloatNamed(_ string) float64        { return 0 }
func (m *mockFlagContainer) StringSliceNamed(_ string) []string { return nil }
func (m *mockFlagContainer) IsSetNamed(_ string) bool           { return false }
func (m *mockFlagContainer) FlagName() string                   { return "test-flag" }

var _ protocli.FlagContainer = (*mockFlagContainer)(nil)

func TestUnit_Timestamp(t *testing.T) {
	ctx := context.Background()
	deser := deserializers.Timestamp()

	t.Run("valid RFC3339", func(t *testing.T) {
		flags := &mockFlagContainer{stringVal: "2024-01-15T10:30:00Z", isSet: true}
		msg, err := deser(ctx, flags)
		require.NoError(t, err)

		ts, ok := msg.(*timestamppb.Timestamp)
		require.True(t, ok)
		assert.Equal(t, int64(1705314600), ts.AsTime().Unix())
	})

	t.Run("empty string returns zero value", func(t *testing.T) {
		flags := &mockFlagContainer{stringVal: ""}
		msg, err := deser(ctx, flags)
		require.NoError(t, err)

		ts, ok := msg.(*timestamppb.Timestamp)
		require.True(t, ok)
		assert.Equal(t, int64(0), ts.GetSeconds())
		assert.Equal(t, int32(0), ts.GetNanos())
	})

	t.Run("invalid input returns error", func(t *testing.T) {
		flags := &mockFlagContainer{stringVal: "not-a-timestamp", isSet: true}
		_, err := deser(ctx, flags)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid RFC3339 timestamp")
	})
}

func TestUnit_Duration(t *testing.T) {
	ctx := context.Background()
	deser := deserializers.Duration()

	t.Run("valid duration", func(t *testing.T) {
		flags := &mockFlagContainer{stringVal: "5m30s", isSet: true}
		msg, err := deser(ctx, flags)
		require.NoError(t, err)

		d, ok := msg.(*durationpb.Duration)
		require.True(t, ok)
		assert.Equal(t, int64(330), d.GetSeconds())
	})

	t.Run("empty string returns zero value", func(t *testing.T) {
		flags := &mockFlagContainer{stringVal: ""}
		msg, err := deser(ctx, flags)
		require.NoError(t, err)

		d, ok := msg.(*durationpb.Duration)
		require.True(t, ok)
		assert.Equal(t, int64(0), d.GetSeconds())
		assert.Equal(t, int32(0), d.GetNanos())
	})

	t.Run("invalid input returns error", func(t *testing.T) {
		flags := &mockFlagContainer{stringVal: "not-a-duration", isSet: true}
		_, err := deser(ctx, flags)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid duration")
	})
}

func TestUnit_FieldMask(t *testing.T) {
	ctx := context.Background()
	deser := deserializers.FieldMask()

	t.Run("valid comma-separated paths", func(t *testing.T) {
		flags := &mockFlagContainer{stringVal: "name,email,address.city", isSet: true}
		msg, err := deser(ctx, flags)
		require.NoError(t, err)

		fm, ok := msg.(*fieldmaskpb.FieldMask)
		require.True(t, ok)
		assert.Equal(t, []string{"name", "email", "address.city"}, fm.GetPaths())
	})

	t.Run("trims whitespace around paths", func(t *testing.T) {
		flags := &mockFlagContainer{stringVal: " name , email ", isSet: true}
		msg, err := deser(ctx, flags)
		require.NoError(t, err)

		fm, ok := msg.(*fieldmaskpb.FieldMask)
		require.True(t, ok)
		assert.Equal(t, []string{"name", "email"}, fm.GetPaths())
	})

	t.Run("empty string returns empty field mask", func(t *testing.T) {
		flags := &mockFlagContainer{stringVal: ""}
		msg, err := deser(ctx, flags)
		require.NoError(t, err)

		fm, ok := msg.(*fieldmaskpb.FieldMask)
		require.True(t, ok)
		assert.Empty(t, fm.GetPaths())
	})
}

func TestUnit_Struct(t *testing.T) {
	ctx := context.Background()
	deser := deserializers.Struct()

	t.Run("valid JSON", func(t *testing.T) {
		flags := &mockFlagContainer{stringVal: `{"key":"value","num":42}`, isSet: true}
		msg, err := deser(ctx, flags)
		require.NoError(t, err)

		st, ok := msg.(*structpb.Struct)
		require.True(t, ok)
		assert.Equal(t, "value", st.GetFields()["key"].GetStringValue())
		assert.InDelta(t, 42.0, st.GetFields()["num"].GetNumberValue(), 0.001)
	})

	t.Run("empty string returns empty struct", func(t *testing.T) {
		flags := &mockFlagContainer{stringVal: ""}
		msg, err := deser(ctx, flags)
		require.NoError(t, err)

		st, ok := msg.(*structpb.Struct)
		require.True(t, ok)
		assert.Nil(t, st.GetFields())
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		flags := &mockFlagContainer{stringVal: "not-json", isSet: true}
		_, err := deser(ctx, flags)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid JSON for Struct")
	})
}

// wrapperTestCase defines a table-driven test case for wrapper value deserializers.
type wrapperTestCase struct {
	name         string
	factory      func() protocli.FlagDeserializer
	validInput   string
	invalidInput string
	errContains  string
	checkValid   func(t *testing.T, msg proto.Message)
	checkZero    func(t *testing.T, msg proto.Message)
}

//nolint:gochecknoglobals // test table
var wrapperTests = []wrapperTestCase{
	{
		name:         "BoolValue",
		factory:      deserializers.BoolValue,
		validInput:   "true",
		invalidInput: "notbool",
		errContains:  "invalid BoolValue",
		checkValid: func(t *testing.T, msg proto.Message) {
			t.Helper()
			bv, ok := msg.(*wrapperspb.BoolValue)
			require.True(t, ok)
			assert.True(t, bv.GetValue())
		},
		checkZero: func(t *testing.T, msg proto.Message) {
			t.Helper()
			bv, ok := msg.(*wrapperspb.BoolValue)
			require.True(t, ok)
			assert.False(t, bv.GetValue())
		},
	},
	{
		name:       "StringValue",
		factory:    deserializers.StringValue,
		validInput: `"hello"`,
		checkValid: func(t *testing.T, msg proto.Message) {
			t.Helper()
			sv, ok := msg.(*wrapperspb.StringValue)
			require.True(t, ok)
			assert.Equal(t, "hello", sv.GetValue())
		},
		checkZero: func(t *testing.T, msg proto.Message) {
			t.Helper()
			sv, ok := msg.(*wrapperspb.StringValue)
			require.True(t, ok)
			assert.Empty(t, sv.GetValue())
		},
	},
	{
		name:         "Int32Value",
		factory:      deserializers.Int32Value,
		validInput:   "42",
		invalidInput: "not-a-number",
		errContains:  "invalid Int32Value",
		checkValid: func(t *testing.T, msg proto.Message) {
			t.Helper()
			iv, ok := msg.(*wrapperspb.Int32Value)
			require.True(t, ok)
			assert.Equal(t, int32(42), iv.GetValue())
		},
		checkZero: func(t *testing.T, msg proto.Message) {
			t.Helper()
			iv, ok := msg.(*wrapperspb.Int32Value)
			require.True(t, ok)
			assert.Equal(t, int32(0), iv.GetValue())
		},
	},
	{
		name:       "Int64Value",
		factory:    deserializers.Int64Value,
		validInput: `"100"`,
		checkValid: func(t *testing.T, msg proto.Message) {
			t.Helper()
			iv, ok := msg.(*wrapperspb.Int64Value)
			require.True(t, ok)
			assert.Equal(t, int64(100), iv.GetValue())
		},
		checkZero: func(t *testing.T, msg proto.Message) {
			t.Helper()
			iv, ok := msg.(*wrapperspb.Int64Value)
			require.True(t, ok)
			assert.Equal(t, int64(0), iv.GetValue())
		},
	},
	{
		name:       "UInt32Value",
		factory:    deserializers.UInt32Value,
		validInput: "99",
		checkValid: func(t *testing.T, msg proto.Message) {
			t.Helper()
			uv, ok := msg.(*wrapperspb.UInt32Value)
			require.True(t, ok)
			assert.Equal(t, uint32(99), uv.GetValue())
		},
		checkZero: func(t *testing.T, msg proto.Message) {
			t.Helper()
			uv, ok := msg.(*wrapperspb.UInt32Value)
			require.True(t, ok)
			assert.Equal(t, uint32(0), uv.GetValue())
		},
	},
	{
		name:       "UInt64Value",
		factory:    deserializers.UInt64Value,
		validInput: `"999"`,
		checkValid: func(t *testing.T, msg proto.Message) {
			t.Helper()
			uv, ok := msg.(*wrapperspb.UInt64Value)
			require.True(t, ok)
			assert.Equal(t, uint64(999), uv.GetValue())
		},
		checkZero: func(t *testing.T, msg proto.Message) {
			t.Helper()
			uv, ok := msg.(*wrapperspb.UInt64Value)
			require.True(t, ok)
			assert.Equal(t, uint64(0), uv.GetValue())
		},
	},
	{
		name:         "FloatValue",
		factory:      deserializers.FloatValue,
		validInput:   "3.14",
		invalidInput: "not-a-float",
		errContains:  "invalid FloatValue",
		checkValid: func(t *testing.T, msg proto.Message) {
			t.Helper()
			fv, ok := msg.(*wrapperspb.FloatValue)
			require.True(t, ok)
			assert.InDelta(t, 3.14, float64(fv.GetValue()), 0.001)
		},
		checkZero: func(t *testing.T, msg proto.Message) {
			t.Helper()
			fv, ok := msg.(*wrapperspb.FloatValue)
			require.True(t, ok)
			assert.InDelta(t, 0.0, float64(fv.GetValue()), 0.001)
		},
	},
	{
		name:         "DoubleValue",
		factory:      deserializers.DoubleValue,
		validInput:   "2.718281828",
		invalidInput: "not-a-double",
		errContains:  "invalid DoubleValue",
		checkValid: func(t *testing.T, msg proto.Message) {
			t.Helper()
			dv, ok := msg.(*wrapperspb.DoubleValue)
			require.True(t, ok)
			assert.InDelta(t, 2.718281828, dv.GetValue(), 0.0001)
		},
		checkZero: func(t *testing.T, msg proto.Message) {
			t.Helper()
			dv, ok := msg.(*wrapperspb.DoubleValue)
			require.True(t, ok)
			assert.InDelta(t, 0.0, dv.GetValue(), 0.001)
		},
	},
}

func TestUnit_WrapperDeserializers(t *testing.T) {
	ctx := context.Background()

	for _, tc := range wrapperTests {
		t.Run(tc.name+"/valid_input", func(t *testing.T) {
			deser := tc.factory()
			flags := &mockFlagContainer{stringVal: tc.validInput, isSet: true}
			msg, err := deser(ctx, flags)
			require.NoError(t, err)
			tc.checkValid(t, msg)
		})

		t.Run(tc.name+"/empty_string_returns_zero_value", func(t *testing.T) {
			deser := tc.factory()
			flags := &mockFlagContainer{stringVal: ""}
			msg, err := deser(ctx, flags)
			require.NoError(t, err)
			tc.checkZero(t, msg)
		})

		if tc.invalidInput != "" {
			t.Run(tc.name+"/invalid_input_returns_error", func(t *testing.T) {
				deser := tc.factory()
				flags := &mockFlagContainer{stringVal: tc.invalidInput, isSet: true}
				_, err := deser(ctx, flags)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errContains)
			})
		}
	}
}

func TestUnit_All(t *testing.T) {
	t.Run("returns correct count", func(t *testing.T) {
		opts := deserializers.All()
		assert.Len(t, opts, 12)
	})

	t.Run("all options are non-nil", func(t *testing.T) {
		opts := deserializers.All()
		for i, opt := range opts {
			assert.NotNil(t, opt, "option at index %d should not be nil", i)
		}
	})

	t.Run("options apply without error", func(t *testing.T) {
		opts := deserializers.All()
		cfg := protocli.ApplyServiceOptions(opts...)
		assert.NotNil(t, cfg)

		// Verify some deserializers are registered
		_, ok := cfg.FlagDeserializer("google.protobuf.Timestamp")
		assert.True(t, ok, "Timestamp deserializer should be registered")

		_, ok = cfg.FlagDeserializer("google.protobuf.Duration")
		assert.True(t, ok, "Duration deserializer should be registered")

		_, ok = cfg.FlagDeserializer("google.protobuf.DoubleValue")
		assert.True(t, ok, "DoubleValue deserializer should be registered")
	})
}
