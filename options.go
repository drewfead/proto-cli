package protocli

import (
	"context"
	"io"
	"log/slog"
	"text/template"
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
	BeforeCommandHooks() []func(context.Context, *cli.Command) error
	AfterCommandHooks() []func(context.Context, *cli.Command) error
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
	LoggingConfig() LoggingConfigCallback
	DefaultVerbosity() string
	HelpCustomization() *HelpCustomization
}

// HelpCustomization holds options for customizing help text display.
// Based on urfave/cli v3 help customization capabilities.
type HelpCustomization struct {
	// RootCommandHelpTemplate overrides the default root command help template
	RootCommandHelpTemplate string

	// CommandHelpTemplate overrides the default command help template
	CommandHelpTemplate string

	// SubcommandHelpTemplate overrides the default subcommand help template
	SubcommandHelpTemplate string
}

// Private interfaces for internal use

// baseOptions defines common options interface for both service and root levels.
type baseOptions interface {
	AddBeforeCommand(func(context.Context, *cli.Command) error)
	AddAfterCommand(func(context.Context, *cli.Command) error)
	SetOutputFormats([]OutputFormat)
	BeforeCommandHooks() []func(context.Context, *cli.Command) error
	AfterCommandHooks() []func(context.Context, *cli.Command) error
	OutputFormats() []OutputFormat
}

// rootOptions extends baseOptions with root-specific methods

// Private implementation structs

// serviceCommandOptions holds configuration for individual service commands.
type serviceCommandOptions struct {
	beforeCommandHooks []func(context.Context, *cli.Command) error
	afterCommandHooks  []func(context.Context, *cli.Command) error
	outputFormats      []OutputFormat
	flagDeserializers  map[string]FlagDeserializer // messageName -> deserializer
}

// AddBeforeCommand adds a before command hook.
// Multiple hooks can be registered and will run in registration order.
func (o *serviceCommandOptions) AddBeforeCommand(fn func(context.Context, *cli.Command) error) {
	o.beforeCommandHooks = append(o.beforeCommandHooks, fn)
}

// AddAfterCommand adds an after command hook.
// Multiple hooks can be registered and will run in REVERSE registration order.
func (o *serviceCommandOptions) AddAfterCommand(fn func(context.Context, *cli.Command) error) {
	o.afterCommandHooks = append(o.afterCommandHooks, fn)
}

// SetOutputFormats sets the output formats.
func (o *serviceCommandOptions) SetOutputFormats(formats []OutputFormat) {
	o.outputFormats = formats
}

// BeforeCommandHooks returns the before command hooks.
// These hooks run in registration order (first registered runs first).
func (o *serviceCommandOptions) BeforeCommandHooks() []func(context.Context, *cli.Command) error {
	return o.beforeCommandHooks
}

