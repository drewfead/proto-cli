// Package cli provides Protocol Buffer definitions for CLI annotations.
//
// This package contains the proto definitions used to annotate gRPC service
// and method definitions with CLI-specific metadata. These annotations control
// how the proto-cli code generator creates CLI commands, flags, and help text.
//
// The main proto definition is cli.proto, which defines extensions for:
//   - Service-level annotations (service names, descriptions)
//   - RPC method annotations (command names, descriptions, hidden flags)
//   - Field-level annotations (flag names, usage text, shorthand aliases)
//
// Generated code in this package should not be edited manually.
// To regenerate after modifying cli.proto, run: go generate
package cli

//go:generate sh -c "true"
//go:generate go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint fmt .
//go:generate go run mvdan.cc/gofumpt -l -w .
