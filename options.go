package protocli

import (
	"context"
	"io"
	"log/slog"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/urfave/cli/v3"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// OutputFormat defines how to format proto messages for output.
type OutputFormat interface {
	// Name returns the format identifier (e.g., "json", "text")
	Name() string

	// Format writes the formatted proto message to the writer
	Format(ctx context.Context, cmd *cli.Command, w io.Writer, msg proto.Message) error
}

// FlagContainer provides type-safe access to flag values for a specific flag
// It encapsulates the CLI command and flag name, exposing convenient accessors
// This abstraction allows deserializers to be reusable across different flag names
//
// For deserializers that need to access multiple flags (e.g., top-level request deserializers),
// use the *Named() methods to read other flags by name.
type FlagContainer interface {
	// Primary flag accessors (use the encapsulated flag name)
	String() string
	Int() int
	Int64() int64
	Uint() uint
	Uint64() uint64
	Bool() bool
	Float() float64
	StringSlice() []string
	IsSet() bool

	// Named flag accessors (for accessing other flags)
	StringNamed(flagName string) string
	IntNamed(flagName string) int
	Int64Named(flagName string) int64
	BoolNamed(flagName string) bool
	FloatNamed(flagName string) float64
	StringSliceNamed(flagName string) []string
	IsSetNamed(flagName string) bool

	// FlagName returns the primary flag name for this container
	FlagName() string
}

// flagContainer implements FlagContainer by wrapping a cli.Command and flag name.
type flagContainer struct {
	cmd      *cli.Command
	flagName string
}

// Primary flag accessors (use encapsulated flag name).
func (f *flagContainer) String() string        { return f.cmd.String(f.flagName) }
func (f *flagContainer) Int() int              { return f.cmd.Int(f.flagName) }
func (f *flagContainer) Int64() int64          { return int64(f.cmd.Int(f.flagName)) }
func (f *flagContainer) Uint() uint            { return f.cmd.Uint(f.flagName) }
func (f *flagContainer) Uint64() uint64        { return uint64(f.cmd.Uint(f.flagName)) }
func (f *flagContainer) Bool() bool            { return f.cmd.Bool(f.flagName) }
func (f *flagContainer) Float() float64        { return f.cmd.Float(f.flagName) }
func (f *flagContainer) StringSlice() []string { return f.cmd.StringSlice(f.flagName) }
func (f *flagContainer) IsSet() bool           { return f.cmd.IsSet(f.flagName) }

// Named flag accessors (for accessing other flags by name).
func (f *flagContainer) StringNamed(name string) string        { return f.cmd.String(name) }
func (f *flagContainer) IntNamed(name string) int              { return f.cmd.Int(name) }
func (f *flagContainer) Int64Named(name string) int64          { return int64(f.cmd.Int(name)) }
func (f *flagContainer) BoolNamed(name string) bool            { return f.cmd.Bool(name) }
func (f *flagContainer) FloatNamed(name string) float64        { return f.cmd.Float(name) }
func (f *flagContainer) StringSliceNamed(name string) []string { return f.cmd.StringSlice(name) }
func (f *flagContainer) IsSetNamed(name string) bool           { return f.cmd.IsSet(name) }

// FlagName returns the encapsulated flag name.
func (f *flagContainer) FlagName() string { return f.flagName }

// NewFlagContainer creates a new FlagContainer for the given command and flag name.
func NewFlagContainer(cmd *cli.Command, flagName string) FlagContainer {
	return &flagContainer{cmd: cmd, flagName: flagName}
}

// FlagDeserializer builds a proto message from CLI flags
// This allows users to implement custom logic for constructing complex messages
// from simple CLI flags. The FlagContainer provides type-safe access to the flag value
// without requiring knowledge of the flag name, making deserializers reusable.
//
// Example of a reusable timestamp deserializer:
//
//	func(ctx context.Context, flags FlagContainer) (proto.Message, error) {
//	    timeStr := flags.String()  // No need to know the flag name!
//	    t, err := time.Parse(time.RFC3339, timeStr)
//	    if err != nil {
//	        return nil, err
//	    }
//	    return timestamppb.New(t), nil
//	}
type FlagDeserializer func(ctx context.Context, flags FlagContainer) (proto.Message, error)

// DaemonStartupHook is called before the gRPC server starts listening
// Receives the gRPC server instance and gateway mux (if transcoding is enabled)
// Returning an error prevents the daemon from starting.
type DaemonStartupHook func(ctx context.Context, server *grpc.Server, mux *runtime.ServeMux) error

// DaemonReadyHook is called after the gRPC server is listening and ready to accept connections
// Errors must be handled within the hook (no error return).
type DaemonReadyHook func(ctx context.Context)

// DaemonShutdownHook is called during graceful shutdown after stop accepting new connections
// The context will be cancelled when the graceful shutdown timeout expires
// Errors must be handled within the hook (no error return).
type DaemonShutdownHook func(ctx context.Context)

// FlagConfiguredOutputFormat is an optional interface for formats that need custom flags.
type FlagConfiguredOutputFormat interface {
	OutputFormat

	// Flags returns additional flags this format needs (e.g., --pretty for JSON).
	Flags() []cli.Flag
}

// Public interfaces - minimal API surface

// ServiceConfig is the configuration returned by ApplyServiceOptions.
// Used by generated service command code.
type ServiceConfig interface {
	BeforeCommand() func(context.Context, *cli.Command) error
	AfterCommand() func(context.Context, *cli.Command) error
	OutputFormats() []OutputFormat
	FlagDeserializer(messageName string) (FlagDeserializer, bool)
}

// RootConfig is the configuration returned by ApplyRootOptions.
// Used by RootCommand.
type RootConfig interface {
	Services() []*ServiceCLI
	GRPCServerOptions() []grpc.ServerOption
	EnableTranscoding() bool
	TranscodingPort() int
	ConfigPaths() []string
	EnvPrefix() string
	ServiceFactory(serviceName string) (any, bool)
	GracefulShutdownTimeout() time.Duration
	DaemonStartupHooks() []DaemonStartupHook
	DaemonReadyHooks() []DaemonReadyHook
	DaemonShutdownHooks() []DaemonShutdownHook
	SlogConfig() SlogConfigFunc
}

// Private interfaces for internal use

// baseOptions defines common options interface for both service and root levels.
type baseOptions interface {
	SetBeforeCommand(func(context.Context, *cli.Command) error)
	SetAfterCommand(func(context.Context, *cli.Command) error)
	SetOutputFormats([]OutputFormat)
	BeforeCommand() func(context.Context, *cli.Command) error
	AfterCommand() func(context.Context, *cli.Command) error
	OutputFormats() []OutputFormat
}

// rootOptions extends baseOptions with root-specific methods

// Private implementation structs

// serviceCommandOptions holds configuration for individual service commands.
type serviceCommandOptions struct {
	beforeCommand     func(context.Context, *cli.Command) error
	afterCommand      func(context.Context, *cli.Command) error
	outputFormats     []OutputFormat
	flagDeserializers map[string]FlagDeserializer // messageName -> deserializer
}

// SetBeforeCommand sets the before hook.
func (o *serviceCommandOptions) SetBeforeCommand(fn func(context.Context, *cli.Command) error) {
	o.beforeCommand = fn
}

// SetAfterCommand sets the after hook.
func (o *serviceCommandOptions) SetAfterCommand(fn func(context.Context, *cli.Command) error) {
	o.afterCommand = fn
}

// SetOutputFormats sets the output formats.
func (o *serviceCommandOptions) SetOutputFormats(formats []OutputFormat) {
	o.outputFormats = formats
}

// BeforeCommand returns the before hook, or nil if not set.
func (o *serviceCommandOptions) BeforeCommand() func(context.Context, *cli.Command) error {
	return o.beforeCommand
}

// AfterCommand returns the after hook, or nil if not set.
func (o *serviceCommandOptions) AfterCommand() func(context.Context, *cli.Command) error {
	return o.afterCommand
}

// OutputFormats returns the registered output formats.
func (o *serviceCommandOptions) OutputFormats() []OutputFormat {
	return o.outputFormats
}

// FlagDeserializer returns the deserializer for a message type, if registered.
func (o *serviceCommandOptions) FlagDeserializer(messageName string) (FlagDeserializer, bool) {
	if o.flagDeserializers == nil {
		return nil, false
	}
	deserializer, ok := o.flagDeserializers[messageName]
	return deserializer, ok
}

// SlogConfigurationContext provides context information for slog configuration.
type SlogConfigurationContext interface {
	// IsDaemon returns true if the logger is being configured for daemon mode.
	IsDaemon() bool
	// Level returns the configured log level from the --verbosity flag.
	Level() slog.Level
}

// SlogConfigFunc is a function that configures the slog logger.
// It receives a context with configuration details and returns a configured logger.
type SlogConfigFunc func(ctx context.Context, config SlogConfigurationContext) *slog.Logger

// slogConfigContext implements SlogConfigurationContext.
type slogConfigContext struct {
	isDaemon bool
	level    slog.Level
}

func (c *slogConfigContext) IsDaemon() bool {
	return c.isDaemon
}

func (c *slogConfigContext) Level() slog.Level {
	return c.level
}

// rootCommandOptions holds configuration for the root CLI command.
type rootCommandOptions struct {
	services                []*ServiceCLI
	beforeCommand           func(context.Context, *cli.Command) error
	afterCommand            func(context.Context, *cli.Command) error
	outputFormats           []OutputFormat
	grpcServerOptions       []grpc.ServerOption
	enableTranscoding       bool
	transcodingPort         int
	configPaths             []string             // Config file paths for loading
	envPrefix               string               // Environment variable prefix
	serviceFactories        map[string]any       // Service name -> factory function
	gracefulShutdownTimeout time.Duration        // Timeout for graceful shutdown
	daemonStartupHooks      []DaemonStartupHook  // Hooks called before server starts
	daemonReadyHooks        []DaemonReadyHook    // Hooks called after server is ready
	daemonShutdownHooks     []DaemonShutdownHook // Hooks called during graceful shutdown
	slogConfig              SlogConfigFunc       // Function to configure slog logger
}

// SetBeforeCommand sets the before hook.
func (o *rootCommandOptions) SetBeforeCommand(fn func(context.Context, *cli.Command) error) {
	o.beforeCommand = fn
}

// SetAfterCommand sets the after hook.
func (o *rootCommandOptions) SetAfterCommand(fn func(context.Context, *cli.Command) error) {
	o.afterCommand = fn
}

// SetOutputFormats sets the output formats.
func (o *rootCommandOptions) SetOutputFormats(formats []OutputFormat) {
	o.outputFormats = formats
}

// AddService adds a service to the root command.
func (o *rootCommandOptions) AddService(service *ServiceCLI) {
	o.services = append(o.services, service)
}

// Services returns the registered services.
func (o *rootCommandOptions) Services() []*ServiceCLI {
	return o.services
}

// BeforeCommand returns the root before hook, or nil if not set.
func (o *rootCommandOptions) BeforeCommand() func(context.Context, *cli.Command) error {
	return o.beforeCommand
}

// AfterCommand returns the root after hook, or nil if not set.
func (o *rootCommandOptions) AfterCommand() func(context.Context, *cli.Command) error {
	return o.afterCommand
}

// OutputFormats returns the root-level output formats.
func (o *rootCommandOptions) OutputFormats() []OutputFormat {
	return o.outputFormats
}

// GRPCServerOptions returns the gRPC server options.
func (o *rootCommandOptions) GRPCServerOptions() []grpc.ServerOption {
	return o.grpcServerOptions
}

// EnableTranscoding returns whether gRPC transcoding is enabled.
func (o *rootCommandOptions) EnableTranscoding() bool {
	return o.enableTranscoding
}

// TranscodingPort returns the port for gRPC transcoding (HTTP/JSON gateway).
func (o *rootCommandOptions) TranscodingPort() int {
	return o.transcodingPort
}

// ConfigPaths returns the config file paths.
func (o *rootCommandOptions) ConfigPaths() []string {
	return o.configPaths
}

// EnvPrefix returns the environment variable prefix.
func (o *rootCommandOptions) EnvPrefix() string {
	return o.envPrefix
}

// ServiceFactory returns the factory function for a service, if registered.
func (o *rootCommandOptions) ServiceFactory(serviceName string) (any, bool) {
	if o.serviceFactories == nil {
		return nil, false
	}
	factory, ok := o.serviceFactories[serviceName]
	return factory, ok
}

// GracefulShutdownTimeout returns the timeout for graceful shutdown.
func (o *rootCommandOptions) GracefulShutdownTimeout() time.Duration {
	if o.gracefulShutdownTimeout == 0 {
		return 15 * time.Second // Default timeout
	}
	return o.gracefulShutdownTimeout
}

// DaemonStartupHooks returns the registered daemon startup hooks.
func (o *rootCommandOptions) DaemonStartupHooks() []DaemonStartupHook {
	return o.daemonStartupHooks
}

// DaemonReadyHooks returns the registered daemon ready hooks.
func (o *rootCommandOptions) DaemonReadyHooks() []DaemonReadyHook {
	return o.daemonReadyHooks
}

// DaemonShutdownHooks returns the registered daemon shutdown hooks.
func (o *rootCommandOptions) DaemonShutdownHooks() []DaemonShutdownHook {
	return o.daemonShutdownHooks
}

// SlogConfig returns the slog configuration function.
func (o *rootCommandOptions) SlogConfig() SlogConfigFunc {
	return o.slogConfig
}

// Option types for type-safe configuration using interface pattern

// ServiceOption interface for service-level configuration.
type ServiceOption interface {
	applyToServiceConfig(*serviceCommandOptions)
}

// RootOption interface for root-level configuration.
type RootOption interface {
	applyToRootConfig(*rootCommandOptions)
}

// SharedOption is a concrete option type that works with both service and root levels.
// It implements both ServiceOption and RootOption interfaces.
type SharedOption func(baseOptions)

var (
	_ ServiceOption = SharedOption(nil)
	_ RootOption    = SharedOption(nil)
)

func (fn SharedOption) applyToServiceConfig(opts *serviceCommandOptions) {
	fn(opts)
}

func (fn SharedOption) applyToRootConfig(opts *rootCommandOptions) {
	fn(opts)
}

// ServiceOnlyOption is a concrete option type that only works with service level.
// It implements only the ServiceOption interface.
type ServiceOnlyOption func(*serviceCommandOptions)

var _ ServiceOption = ServiceOnlyOption(nil)

func (fn ServiceOnlyOption) applyToServiceConfig(opts *serviceCommandOptions) {
	fn(opts)
}

// RootOnlyOption is a concrete option type that only works with root level.
// It implements only the RootOption interface.
type RootOnlyOption func(*rootCommandOptions)

var _ RootOption = RootOnlyOption(nil)

func (fn RootOnlyOption) applyToRootConfig(opts *rootCommandOptions) {
	fn(opts)
}

// WithBeforeCommand registers a hook that runs before each command execution.
// Works with both ServiceCommand and RootCommand.
func WithBeforeCommand(fn func(context.Context, *cli.Command) error) SharedOption {
	return SharedOption(func(o baseOptions) {
		o.SetBeforeCommand(fn)
	})
}

// WithAfterCommand registers a hook that runs after each command execution.
// Works with both ServiceCommand and RootCommand.
func WithAfterCommand(fn func(context.Context, *cli.Command) error) SharedOption {
	return SharedOption(func(o baseOptions) {
		o.SetAfterCommand(fn)
	})
}

// WithOutputFormats registers output formatters for response rendering.
// Works with both ServiceCommand and RootCommand.
func WithOutputFormats(formats ...OutputFormat) SharedOption {
	return SharedOption(func(o baseOptions) {
		o.SetOutputFormats(formats)
	})
}

// Service-only options

// WithFlagDeserializer registers a custom deserializer for a specific message type
// This allows users to implement custom logic for constructing complex proto messages
// from CLI flags, enabling advanced transformations and validation.
//
// Example:
//
//	WithFlagDeserializer("GetUserRequest", func(ctx context.Context, cmd *cli.Command) (proto.Message, error) {
//	    // Custom logic to build GetUserRequest from flags
//	    userId := cmd.String("user-id")
//	    return &pb.GetUserRequest{
//	        UserId: userId,
//	        IncludeDetails: cmd.Bool("details"),
//	    }, nil
//	})
//
// Type-safe: only works with ServiceOptions.
func WithFlagDeserializer(messageName string, deserializer FlagDeserializer) ServiceOnlyOption {
	return ServiceOnlyOption(func(o *serviceCommandOptions) {
		if o.flagDeserializers == nil {
			o.flagDeserializers = make(map[string]FlagDeserializer)
		}
		o.flagDeserializers[messageName] = deserializer
	})
}

// Root-only options

// WithService registers a service CLI (root level only).
// Type-safe: only works with RootOptions.
func WithService(service *ServiceCLI) RootOnlyOption {
	return RootOnlyOption(func(o *rootCommandOptions) {
		o.AddService(service)
	})
}

// WithGRPCServerOptions adds gRPC server options (e.g., for interceptors).
// Type-safe: only works with RootOptions.
func WithGRPCServerOptions(opts ...grpc.ServerOption) RootOnlyOption {
	return RootOnlyOption(func(o *rootCommandOptions) {
		o.grpcServerOptions = append(o.grpcServerOptions, opts...)
	})
}

// WithUnaryInterceptor adds a unary interceptor to the gRPC server.
// Type-safe: only works with RootOptions.
func WithUnaryInterceptor(interceptor grpc.UnaryServerInterceptor) RootOnlyOption {
	return WithGRPCServerOptions(grpc.ChainUnaryInterceptor(interceptor))
}

// WithStreamInterceptor adds a stream interceptor to the gRPC server.
// Type-safe: only works with RootOptions.
func WithStreamInterceptor(interceptor grpc.StreamServerInterceptor) RootOnlyOption {
	return WithGRPCServerOptions(grpc.ChainStreamInterceptor(interceptor))
}

// WithTranscoding enables gRPC-Gateway transcoding (HTTP/JSON to gRPC).
// This allows clients to call gRPC services via REST/JSON on the specified port.
// Type-safe: only works with RootOptions.
func WithTranscoding(httpPort int) RootOnlyOption {
	return RootOnlyOption(func(o *rootCommandOptions) {
		o.enableTranscoding = true
		o.transcodingPort = httpPort
	})
}

// WithConfigFile adds a config file path to load.
// Can be called multiple times to specify multiple config files (deep merge).
// Type-safe: only works with RootOptions.
func WithConfigFile(path string) RootOnlyOption {
	return RootOnlyOption(func(o *rootCommandOptions) {
		o.configPaths = append(o.configPaths, path)
	})
}

// WithEnvPrefix sets the environment variable prefix for config overrides.
// Example: WithEnvPrefix("USERCLI") enables USERCLI_DB_URL env var.
// Type-safe: only works with RootOptions.
func WithEnvPrefix(prefix string) RootOnlyOption {
	return RootOnlyOption(func(o *rootCommandOptions) {
		o.envPrefix = prefix
	})
}

// WithConfigFactory registers a factory function for a service.
// The factory function takes a config message and returns a service implementation.
// Example: WithConfigFactory("userservice", func(cfg *UserServiceConfig) UserServiceServer { ... }).
// Type-safe: only works with RootOptions.
func WithConfigFactory(serviceName string, factory any) RootOnlyOption {
	return RootOnlyOption(func(o *rootCommandOptions) {
		if o.serviceFactories == nil {
			o.serviceFactories = make(map[string]any)
		}
		o.serviceFactories[serviceName] = factory
	})
}

// WithGracefulShutdownTimeout sets the timeout for graceful daemon shutdown.
// Default is 15 seconds if not specified.
// During graceful shutdown, the daemon will wait for in-flight requests to complete.
// before forcefully terminating after this timeout.
// Type-safe: only works with RootOptions.
func WithGracefulShutdownTimeout(timeout time.Duration) RootOnlyOption {
	return RootOnlyOption(func(o *rootCommandOptions) {
		o.gracefulShutdownTimeout = timeout
	})
}

// WithOnDaemonStartup registers a hook that runs before the gRPC server starts listening.
// Multiple hooks can be registered and will run in registration order.
// The hook receives the gRPC server instance and gateway mux (may be nil if transcoding disabled).
// Returning an error prevents the daemon from starting.
// Type-safe: only works with RootOptions.
func WithOnDaemonStartup(hook DaemonStartupHook) RootOnlyOption {
	return RootOnlyOption(func(o *rootCommandOptions) {
		o.daemonStartupHooks = append(o.daemonStartupHooks, hook)
	})
}

// WithOnDaemonReady registers a hook that runs after the gRPC server is listening and ready.
// Multiple hooks can be registered and will run in registration order.
// The hook cannot return errors - errors must be handled within the hook.
// Type-safe: only works with RootOptions.
func WithOnDaemonReady(hook DaemonReadyHook) RootOnlyOption {
	return RootOnlyOption(func(o *rootCommandOptions) {
		o.daemonReadyHooks = append(o.daemonReadyHooks, hook)
	})
}

// WithOnDaemonShutdown registers a hook that runs during graceful shutdown.
// Multiple hooks can be registered and will run in REVERSE registration order.
// The hook runs after stop accepting new connections but before forcing shutdown.
// The context will be cancelled when the graceful shutdown timeout expires.
// The hook cannot return errors - errors must be handled within the hook.
// Type-safe: only works with RootOptions.
func WithOnDaemonShutdown(hook DaemonShutdownHook) RootOnlyOption {
	return RootOnlyOption(func(o *rootCommandOptions) {
		o.daemonShutdownHooks = append(o.daemonShutdownHooks, hook)
	})
}

// WithSlogConfig provides a custom slog logger configuration function.
// If not specified, the framework uses sensible defaults:
//   - Single commands: text-formatted logs to stderr
//   - Daemon mode: JSON-formatted logs to stdout
//
// The function receives a context and a SlogConfigurationContext providing:
//   - IsDaemon(): true for daemon mode, false for single commands
//   - Level(): the configured log level from the --verbosity flag
//
// Type-safe: only works with RootOptions.
func WithSlogConfig(configFunc SlogConfigFunc) RootOnlyOption {
	return RootOnlyOption(func(o *rootCommandOptions) {
		o.slogConfig = configFunc
	})
}

// Helper functions to apply options

// ApplyServiceOptions applies functional options and returns configured service settings.
func ApplyServiceOptions(opts ...ServiceOption) ServiceConfig {
	options := &serviceCommandOptions{}
	for _, opt := range opts {
		opt.applyToServiceConfig(options)
	}

	// If no output formats are registered, use Go format as the default.
	if len(options.outputFormats) == 0 {
		options.outputFormats = []OutputFormat{Go()}
	}

	return options
}

// ApplyRootOptions applies functional options and returns configured root settings.
func ApplyRootOptions(opts ...RootOption) RootConfig {
	options := &rootCommandOptions{}
	for _, opt := range opts {
		opt.applyToRootConfig(options)
	}
	return options
}
