// Package bundle provides convenience functions that combine contrib deserializers
// and formats into ready-to-use option sets for proto-cli service commands.
//
// Usage:
//
//	svc := proto.FooServiceCommand(ctx, impl, bundle.ServiceOptions()...)
package bundle

import (
	protocli "github.com/drewfead/proto-cli"
	"github.com/drewfead/proto-cli/contrib/deserializers"
)

// Formats returns a SharedOption that registers JSON and YAML output formats.
// Works at both service and root level.
func Formats() protocli.SharedOption {
	return protocli.WithOutputFormats(protocli.JSON(), protocli.YAML())
}

// Deserializers returns ServiceOptions that register all well-known type deserializers.
// Delegates to deserializers.All().
func Deserializers() []protocli.ServiceOption {
	return deserializers.All()
}

// ServiceOptions returns a combined set of ServiceOptions that include both
// Formats() and Deserializers(). This is the recommended way to configure
// a service command with all conventions:
//
//	svc := proto.FooServiceCommand(ctx, impl, bundle.ServiceOptions()...)
func ServiceOptions() []protocli.ServiceOption {
	opts := []protocli.ServiceOption{Formats()}
	opts = append(opts, Deserializers()...)

	return opts
}
