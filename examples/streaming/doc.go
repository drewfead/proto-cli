// Package streaming demonstrates server streaming gRPC support in proto-cli.
//
// This example shows how to:
//   - Define server streaming RPC methods (one request, multiple responses)
//   - Stream responses with configurable output formats (JSON, YAML, Go)
//   - Configure message delimiters for NDJSON and other formats
//   - Use streaming in both local (direct) and remote (gRPC client) modes
//   - Handle real-time data feeds like watch operations
//
// The example includes:
//   - ListItems: Streams a list of items matching filter criteria
//   - WatchItems: Streams item change events in real-time
//
// Streaming features:
//   - Each message is formatted independently using the selected OutputFormat
//   - Messages are separated by configurable delimiters (default: newline)
//   - Works seamlessly with Unix tools via NDJSON (e.g., | jq ., | grep)
//   - Supports both local execution and remote gRPC server calls
//
// To run the example:
//
//	go run ./streamcli streaming-service list-items --category books --format json
//	go run ./streamcli streaming-service watch-items --start-id 1 --format yaml
//
// Generated code in this package should not be edited manually.
// To regenerate after modifying streaming.proto, run: go generate
package streaming

//go:generate sh -c "true"
//go:generate go tool golangci-lint fmt .
//go:generate go tool gofumpt -l -w .