// AfterCommandHooks returns the after command hooks.
// These hooks run in REVERSE registration order (last registered runs first).
func (o *serviceCommandOptions) AfterCommandHooks() []func(context.Context, *cli.Command) error {
	return o.afterCommandHooks
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

// LoggingConfigCallback is a function that configures the slog logger.
// It receives a context with configuration details and returns a configured logger.
type LoggingConfigCallback func(ctx context.Context, config SlogConfigurationContext) *slog.Logger

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
// serviceRegistration tracks a service and how it should be registered.
type serviceRegistration struct {
	service *ServiceCLI
	hoisted bool // If true, RPC commands added to root instead of nested
}

type rootCommandOptions struct {
	serviceRegistrations    []*serviceRegistration
	beforeCommandHooks      []func(context.Context, *cli.Command) error
	afterCommandHooks       []func(context.Context, *cli.Command) error
	outputFormats           []OutputFormat
	grpcServerOptions       []grpc.ServerOption
	enableTranscoding       bool
	transcodingPort         int
	configPaths             []string              // Config file paths for loading
	envPrefix               string                // Environment variable prefix
	serviceFactories        map[string]any        // Service name -> factory function
	gracefulShutdownTimeout time.Duration         // Timeout for graceful shutdown
	daemonStartupHooks      []DaemonStartupHook   // Hooks called before server starts
	daemonReadyHooks        []DaemonReadyHook     // Hooks called after server is ready
	daemonShutdownHooks     []DaemonShutdownHook  // Hooks called during graceful shutdown
	loggingConfig           LoggingConfigCallback // Function to configure slog logger
	defaultVerbosity        *slog.Level           // Default verbosity level (nil = info)
	helpCustomization       *HelpCustomization    // Help text customization options
	configManager           proto.Message         // Config message for config management command suite
	configServiceName       string                // Service name for config management
	globalConfigPath        string                // Custom global config path
	localConfigPath         string                // Custom local config path
}

// AddBeforeCommand adds a before command hook.
// Multiple hooks can be registered and will run in registration order.
func (o *rootCommandOptions) AddBeforeCommand(fn func(context.Context, *cli.Command) error) {
	o.beforeCommandHooks = append(o.beforeCommandHooks, fn)
}

// AddAfterCommand adds an after command hook.
// Multiple hooks can be registered and will run in REVERSE registration order.
func (o *rootCommandOptions) AddAfterCommand(fn func(context.Context, *cli.Command) error) {
	o.afterCommandHooks = append(o.afterCommandHooks, fn)
}

// SetOutputFormats sets the output formats.
func (o *rootCommandOptions) SetOutputFormats(formats []OutputFormat) {
	o.outputFormats = formats
}

// AddService adds a service to the root command.
func (o *rootCommandOptions) AddService(service *ServiceCLI, hoisted bool) {
	o.serviceRegistrations = append(o.serviceRegistrations, &serviceRegistration{
		service: service,
		hoisted: hoisted,
	})
}

// Services returns the registered services.
func (o *rootCommandOptions) Services() []*ServiceCLI {
	services := make([]*ServiceCLI, 0, len(o.serviceRegistrations))
	for _, reg := range o.serviceRegistrations {
		services = append(services, reg.service)
	}
	return services
}

// ServiceRegistrations returns the service registrations (for internal use by RootCommand).
func (o *rootCommandOptions) ServiceRegistrations() []*serviceRegistration {
	return o.serviceRegistrations
}

// BeforeCommandHooks returns the before command hooks.
// These hooks run in registration order (first registered runs first).
func (o *rootCommandOptions) BeforeCommandHooks() []func(context.Context, *cli.Command) error {
	return o.beforeCommandHooks
}

// AfterCommandHooks returns the after command hooks.
// These hooks run in REVERSE registration order (last registered runs first).
func (o *rootCommandOptions) AfterCommandHooks() []func(context.Context, *cli.Command) error {
	return o.afterCommandHooks
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

// LoggingConfig returns the logging configuration function.
func (o *rootCommandOptions) LoggingConfig() LoggingConfigCallback {
	return o.loggingConfig
}

// DefaultVerbosity returns the default verbosity level as a string.
// Returns "info" if not explicitly set.
func (o *rootCommandOptions) DefaultVerbosity() string {
	if o.defaultVerbosity == nil {
		return "info"
	}
	return slogLevelToString(*o.defaultVerbosity)
}

// HelpCustomization returns the help customization options.
func (o *rootCommandOptions) HelpCustomization() *HelpCustomization {
	return o.helpCustomization
}

// slogLevelToString converts an slog.Level to the CLI verbosity string format.
// Note: In slog, higher numeric values = less verbose logging.
func slogLevelToString(level slog.Level) string {
	switch level {
	case slog.LevelDebug:
		return "debug"
	case slog.LevelInfo:
		return "info"
	case slog.LevelWarn:
		return "warn"
	case slog.LevelError:
		return "error"
	default:
		// Very high levels (1000+) effectively disable logging
		if level >= 1000 {
			return "none"
		}
		// Levels below Debug (-4) map to debug for maximum visibility
		// Levels between standard levels default to the next higher verbosity
		if level < slog.LevelDebug {
			return "debug"
		}
		if level < slog.LevelInfo {
			return "debug"
		}
		if level < slog.LevelWarn {
			return "info"
		}
		if level < slog.LevelError {
			return "warn"
		}
		return "error"
	}
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

// BeforeCommand registers a hook that runs before each command execution.
// Multiple hooks can be registered and will run in registration order.
// Works with both ServiceCommand and RootCommand.
func BeforeCommand(fn func(context.Context, *cli.Command) error) SharedOption {
	return SharedOption(func(o baseOptions) {
		o.AddBeforeCommand(fn)
	})
}

// AfterCommand registers a hook that runs after each command execution.
// Works with both ServiceCommand and RootCommand.
// Multiple hooks can be registered and will run in REVERSE registration order.
// This allows cleanup to happen in the opposite order of setup (LIFO pattern).
// Works with both ServiceCommand and RootCommand.
func AfterCommand(fn func(context.Context, *cli.Command) error) SharedOption {
	return SharedOption(func(o baseOptions) {
		o.AddAfterCommand(fn)
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

// ServiceRegistrationOption configures how a service is registered in the root command.
type ServiceRegistrationOption func(*serviceRegistration)

// Hoisted returns an option that hoists service RPC commands to the root level.
// When hoisted, RPC commands appear as siblings of the daemonize command instead of nested under the service name.
// Multiple services can be hoisted - naming collisions will cause a runtime error.
// Example: protocli.WithService(serviceCLI, protocli.Hoisted())
func Hoisted() ServiceRegistrationOption {
	return func(reg *serviceRegistration) {
		reg.hoisted = true
	}
}

// Service registers a service CLI (root level only).
// Accepts optional ServiceRegistrationOptions to customize registration (e.g., Hoisted()).
// Type-safe: only works with RootOptions.
func Service(service *ServiceCLI, opts ...ServiceRegistrationOption) RootOnlyOption {
	return RootOnlyOption(func(o *rootCommandOptions) {
		reg := &serviceRegistration{
			service: service,
			hoisted: false,
		}
		for _, opt := range opts {
			opt(reg)
		}
		o.serviceRegistrations = append(o.serviceRegistrations, reg)
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

// OnDaemonStartup registers a hook that runs before the gRPC server starts listening.
// Multiple hooks can be registered and will run in registration order.
// The hook receives the gRPC server instance and gateway mux (may be nil if transcoding disabled).
// Returning an error prevents the daemon from starting.
// Type-safe: only works with RootOptions.
func OnDaemonStartup(hook DaemonStartupHook) RootOnlyOption {
	return RootOnlyOption(func(o *rootCommandOptions) {
		o.daemonStartupHooks = append(o.daemonStartupHooks, hook)
	})
}

// OnDaemonReady registers a hook that runs after the gRPC server is listening and ready.
// Multiple hooks can be registered and will run in registration order.
// The hook cannot return errors - errors must be handled within the hook.
// Type-safe: only works with RootOptions.
func OnDaemonReady(hook DaemonReadyHook) RootOnlyOption {
	return RootOnlyOption(func(o *rootCommandOptions) {
		o.daemonReadyHooks = append(o.daemonReadyHooks, hook)
	})
}

// OnDaemonShutdown registers a hook that runs during graceful shutdown.
// Multiple hooks can be registered and will run in REVERSE registration order.
// The hook runs after stop accepting new connections but before forcing shutdown.
// The context will be cancelled when the graceful shutdown timeout expires.
// The hook cannot return errors - errors must be handled within the hook.
// Type-safe: only works with RootOptions.
func OnDaemonShutdown(hook DaemonShutdownHook) RootOnlyOption {
	return RootOnlyOption(func(o *rootCommandOptions) {
		o.daemonShutdownHooks = append(o.daemonShutdownHooks, hook)
	})
}

// ConfigureLogging provides a custom slog logger configuration function.
// If not specified, the framework uses sensible defaults:
//   - Single commands: human-friendly colored logs to stderr (via clilog.HumanFriendlySlogHandler)
//   - Daemon mode: JSON-formatted logs to stdout
//
// The function receives a context and a SlogConfigurationContext providing:
//   - IsDaemon(): true for daemon mode, false for single commands
//   - Level(): the configured log level from the --verbosity flag
//
// IMPORTANT: Your custom logger factory MUST respect config.Level() to honor the --verbosity flag.
//
// Type-safe: only works with RootOptions.
//
// Example - Custom handler that respects verbosity:
//
//	protocli.ConfigureLogging(func(ctx context.Context, config protocli.SlogConfigurationContext) *slog.Logger {
//	    handler := clilog.HumanFriendlySlogHandler(os.Stderr, &slog.HandlerOptions{
//	        Level: config.Level(),  // IMPORTANT: Use config.Level() to respect --verbosity flag
//	    })
//	    return slog.New(handler)
//	})
//
// Example - Different loggers for daemon vs single-command mode:
//
//	protocli.ConfigureLogging(func(ctx context.Context, config protocli.SlogConfigurationContext) *slog.Logger {
//	    if config.IsDaemon() {
//	        handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: config.Level()})
//	        return slog.New(handler)
//	    }
//	    handler := clilog.HumanFriendlySlogHandler(os.Stderr, &slog.HandlerOptions{Level: config.Level()})
//	    return slog.New(handler)
//	})
//
// Example - Use the convenience function for always human-friendly logging:
//
//	protocli.ConfigureLogging(clilog.AlwaysHumanFriendly())
func ConfigureLogging(configFunc LoggingConfigCallback) RootOnlyOption {
	return RootOnlyOption(func(o *rootCommandOptions) {
		o.loggingConfig = configFunc
	})
}

// WithDefaultVerbosity sets the default verbosity level for the --verbosity flag.
// Accepts standard slog.Level values: slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError.
// Note: In slog, higher numeric values = less verbose logging:
//   - slog.LevelDebug (-4) = most verbose
//   - slog.LevelInfo (0) = normal
//   - slog.LevelWarn (4) = warnings and errors only
//   - slog.LevelError (8) = errors only
//   - slog.Level(1000) or higher = effectively disables logging
//
// Default is slog.LevelInfo if not specified.
// Users can still override via the --verbosity flag or -v shorthand.
// Type-safe: only works with RootOptions.
//
// Example:
//
//	protocli.WithDefaultVerbosity(slog.LevelDebug)    // Most verbose (debug and above)
//	protocli.WithDefaultVerbosity(slog.LevelWarn)     // Less verbose (warn and error only)
//	protocli.WithDefaultVerbosity(slog.Level(1000))   // Disable logging
func WithDefaultVerbosity(level slog.Level) RootOnlyOption {
	return RootOnlyOption(func(o *rootCommandOptions) {
		o.defaultVerbosity = &level
	})
}

// WithHelpCustomization sets custom help templates and printer functions.
// This allows full customization of help text display following urfave/cli v3 patterns.
//
// Example:
//
//	protocli.WithHelpCustomization(&protocli.HelpCustomization{
//	    RootCommandHelpTemplate: myCustomTemplate,
//	    CustomizeRootCommand: func(cmd *cli.Command) {
//	        cmd.Usage = "My custom usage text"
//	    },
//	})
func WithHelpCustomization(custom *HelpCustomization) RootOnlyOption {
	return RootOnlyOption(func(o *rootCommandOptions) {
		o.helpCustomization = custom
	})
}

// WithRootCommandHelpTemplate sets a custom template for root command help.
// This is a convenience function for the most common help customization.
//
// Example:
//
//	protocli.WithRootCommandHelpTemplate(`
//	NAME:
//	   {{.Name}} - {{.Usage}}
//
//	USAGE:
//	   {{.HelpName}} {{if .VisibleFlags}}[options]{{end}} command [command options]
//
//	VERSION:
//	   {{.Version}}
//	`)
func WithRootCommandHelpTemplate(template string) RootOnlyOption {
	return RootOnlyOption(func(o *rootCommandOptions) {
		if o.helpCustomization == nil {
			o.helpCustomization = &HelpCustomization{}
		}
		o.helpCustomization.RootCommandHelpTemplate = template
	})
}

// WithCustomizeRootCommand allows modifying the root command after creation.
// This is useful for adding custom fields, metadata, or other modifications.
//
// Example:
//
//	protocli.WithCustomizeRootCommand(func(cmd *cli.Command) {
//	    cmd.Version = "1.0.0"
//	    cmd.Copyright = "(c) 2026 MyCompany"
//	    cmd.Authors = []any{"John Doe <john@example.com>"}
//	})

// WithConfigManagementCommands enables the config command suite (init, set, get, list).
// This adds 'config' subcommands to the root CLI for managing configuration files.
// Config files are YAML-based and validated against the service's config proto schema.
//
// By default:
//   - Global config: ~/.config/appname/config.yaml
//   - Local config: ./.appname/config.yaml
//
// Use WithGlobalConfigPath and WithLocalConfigPath to customize locations.
//
// Example:
//
//	protocli.WithConfigManagementCommands(&simple.UserServiceConfig{}, "myapp", "userservice")
func WithConfigManagementCommands(configMsg proto.Message, appName string, serviceName string) RootOnlyOption {
	return RootOnlyOption(func(o *rootCommandOptions) {
		o.configManager = configMsg
		o.configServiceName = serviceName
		// Paths will be set from configPaths in RootCommand
		// Use WithLocalConfigPath/WithGlobalConfigPath to override
	})
}

// WithGlobalConfigPath sets a custom global config file path.
// This overrides the default ~/.config/appname/config.yaml location.
// Type-safe: only works with RootOptions.
//
// Example:
//
//	protocli.WithGlobalConfigPath("/etc/myapp/config.yaml")
func WithGlobalConfigPath(path string) RootOnlyOption {
	return RootOnlyOption(func(o *rootCommandOptions) {
		o.globalConfigPath = path
	})
}

// WithLocalConfigPath sets a custom local config file path.
// This overrides the default ./.appname/config.yaml location.
// Type-safe: only works with RootOptions.
//
// Example:
//
//	protocli.WithLocalConfigPath("./config.yaml")
func WithLocalConfigPath(path string) RootOnlyOption {
	return RootOnlyOption(func(o *rootCommandOptions) {
		o.localConfigPath = path
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

// TemplateFunctionRegistry manages custom template functions for use in template-based output formats.
// It provides a way to register custom functions that templates can use to format proto messages.
type TemplateFunctionRegistry struct {
	functions template.FuncMap
}

// NewTemplateFunctionRegistry creates a new registry with default template functions.
// Default functions include:
//   - protoField: access message fields by JSON name, preserving proto types
//   - protoJSON: converts a proto message to JSON string using protojson
//   - protoJSONIndent: converts a proto message to indented JSON string
//   - protoFields: converts a proto message to map for dot-chain field access
func NewTemplateFunctionRegistry() *TemplateFunctionRegistry {
	return &TemplateFunctionRegistry{
		functions: DefaultTemplateFunctions(),
	}
}

// Register adds or replaces a template function.
// If a function with the same name already exists, it will be replaced.
func (r *TemplateFunctionRegistry) Register(name string, fn any) {
	r.functions[name] = fn
}

// RegisterMap adds multiple template functions at once.
// Existing functions with the same names will be replaced.
func (r *TemplateFunctionRegistry) RegisterMap(funcMap template.FuncMap) {
	for name, fn := range funcMap {
		r.functions[name] = fn
	}
}

// Functions returns the complete set of registered template functions.
// This includes both default functions and any user-registered functions.
func (r *TemplateFunctionRegistry) Functions() template.FuncMap {
	// Return a copy to prevent external modification
	result := make(template.FuncMap, len(r.functions))
	for k, v := range r.functions {
		result[k] = v
	}
	return result
}

// Global template function registry that can be accessed by generated code
//
//nolint:gochecknoglobals // intentional global registry for template functions
var globalTemplateFunctionRegistry = NewTemplateFunctionRegistry()

// TemplateFunctions returns the global template function registry.
// This can be used to register custom template functions globally.
//
// Example:
//
//	protocli.TemplateFunctions().Register("formatDate", func(ts *timestamppb.Timestamp) string {
//	    return ts.AsTime().Format("2006-01-02")
//	})
func TemplateFunctions() *TemplateFunctionRegistry {
	return globalTemplateFunctionRegistry
}
