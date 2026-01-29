# Flat Command Structure Example

This example demonstrates the **flat command structure** for single-service CLIs.

## Flat vs Nested Structure

Proto-CLI supports two command structures:

### Nested (Default) - For Multi-Service CLIs
```bash
usercli user-service get --id 1
usercli admin health-check
usercli daemonize --port 50051
```
Commands are organized under service names.

### Flat (Optional) - For Single-Service CLIs
```bash
usercli-flat get --id 1
usercli-flat create --name "Alice" --email "alice@example.com"
usercli-flat daemonize --port 50051
```
RPC commands are at the root level, resulting in a cleaner CLI for single-service applications.

## Usage

### Using Nested Structure (default)
```go
// Returns *ServiceCLI with nested commands
userServiceCLI := simple.UserServiceCommand(ctx, newUserService, opts...)

// Add to root with other services
rootCmd := protocli.RootCommand("usercli",
    protocli.Service(userServiceCLI),
    protocli.Service(adminServiceCLI),
)
```

### Using Flat Structure (single-service)
```go
// Returns []*cli.Command with RPC commands + daemonize
commands := simple.UserServiceCommandsFlat(ctx, newUserService, opts...)

// Use commands directly at root level
rootCmd := &cli.Command{
    Name:     "usercli-flat",
    Commands: commands,  // All RPC commands + daemonize
}
```

## When to Use Flat Structure

Use the flat structure when:
- You have a **single service** and don't need service-level grouping
- You want a **simpler command structure** (e.g., `get` instead of `user-service get`)
- Your CLI mirrors a single microservice or API

Use the nested structure when:
- You have **multiple services** in one CLI
- You want clear **service boundaries** in the command structure
- You're building an umbrella CLI for multiple backends

## Example Commands

```bash
# Build the flat example
go build -o bin/usercli-flat ./examples/simple/usercli_flat

# View help
bin/usercli-flat --help

# Get a user
bin/usercli-flat get --id 1 --db-url "postgres://localhost/test"

# Create a user with optional fields
bin/usercli-flat create \
  --name "Alice" \
  --email "alice@example.com" \
  --nickname "ace" \
  --age 30 \
  --verified

# Start gRPC server
bin/usercli-flat daemonize --port 50051

# Call remotely
bin/usercli-flat get --id 1 --remote localhost:50051
```

## Generated Functions

For each service, proto-cli generates **two functions**:

1. **`ServiceNameCommand`** - Returns `*ServiceCLI` with nested structure
   ```go
   func UserServiceCommand(ctx context.Context, implOrFactory interface{}, opts ...ServiceOption) *ServiceCLI
   ```

2. **`ServiceNameCommandsFlat`** - Returns `[]*cli.Command` with flat structure
   ```go
   func UserServiceCommandsFlat(ctx context.Context, implOrFactory interface{}, opts ...ServiceOption) []*cli.Command
   ```

Both functions accept the same parameters and options.
