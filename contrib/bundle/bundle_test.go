package bundle_test

import (
	"testing"

	protocli "github.com/drewfead/proto-cli"
	"github.com/drewfead/proto-cli/contrib/bundle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnit_Formats(t *testing.T) {
	t.Run("applies to service options", func(t *testing.T) {
		cfg := protocli.ApplyServiceOptions(bundle.Formats())
		require.NotNil(t, cfg)

		formats := cfg.OutputFormats()
		require.Len(t, formats, 2)
		assert.Equal(t, "json", formats[0].Name())
		assert.Equal(t, "yaml", formats[1].Name())
	})

	t.Run("applies to root options without error", func(t *testing.T) {
		// RootConfig doesn't expose OutputFormats() in its public interface,
		// but the option applies without panicking.
		cfg := protocli.ApplyRootOptions(bundle.Formats())
		require.NotNil(t, cfg)
	})
}

func TestUnit_Deserializers(t *testing.T) {
	t.Run("returns correct count", func(t *testing.T) {
		opts := bundle.Deserializers()
		assert.Len(t, opts, 12)
	})

	t.Run("all options are non-nil", func(t *testing.T) {
		opts := bundle.Deserializers()
		for i, opt := range opts {
			assert.NotNil(t, opt, "option at index %d should not be nil", i)
		}
	})
}

func TestUnit_ServiceOptions(t *testing.T) {
	t.Run("composes formats and deserializers", func(t *testing.T) {
		opts := bundle.ServiceOptions()
		// 1 formats option + 12 deserializer options = 13
		assert.Len(t, opts, 13)
	})

	t.Run("applies without error", func(t *testing.T) {
		opts := bundle.ServiceOptions()
		cfg := protocli.ApplyServiceOptions(opts...)
		require.NotNil(t, cfg)

		// Verify formats are registered
		formats := cfg.OutputFormats()
		require.Len(t, formats, 2)
		assert.Equal(t, "json", formats[0].Name())
		assert.Equal(t, "yaml", formats[1].Name())

		// Verify deserializers are registered
		_, ok := cfg.FlagDeserializer("google.protobuf.Timestamp")
		assert.True(t, ok, "Timestamp deserializer should be registered")

		_, ok = cfg.FlagDeserializer("google.protobuf.Duration")
		assert.True(t, ok, "Duration deserializer should be registered")
	})
}
