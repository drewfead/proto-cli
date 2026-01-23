# gRPC Proto Generation with Buf + CLI Code Generator

This project uses [Buf](https://buf.build) to generate Go code from Protocol Buffer definitions, plus a **custom CLI code generator** that creates command-line interfaces directly from your gRPC service definitions. All dependencies are managed through Go modules, so no external tools need to be installed.

## Project Structure

```
.
├── proto/
│   ├── example.proto          # Protocol buffer definitions
│   ├── example.pb.go          # Generated protobuf code
│   ├── example_grpc.pb.go     # Generated gRPC service code
│   └── example_cli.pb.go      # Generated CLI code (NEW!)
├── pkg/protocli/
│   ├── cli.proto              # CLI annotation definitions
│   └── README.md              # CLI generator documentation
├── cmd/
│   ├── protoc-gen-cli/        # CLI code generator plugin
│   │   └── main.go
│   └── usercli/               # Example generated CLI binary
│       └── main.go
├── tools/
│   └── tools.go               # Tracks code generation tool dependencies
├── buf.yaml                   # Buf module configuration
├── buf.gen.yaml               # Buf code generation configuration (includes CLI plugin)
├── doc.go                     # Contains go:generate directive
├── main.go                    # Example gRPC server implementation
└── go.mod                     # Go module dependencies
```

## How It Works

1. **tools/tools.go** - Uses the Go tools pattern to track build-time dependencies in `go.mod`:
   - `buf` - Modern protocol buffer compiler
   - `protoc-gen-go` - Protocol buffer Go plugin
   - `protoc-gen-go-grpc` - gRPC Go plugin

2. **doc.go** - Contains the `//go:generate` directive that runs buf

3. **buf.gen.yaml** - Configures buf to use `go run` for plugins, ensuring all tools are resolved through `go.mod`

## Generating Code

Simply run:

```bash
go generate ./...
```

This will:
1. Run buf via `go run` (no installation required)
2. Generate `proto/*.pb.go` and `proto/*_grpc.pb.go` files
3. All dependencies are automatically fetched from `go.mod`

## Adding New Proto Files

1. Create your `.proto` file in the `proto/` directory
2. Add the `go_package` option:
   ```protobuf
   option go_package = "sandbox/proto";
   ```
3. Run `go generate ./...`

## Building and Running

```bash
# Generate proto files (including CLI code)
go generate ./...

# Build the gRPC server
go build -o server .

# Build the CLI client
go build -o usercli ./cmd/usercli

# Run the server (in one terminal)
./server

# Use the CLI client (in another terminal)
./usercli createuser --name "Alice" --email "alice@example.com"
./usercli getuser --id 1
```

## Features

### Standard Proto Generation (Buf)
- **No external dependencies**: Everything runs through Go tooling
- **Faster**: More efficient than protoc
- **Better linting**: Built-in proto linting and breaking change detection
- **Simpler**: Cleaner configuration than protoc
- **Module-aware**: Integrates seamlessly with Go modules

### Auto-Generated CLI (pkg/protocli)
- **Type-Safe Code Gen**: Uses **jennifer** for generating clean, type-safe Go code
- **Modern CLI Framework**: Built on **urfave/cli v3** (simpler and lighter than Cobra)
- **CLI from gRPC**: Automatically generates CLIs from service definitions
- **Custom Annotations**: Use `(cli.command)` and `(cli.flag)` annotations (coming soon)
- **Proper Types**: Automatic type handling (int → int64, string, bool) from proto fields
- **Reusable**: The `pkg/protocli` library can be integrated into any project

See [pkg/protocli/README.md](pkg/protocli/README.md) for detailed CLI generation documentation.

## Clean Up

To remove generated files:

```bash
make clean
```

Or manually:

```bash
rm proto/*.pb.go
```
# proto-cli
# proto-cli
