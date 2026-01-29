# proto-cli

[![PR Validation](https://github.com/drewfead/proto-cli/actions/workflows/pr-validation.yml/badge.svg)](https://github.com/drewfead/proto-cli/actions/workflows/pr-validation.yml)
[![BSR](https://img.shields.io/badge/BSR-buf.build%2Ffernet%2Fproto--cli-blue)](https://buf.build/fernet/proto-cli)

**Automatically generate production-ready CLI applications from your gRPC service definitions.**

proto-cli is a protoc plugin that transforms Protocol Buffer service definitions into feature-rich command-line interfaces. Define your API once, get a complete CLI with proper flag handling, configuration management, lifecycle hooks, and both local and remote execution.

> **📦 Available on Buf Schema Registry:** Import CLI annotations directly in your proto files:
> ```yaml
> deps:
>   - buf.build/fernet/proto-cli
> ```

## Features

### Core Capabilities
- **Zero Boilerplate** - Define services in `.proto`, generate complete CLIs automatically
- **Type-Safe Generation** - Clean, idiomatic Go code via [jennifer](https://github.com/dave/jennifer)
- **Dual Execution Modes** - Run in-process (direct calls) or remote (gRPC client)
- **Streaming Support** - Server-side streaming RPCs with line-delimited output (NDJSON, YAML)
- **Multi-Service CLIs** - Organize multiple services under one CLI with nested commands

### Configuration & Customization
- **Configuration Loading** - YAML/JSON config files with environment variable overrides
- **Optional Fields** - Full proto3 optional field support with explicit presence tracking
- **Custom Deserializers** - Transform CLI flags into complex proto messages
- **Lifecycle Hooks** - Before/after command execution, daemon startup/ready/shutdown
- **gRPC Interceptors** - Add unary and stream interceptors for logging, auth, metrics

### Output & Display
- **Multiple Formats** - JSON, YAML, and Go-native formatting built-in
- **Template Formats** - Create custom formats using Go text templates
- **Format-Specific Flags** - Custom flags per format (e.g., `--pretty` for JSON)
- **Streaming Output** - NDJSON for JSON, document-delimited for YAML

### Service Management
- **Flat Command Structure** - Hoist service commands to root level for single-service CLIs
- **Selective Service Enable** - Start daemon with specific services: `--service userservice`
- **Collision Detection** - Clear errors when command names conflict in hoisted services

### Developer Experience
- **CLI Annotations** - Customize command names, flags, descriptions via proto options
- **Type-Safe Options API** - Functional options pattern for configuration
- **Built on [urfave/cli v3](https://github.com/urfave/cli)** - Modern, well-tested CLI framework

## Quick Start

### Installation

Add to `buf.gen.yaml`:

```yaml
version: v2
plugins:
  - local: ["go", "run", "google.golang.org/protobuf/cmd/protoc-gen-go"]
    out: .
    opt: [paths=source_relative]
  - local: ["go", "run", "google.golang.org/grpc/cmd/protoc-gen-go-grpc"]
    out: .
    opt: [paths=source_relative]
  - local: ["go", "run", "github.com/drewfead/proto-cli/cmd/gen"]
    out: .
    opt: [paths=source_relative]
```

### Basic Example

**1. Define your service** ([example.proto](examples/simple/example.proto)):

```protobuf
import "internal/clipb/cli.proto";

service UserService {
  option (cli.service) = {
    name: "user-service"
    description: "Manage users"
  };

  rpc GetUser(GetUserRequest) returns (UserResponse) {
    option (cli.command) = {
      name: "get"
      description: "Retrieve a user by ID"
    };
  }
}

message GetUserRequest {
  int64 id = 1 [(cli.flag) = {name: "id", usage: "User ID"}];
  optional string fields = 2 [(cli.flag) = {name: "fields", usage: "Fields to return"}];
}
```

**2. Generate code**:

```bash
buf generate
```

**3. Build your CLI** ([main.go](examples/simple/usercli/main.go)):

```go
ctx := context.Background()

// Create service implementation
userServiceCLI := simple.UserServiceCommand(ctx, &userService{},
    protocli.WithOutputFormats(protocli.JSON(), protocli.YAML()),
)

// Create root command
rootCmd, err := protocli.RootCommand("usercli",
    protocli.Service(userServiceCLI),
    protocli.WithEnvPrefix("USERCLI"),
)

rootCmd.Run(ctx, os.Args)
```

**4. Use your CLI**:

```bash
# Direct call (in-process)
./usercli user-service get --id 1

# Start gRPC server
./usercli daemonize --port 50051

# Remote call
./usercli user-service get --id 1 --remote localhost:50051
```

## Examples

### [Simple Example](examples/simple/)
Basic CRUD operations with configuration loading, custom deserializers, and multi-service support.

**Highlights:**
- Configuration from YAML files and environment variables
- Optional proto3 fields with explicit presence
- Custom timestamp deserializers
- Nested configuration messages
- Multi-service CLI (UserService + AdminService)

**Try it:**
```bash
make build/example
./bin/usercli user-service get --id 1 --format json
./bin/usercli daemonize --port 50051
```

### [Streaming Example](examples/streaming/)
Server-side streaming RPCs with line-delimited output formats.

**Highlights:**
- Server streaming RPC support
- NDJSON output for JSON format
- Offset and filtering with optional fields
- Works with Unix tools (`jq`, `grep`, `wc`)

**Try it:**
```bash
go build -o bin/streamcli ./examples/streaming/streamcli
./bin/streamcli streaming-service list-items --category books --format json | jq .
```

### [Flat Command Structure](examples/simple/usercli_flat/)
Single-service CLIs with commands at the root level using `protocli.Hoisted()`.

**Comparison:**
- Nested: `./usercli user-service get --id 1`
- Flat: `./usercli-flat get --id 1`

**Usage:**
```go
rootCmd, err := protocli.RootCommand("usercli-flat",
    protocli.Service(userServiceCLI, protocli.Hoisted()),
)
```

See [usercli_flat/README.md](examples/simple/usercli_flat/README.md) for details.

## Key Features

### Configuration Loading

Define configuration in your proto file using the `service_config` annotation:

```protobuf
// Define your configuration message
message UserServiceConfig {
  string database_url = 1 [(cli.flag) = {
    name: "db-url"
    usage: "PostgreSQL connection URL"
  }];
  int64 max_connections = 2 [(cli.flag) = {
    name: "max-conns"
    usage: "Maximum database connections"
  }];
}

// Attach config to your service
service UserService {
  option (cli.service_config) = {
    config_message: "UserServiceConfig"
  };

  rpc GetUser(GetUserRequest) returns (UserResponse);
}
```

Implement a factory function that receives the config:

```go
func newUserService(config *simple.UserServiceConfig) simple.UserServiceServer {
    log.Printf("DB URL: %s, Max Conns: %d", config.DatabaseUrl, config.MaxConnections)
    return &userService{dbURL: config.DatabaseUrl}
}

// Register the factory
rootCmd := protocli.RootCommand("usercli",
    protocli.Service(userServiceCLI),
    protocli.WithConfigFactory("userservice", newUserService),
)
```

Load configuration from YAML files:

```yaml
# usercli.yaml
services:
  userservice:
    database-url: postgresql://localhost:5432/users
    max-connections: 25
```

Override with environment variables:

```bash
USERCLI_SERVICES_USERSERVICE_DATABASE_URL=postgresql://prod/db ./usercli daemonize
```

**Debugging Configuration Issues**

Enable debug logging to see which config files are loaded and how values are merged:

```bash
# Use debug verbosity to see config loading details
./usercli daemonize --verbosity=debug

# Output shows:
# - Which config files were found/missing
# - Environment variables applied
# - Final merged configuration
```

Programmatically inspect configuration loading:

```go
loader := protocli.NewConfigLoader(
    protocli.DaemonMode,
    protocli.FileConfig("./config.yaml"),
    protocli.EnvPrefix("MYAPP"),
    protocli.DebugMode(true),  // Enable debug tracking
)

config := &myapp.ServiceConfig{}
err := loader.LoadServiceConfig(nil, "myservice", config)

// Get detailed debug information
debug := loader.DebugInfo()
fmt.Printf("Paths checked: %v\n", debug.PathsChecked)
fmt.Printf("Files loaded: %v\n", debug.FilesLoaded)
fmt.Printf("Files failed: %v\n", debug.FilesFailed)
fmt.Printf("Env vars applied: %v\n", debug.EnvVarsApplied)
fmt.Printf("Final config: %+v\n", debug.FinalConfig)
```

**Common Issues:**

1. **Config file not found**: Check `debug.PathsChecked` to see where the CLI looked for config files
2. **Values not applied**: Check `debug.EnvVarsApplied` to verify environment variable names (they must match the prefix + field path)
3. **Wrong precedence**: Remember: CLI flags > environment variables > config files
4. **Field naming**: Proto fields use kebab-case in YAML (e.g., `database_url` → `database-url`)

See [config_debug_test.go](config_debug_test.go) and [nested_config_test.go](nested_config_test.go) for more examples.

### Custom Flag Deserializers

Transform CLI flags into complex proto messages:

```go
protocli.WithFlagDeserializer("google.protobuf.Timestamp",
    func(ctx context.Context, flags protocli.FlagContainer) (proto.Message, error) {
        t, err := time.Parse(time.RFC3339, flags.String())
        if err != nil {
            return nil, err
        }
        return timestamppb.New(t), nil
    },
)
```

See [timestamp_deserializer_test.go](examples/simple/timestamp_deserializer_test.go).

### Template-Based Output Formats

Create custom output formats using Go text templates without writing format code:

```go
// Define templates for each message type
templates := map[string]string{
    "example.UserResponse": `User: {{.user.name}} (ID: {{.user.id}})
Email: {{.user.email}}
{{if .user.address}}Address: {{.user.address.city}}, {{.user.address.state}}{{end}}`,

    "example.CreateUserRequest": `Creating user: {{.name}} <{{.email}}>`,
}

// Create the format
userFormat, err := protocli.TemplateFormat("user-detail", templates)

// Use in CLI
userServiceCLI := simple.UserServiceCommand(ctx, newUserService,
    protocli.WithOutputFormats(userFormat, protocli.JSON()),
)
```

**With Custom Template Functions:**

```go
funcMap := template.FuncMap{
    "upper": strings.ToUpper,
    "date": func(ts string) string {
        // Custom date formatting
        return formattedDate
    },
}

format, err := protocli.TemplateFormat("custom", templates, funcMap)
```

**Template Features:**
- Access all message fields as `{{.fieldName}}`
- Conditionals: `{{if .field}}...{{end}}`
- Loops: `{{range .list}}...{{end}}`
- Custom functions via `template.FuncMap`
- Nested field access: `{{.user.address.city}}`
- Format any message type by fully qualified name

**Common Use Cases:**
- Table formats: Create ASCII tables with `printf` for alignment
- Compact formats: One-line summaries like `{{.name}} <{{.email}}>`
- CSV/TSV: `{{.id}},{{.name}},{{.email}}`
- Custom business formats: Match your organization's output standards

See [template_format_test.go](template_format_test.go) for comprehensive examples.

### Lifecycle Hooks

Add hooks for logging, authentication, metrics:

```go
protocli.RootCommand("usercli",
    protocli.Service(userServiceCLI),
    protocli.BeforeCommand(func(ctx context.Context, cmd *cli.Command) error {
        log.Printf("Executing: %s", cmd.Name)
        return nil
    }),
    protocli.OnDaemonStartup(func(ctx context.Context, server *grpc.Server, mux *runtime.ServeMux) error {
        // Register additional services, configure server
        return nil
    }),
    protocli.OnDaemonReady(func(ctx context.Context) {
        log.Println("Server is ready")
    }),
    protocli.OnDaemonShutdown(func(ctx context.Context) {
        log.Println("Shutting down gracefully")
    }),
)
```

See [daemon_lifecycle_test.go](daemon_lifecycle_test.go) for complete examples.

### Help Text Customization

Proto-CLI follows [urfave/cli v3 best practices](https://cli.urfave.org/v3/examples/help/generated-help-text/) for help text. Customize help at multiple levels:

**Proto Annotations:**

Use proto annotations to define help text fields following urfave/cli v3 conventions:

```protobuf
service UserService {
  option (cli.service) = {
    name: "user-service",
    description: "User management commands",  // Short one-liner
    long_description: "Detailed explanation of the service...",  // Multi-paragraph
    usage_text: "user-service [command] [options]",  // Custom USAGE line
    args_usage: "[filter-expression]"  // Argument description
  };

  rpc GetUser(GetUserRequest) returns (UserResponse) {
    option (cli.command) = {
      name: "get",
      description: "Retrieve a user by ID",  // Short (shown in lists)
      long_description: "Fetch detailed user information...\n\nExamples:\n  usercli get --id 123",
      usage_text: "get --id <user-id> [options]",  // Override auto-generated USAGE
      args_usage: "<user-id>"  // Describe positional args
    };
  }
}
```

**Help Field Guidelines** (urfave/cli v3):
- **description**: Short one-liner for command lists (e.g., "retrieve a user by ID")
- **long_description**: Detailed explanation with examples and context
- **usage_text**: Override auto-generated USAGE line format
- **args_usage**: Describe expected arguments

**Programmatic Customization:**

```go
// Method 1: Custom root command template
rootCmd, _ := protocli.RootCommand("myapp",
    protocli.Service(userServiceCLI),
    protocli.WithRootCommandHelpTemplate(`
NAME:
   {{.Name}} - {{.Usage}}

USAGE:
   {{.HelpName}} {{if .VisibleFlags}}[options]{{end}} command

VERSION:
   {{.Version}}

WEBSITE:
   https://example.com
`),
)

// Method 2: Full control with HelpCustomization
protocli.WithHelpCustomization(&protocli.HelpCustomization{
    RootCommandHelpTemplate: myTemplate,
    CommandHelpTemplate: myCommandTemplate,
    SubcommandHelpTemplate: mySubcommandTemplate,
})

// Method 3: Modify the returned root command
rootCmd, _ := protocli.RootCommand("myapp", protocli.Service(userServiceCLI))
rootCmd.Version = "1.0.0"
rootCmd.Copyright = "(c) 2026 MyCompany"
rootCmd.Authors = []any{"John Doe <john@example.com>"}
```

**Best Practices:**
- Keep `description` concise (one line) for readability in command lists
- Use `long_description` for detailed explanations, examples, and context
- Follow the [urfave/cli v3 help conventions](https://cli.urfave.org/v3/)
- Include examples in `long_description` to aid discovery

### Streaming RPCs

Server streaming RPCs output line-delimited messages:

```bash
# JSON format produces NDJSON (one JSON object per line)
./streamcli streaming-service list-items --format json
{"item":{"id":"1","name":"Item 1"}}
{"item":{"id":"2","name":"Item 2"}}

# Works with jq and other Unix tools
./streamcli streaming-service list-items --format json | jq 'select(.item.id > "1")'
```

See [streaming example](examples/streaming/) for details.

### Optional Fields

Full support for proto3 optional fields with explicit presence:

```protobuf
message CreateUserRequest {
  string name = 1;  // Required
  optional string nickname = 2;  // Optional with presence tracking
  optional int32 age = 3;  // Only set if flag provided
}
```

Only sets the field if the flag is provided:
```bash
./usercli user-service create --name "Alice"  # nickname and age unset
./usercli user-service create --name "Alice" --nickname "ace"  # nickname set
```

### gRPC Interceptors

Add interceptors for cross-cutting concerns:

```go
protocli.WithUnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
    start := time.Now()
    resp, err := handler(ctx, req)
    log.Printf("%s took %v", info.FullMethod, time.Since(start))
    return resp, err
}),
protocli.WithStreamInterceptor(streamLoggingInterceptor),
```

### Selective Service Enable

Start daemon with only specific services:

```bash
# Start all services
./usercli daemonize --port 50051

# Start only userservice and productservice
./usercli daemonize --port 50051 --service userservice --service productservice
```

## CLI Annotations

Customize generated CLIs using proto options from [`internal/clipb/cli.proto`](internal/clipb/cli.proto):

```protobuf
service UserService {
  option (cli.service) = {
    name: "users"  // Command name (default: snake_case service name)
    description: "User management commands"
  };

  rpc GetUser(GetUserRequest) returns (UserResponse) {
    option (cli.command) = {
      name: "get"  // Subcommand name
      description: "Retrieve user by ID"
    };
  }
}

message GetUserRequest {
  int64 id = 1 [(cli.flag) = {
    name: "id"
    shorthand: "i"
    usage: "User ID to retrieve"
  }];
}
```

## Development

### Prerequisites
- Go 1.23+
- [Buf](https://buf.build)

### Building

```bash
# Generate code
make generate

# Run tests
make test

# Run linter
make lint

# Build examples
make build/example
```

### Project Structure

```
proto-cli/
├── cmd/gen/          # Code generator (protoc plugin)
├── examples/
│   ├── simple/       # Basic CRUD example
│   │   ├── usercli/      # Multi-service CLI
│   │   └── usercli_flat/ # Flat command structure
│   └── streaming/    # Server streaming example
├── internal/clipb/   # CLI annotation proto definitions
├── root.go           # Root command implementation
├── options.go        # Configuration options
├── config.go         # Configuration loading
└── formats.go        # Output formatters
```

## Contributing

Contributions welcome! Please:
1. Add tests for new features
2. Run `make lint` before submitting
3. Update documentation

## License

[MIT License](LICENSE)

## Acknowledgments

- Built on [urfave/cli v3](https://github.com/urfave/cli)
- Code generation via [jennifer](https://github.com/dave/jennifer)
- Proto tooling via [Buf](https://buf.build)
