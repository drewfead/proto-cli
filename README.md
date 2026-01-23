# proto-cli

**Automatically generate type-safe, feature-rich CLI applications from your gRPC/Protocol Buffer service definitions.**

proto-cli is a protoc plugin that generates complete command-line interfaces from Protocol Buffer service definitions. It creates CLIs with proper flag handling, multiple output formats, lifecycle hooks, gRPC interceptors, and both local and remote execution modes.

## Features

- 🎯 **Type-Safe Code Generation** - Uses [jennifer](https://github.com/dave/jennifer) to generate clean, idiomatic Go code
- 🚀 **Zero Boilerplate** - Define your service once in `.proto`, get a full CLI automatically
- 🔌 **Dual Mode Execution** - Call services directly (in-process) or via gRPC (remote)
- 🎛️ **Selective Service Enable** - Start daemon with only specific services using `--service` flag
- 🎨 **Multiple Output Formats** - JSON, YAML, and Go-native formatting built-in
- 🪝 **Lifecycle Hooks** - Before/after command hooks for logging, auth, metrics
- 🛡️ **gRPC Interceptors** - Add unary and stream interceptors for cross-cutting concerns
- 🌐 **gRPC-Gateway Ready** - Built-in support for HTTP/JSON transcoding (coming soon)
- 🏗️ **Multi-Service Support** - One CLI with multiple gRPC services as subcommands
- 📦 **Extensible Options API** - Type-safe functional options pattern
- 🎭 **Modern CLI Framework** - Built on [urfave/cli v3](https://github.com/urfave/cli)

## Quick Start

### Installation

```bash
go install github.com/drewfead/proto-cli/cmd/gen@latest
```

Or use it directly via `go run` in your `buf.gen.yaml` (recommended).

### Basic Usage

1. **Define your service** in a `.proto` file:

```protobuf
syntax = "proto3";

package example;

option go_package = "github.com/yourorg/yourproject/api";

service UserService {
  rpc GetUser(GetUserRequest) returns (UserResponse);
  rpc CreateUser(CreateUserRequest) returns (UserResponse);
}

message GetUserRequest {
  int64 id = 1;
}

message CreateUserRequest {
  string name = 1;
  string email = 2;
}

message User {
  int64 id = 1;
  string name = 2;
  string email = 3;
}

message UserResponse {
  User user = 1;
  string message = 2;
}
```

2. **Configure buf generation** in `buf.gen.yaml`:

```yaml
version: v2
plugins:
  - local: ["go", "run", "google.golang.org/protobuf/cmd/protoc-gen-go"]
    out: .
    opt:
      - paths=source_relative
  - local: ["go", "run", "google.golang.org/grpc/cmd/protoc-gen-go-grpc"]
    out: .
    opt:
      - paths=source_relative
  - local: ["go", "run", "github.com/drewfead/proto-cli/cmd/gen"]
    out: .
    opt:
      - paths=source_relative
```

3. **Generate code**:

```bash
buf generate
```

4. **Build your CLI**:

```go
package main

import (
    "context"
    "os"

    protocli "github.com/drewfead/proto-cli"
    "yourproject/api"
)

type userService struct {
    api.UnimplementedUserServiceServer
}

func (s *userService) GetUser(ctx context.Context, req *api.GetUserRequest) (*api.UserResponse, error) {
    // Your implementation
}

func main() {
    ctx := context.Background()
    impl := &userService{}

    // Generate CLI from service
    serviceCLI := api.UserServiceServiceCommand(ctx, impl,
        protocli.WithOutputFormats(
            protocli.JSON(),
            protocli.YAML(),
        ),
    )

    // Create root command
    rootCmd := protocli.RootCommand("usercli",
        protocli.WithService(serviceCLI),
    )

    rootCmd.Run(ctx, os.Args)
}
```

5. **Use your CLI**:

```bash
# Direct call (in-process)
./usercli userservice getuser --id 1 --format json

# Start as gRPC server (all services)
./usercli daemonize --port 50051

# Start with specific services only
./usercli daemonize --port 50051 --service userservice --service productservice

# Remote call (gRPC client)
./usercli userservice getuser --id 1 --remote localhost:50051 --format yaml
```

## Examples

See the [examples](examples) directory for complete working examples including:

Run the example:

```bash
# Build the example
make build/example

# Direct call
./bin/usercli userservice getuser --id 1 --format json

# Start daemon (all services)
./bin/usercli daemonize --port 50051

# Start daemon with specific services only
./bin/usercli daemonize --port 50051 --service userservice

# Remote call
./bin/usercli userservice getuser --id 1 --remote localhost:50051 --format yaml
```

## Development Setup

### Tools Directory

This project uses a `tools` directory to track build-time dependencies in `go.mod` without including them in the final binary. This ensures all developers use the same versions of build tools like `buf`, `golangci-lint`, and code generators.

To set up the tools directory in your own project:

1. **Create the tools directory**:

```bash
mkdir -p tools
```

2. **Create `tools/tools.go`** with build constraints:

```go
//go:build tools
// +build tools

// Package tools tracks dependencies for build tools
package tools

import (
    _ "github.com/bufbuild/buf/cmd/buf"
    _ "github.com/drewfead/proto-cli/cmd/gen"
    _ "github.com/golangci/golangci-lint/cmd/golangci-lint"
    _ "google.golang.org/grpc/cmd/protoc-gen-go-grpc"
    _ "google.golang.org/protobuf/cmd/protoc-gen-go"
)
```

3. **Add tools to go.mod**:

```bash
go mod tidy
```

The `//go:build tools` constraint ensures this package is only compiled when explicitly requested, preventing these dependencies from being included in your production binaries. You can then use `go run` to execute these tools:

```bash
go run github.com/bufbuild/buf/cmd/buf generate
go run github.com/golangci/golangci-lint/cmd/golangci-lint run
```

This pattern guarantees consistent tool versions across your team and CI/CD pipeline.

## License

[MIT License](LICENSE)

## Acknowledgments

- Built on [urfave/cli v3](https://github.com/urfave/cli)
- Code generation via [jennifer](https://github.com/dave/jennifer)
- Proto generation via [Buf](https://buf.build)
