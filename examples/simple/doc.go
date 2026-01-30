// Package simple demonstrates basic proto-cli usage with unary gRPC methods.
//
// This example shows how to:
//   - Define gRPC services with CLI annotations
//   - Configure service and command names via (cli.service) and (cli.command)
//   - Customize flag names, usage text, and shorthand aliases
//   - Use nested configuration messages with factory functions
//   - Support both direct invocation and remote gRPC calls
//   - Handle custom deserializers for complex types (timestamps, nested messages)
//   - Load configuration from files and environment variables
//   - Use optional proto3 fields with explicit presence tracking
//
// The example includes two services:
//   - UserService: Demonstrates CRUD operations with custom config loading
//   - AdminService: Shows service name overrides and simple operations
//
// Command Structures:
//   - Nested (default): go run ./usercli user-service get --id 1
//   - Flat (single-service): go run ./usercli_flat get --id 1
//
// To run the example:
//
//	go run ./usercli user-service get --id 1
//	go run ./usercli admin health-check
//	go run ./usercli_flat get --id 1  # flat structure
//
// Generated code in this package should not be edited manually.
// To regenerate after modifying example.proto, run: go generate
package simple

//go:generate sh -c "true"
//go:generate go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint fmt .
//go:generate go run mvdan.cc/gofumpt -l -w .
