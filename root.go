package protocli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/urfave/cli/v3"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// ServiceCLI represents a service CLI with its command and gRPC registration function.
type ServiceCLI struct {
	Command             *cli.Command
	ServiceName         string                                   // Service name (e.g., "userservice")
	ConfigMessageType   string                                   // Config message type name (empty if no config)
	ConfigPrototype     proto.Message                            // Prototype config message instance (for cloning)
	FactoryOrImpl       any                                      // Factory function or direct service implementation
	RegisterFunc        func(*grpc.Server, any)                  // Register service with gRPC server (takes impl)
	GatewayRegisterFunc func(ctx context.Context, mux any) error // mux is *runtime.ServeMux from grpc-gateway
}

// parseVerbosity parses the verbosity flag value into a slog.Level.
// Supports: "debug"/"4", "info"/"3", "warn"/"2", "error"/"1", "none"/"0".
func parseVerbosity(value string) slog.Level {
	value = strings.ToLower(strings.TrimSpace(value))

	switch value {
	case "debug", "4":
		return slog.LevelDebug
	case "info", "3":
		return slog.LevelInfo
	case "warn", "warning", "2":
		return slog.LevelWarn
	case "error", "1":
		return slog.LevelError
	case "none", "0":
		// Return a very high level to effectively disable logging
		return slog.Level(1000)
	default:
		return slog.LevelInfo
	}
}

// setupSlog configures the global slog logger based on mode and verbosity.
func setupSlog(ctx context.Context, cmd *cli.Command, isDaemon bool, slogConfig SlogConfigFunc) {
	verbosity := cmd.String("verbosity")
	level := parseVerbosity(verbosity)

	// Create configuration context
	configCtx := &slogConfigContext{
		isDaemon: isDaemon,
		level:    level,
	}

	var logger *slog.Logger

	if slogConfig != nil {
		// Use custom slog configuration
		logger = slogConfig(ctx, configCtx)
	} else {
		// Use default configuration
		var output io.Writer
		var handler slog.Handler

		if isDaemon {
			// Daemon mode: JSON to stdout
			output = os.Stdout
			handler = slog.NewJSONHandler(output, &slog.HandlerOptions{
				Level: level,
			})
		} else {
			// Single command mode: Text to stderr
			output = os.Stderr
			handler = slog.NewTextHandler(output, &slog.HandlerOptions{
				Level: level,
			})
		}

		logger = slog.New(handler)
	}

	slog.SetDefault(logger)
}

// NewDaemonizeCommand creates a daemonize command for the given services.
// This is useful for single-service CLIs using the flat command structure.
func NewDaemonizeCommand(_ context.Context, services []*ServiceCLI, _ ServiceConfig) *cli.Command {
	return &cli.Command{
		Name:  "daemonize",
		Usage: "Start a gRPC server",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "host",
				Value: "0.0.0.0",
				Usage: "Host to bind the gRPC server to",
			},
			&cli.IntFlag{
				Name:  "port",
				Value: 50051,
				Usage: "Port to bind the gRPC server to",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			// Create minimal root options for single-service mode
			rootOpts := ApplyRootOptions()
			for _, svc := range services {
				rootOpts = ApplyRootOptions(Service(svc))
			}
			return runDaemon(ctx, cmd, services, rootOpts)
		},
	}
}

// RootCommand creates a root CLI command with the given app name and options.
// Returns an error if there are naming collisions between hoisted service commands.
func RootCommand(appName string, opts ...RootOption) (*cli.Command, error) {
	options := ApplyRootOptions(opts...)

	var commands []*cli.Command
	services := options.Services()
	commandNames := make(map[string]bool) // Track command names for collision detection

	// Setup default config paths if not provided
	configPaths := options.ConfigPaths()
	if len(configPaths) == 0 {
		configPaths = DefaultConfigPaths(appName)
	}

	// Access service registrations to check hoisting
	// Type assert to access internal registrations
	if opts, ok := options.(*rootCommandOptions); ok {
		for _, reg := range opts.ServiceRegistrations() {
			if reg.hoisted {
				// Hoisted: add RPC commands directly to root level
				for _, rpcCmd := range reg.service.Command.Commands {
					if commandNames[rpcCmd.Name] {
						return nil, fmt.Errorf("%w: command '%s' from service '%s'",
							ErrAmbiguousCommandInvocation, rpcCmd.Name, reg.service.ServiceName)
					}
					commandNames[rpcCmd.Name] = true
					commands = append(commands, rpcCmd)
				}
			} else {
				// Not hoisted: add service command as nested
				if commandNames[reg.service.Command.Name] {
					return nil, fmt.Errorf("%w: service command '%s'",
						ErrAmbiguousCommandInvocation, reg.service.Command.Name)
				}
				commandNames[reg.service.Command.Name] = true
				commands = append(commands, reg.service.Command)
			}
		}
	} else {
		// Fallback to old behavior if type assertion fails
		for _, svc := range services {
			commands = append(commands, svc.Command)
		}
	}

	// Check if daemonize command name would collide
	if commandNames["daemonize"] {
		return nil, fmt.Errorf("%w: 'daemonize' is reserved and conflicts with a hoisted service command",
			ErrAmbiguousCommandInvocation)
	}

	// Add daemonize command that registers all services
	commands = append(commands, &cli.Command{
		Name:  "daemonize",
		Usage: "Start a gRPC server with all services",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "host",
				Value: "0.0.0.0",
				Usage: "Host to bind the gRPC server to",
			},
			&cli.IntFlag{
				Name:  "port",
				Value: 50051,
				Usage: "Port to bind the gRPC server to",
			},
			&cli.StringSliceFlag{
				Name:  "service",
				Usage: "Service to enable (by name). Can be specified multiple times. If not specified, all services are enabled. Example: --service userservice --service productservice",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runDaemon(ctx, cmd, services, options)
		},
	})

	// Global flags including --config and --verbosity
	globalFlags := []cli.Flag{
		&cli.StringSliceFlag{
			Name:  "config",
			Value: configPaths,
			Usage: "Config file path (can specify multiple for deep merge)",
		},
		&cli.StringFlag{
			Name:   "env-prefix",
			Value:  options.EnvPrefix(),
			Usage:  "Environment variable prefix for config overrides",
			Hidden: true,
		},
		&cli.StringFlag{
			Name:    "verbosity",
			Aliases: []string{"v"},
			Value:   options.DefaultVerbosity(),
			Usage:   "Log verbosity level (debug/4, info/3, warn/2, error/1, none/0)",
		},
	}

	rootCmd := &cli.Command{
		Name:     appName,
		Usage:    fmt.Sprintf("%s - gRPC service CLI", appName),
		Flags:    globalFlags,
		Commands: commands,
	}

	// Add Before hook to setup slog for non-daemon commands
	rootCmd.Before = func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
		// Setup slog for single command mode (non-daemon)
		// For daemon mode, setupSlog is called in runDaemon
		if cmd.Name != "daemonize" {
			setupSlog(ctx, cmd.Root(), false, options.SlogConfig())
		}
		return ctx, nil
	}

	// Apply help customization if provided
	if helpCustom := options.HelpCustomization(); helpCustom != nil {
		// Set custom help templates if provided
		if helpCustom.RootCommandHelpTemplate != "" {
			rootCmd.CustomRootCommandHelpTemplate = helpCustom.RootCommandHelpTemplate
		}
		// Note: CommandHelpTemplate and SubcommandHelpTemplate are global in urfave/cli
		// They would need to be set via cli.CommandHelpTemplate and cli.SubcommandHelpTemplate
	}

	return rootCmd, nil
}

var (
	ErrWrongConfigType            = errors.New("wrong config type")
	ErrAmbiguousCommandInvocation = errors.New("more than one action registered for the same command")
)

// createServiceImpl loads config and creates service implementation.
func createServiceImpl(
	loader *ConfigLoader,
	cmd *cli.Command,
	svc *ServiceCLI,
	options RootConfig,
) (any, error) {
	// If no config message type, use impl directly (no config needed)
	if svc.ConfigMessageType == "" {
		// Assume FactoryOrImpl is a direct implementation
		return svc.FactoryOrImpl, nil
	}

	// Service has config annotation - need factory function
	factory, hasFactory := options.ServiceFactory(svc.ServiceName)
	if !hasFactory {
		// Try using FactoryOrImpl as the factory
		factory = svc.FactoryOrImpl
	}

	// If we don't have a config prototype, we can't instantiate config
	if svc.ConfigPrototype == nil {
		return nil, fmt.Errorf("%w: service %s has config type %s but no config prototype provided",
			ErrWrongConfigType, svc.ServiceName, svc.ConfigMessageType)
	}

	// 1. Create a new config message instance by cloning the prototype
	config := NewConfigMessage(svc.ConfigPrototype)

	// 2. Load config from files and environment variables using the loader
	if err := loader.LoadServiceConfig(cmd, svc.ServiceName, config); err != nil {
		return nil, fmt.Errorf("failed to load config for %s: %w", svc.ServiceName, err)
	}

	// 3. Call factory with loaded config to create service implementation
	impl, err := CallFactory(factory, config)
	if err != nil {
		return nil, fmt.Errorf("failed to call factory for %s: %w", svc.ServiceName, err)
	}

	return impl, nil
}

// filterServices filters services based on --service flag.
func filterServices(services []*ServiceCLI, enabledNames []string) []*ServiceCLI {
	if len(enabledNames) == 0 {
		return services
	}

	enabledMap := make(map[string]bool)
	for _, name := range enabledNames {
		enabledMap[name] = true
	}

	filtered := make([]*ServiceCLI, 0, len(enabledNames))
	for _, svc := range services {
		if enabledMap[svc.ServiceName] {
			filtered = append(filtered, svc)
		}
	}

	return filtered
}

// runDaemon implements the daemon command with proper signal handling and lifecycle hooks.
func runDaemon(ctx context.Context, cmd *cli.Command, services []*ServiceCLI, options RootConfig) error {
	// Get root command for accessing global flags
	rootCmd := cmd.Root()

	// Setup slog for daemon mode (JSON to stdout)
	setupSlog(ctx, rootCmd, true, options.SlogConfig())

	host := cmd.String("host")
	port := cmd.Int("port")
	address := fmt.Sprintf("%s:%d", host, port)

	// Get config paths from root command
	configFilePaths := rootCmd.StringSlice("config")

	// Create config loader (daemon mode = no flag overrides)
	loader := NewConfigLoader(DaemonMode,
		FileConfig(configFilePaths...),
		EnvPrefix(options.EnvPrefix()),
	)

	// Create service implementations with config
	serviceImpls := make(map[string]any)
	for _, svc := range services {
		impl, err := createServiceImpl(loader, cmd, svc, options)
		if err != nil {
			return fmt.Errorf("failed to create %s: %w", svc.ServiceName, err)
		}
		serviceImpls[svc.ServiceName] = impl
	}

	// Filter services based on --service flag
	enabledServices := cmd.StringSlice("service")
	servicesToRegister := filterServices(services, enabledServices)

	// Warn if requested services weren't found
	if len(enabledServices) > 0 && len(servicesToRegister) != len(enabledServices) {
		registeredNames := make([]string, 0, len(servicesToRegister))
		for _, svc := range servicesToRegister {
			registeredNames = append(registeredNames, svc.ServiceName)
		}
		slog.Warn("Requested services not all found",
			"requested", len(enabledServices),
			"found", len(servicesToRegister),
			"registered", registeredNames)
	}

	// Create gRPC server with configured options
	grpcServer := grpc.NewServer(options.GRPCServerOptions()...)

	// Create gateway mux if transcoding is enabled
	var gwMux *runtime.ServeMux
	if options.EnableTranscoding() {
		gwMux = runtime.NewServeMux()
	}

	// Run OnDaemonStartup hooks (before server starts listening)
	for i, hook := range options.DaemonStartupHooks() {
		if err := hook(ctx, grpcServer, gwMux); err != nil {
			return fmt.Errorf("daemon startup hook %d failed: %w", i, err)
		}
	}

	// Register selected services with their implementations
	for _, svc := range servicesToRegister {
		impl := serviceImpls[svc.ServiceName]
		svc.RegisterFunc(grpcServer, impl)
	}

	// Create TCP listener
	lis, err := (&net.ListenConfig{}).Listen(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", address, err)
	}

	slog.Info("Starting gRPC server", "address", address, "services", len(servicesToRegister))

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server in goroutine
	servErr := make(chan error, 1)
	go func() {
		servErr <- grpcServer.Serve(lis)
	}()

	// Run OnDaemonReady hooks (after server is ready to accept connections)
	for _, hook := range options.DaemonReadyHooks() {
		hook(ctx)
	}

	// Wait for signal or server error
	select {
	case sig := <-sigChan:
		slog.Info("Received signal %v, initiating graceful shutdown...", "shutdown.signal", sig)
		return gracefulShutdown(ctx, grpcServer, options)
	case err := <-servErr:
		if err != nil {
			return fmt.Errorf("server error: %w", err)
		}
		return nil
	}
}

// gracefulShutdown handles graceful shutdown with timeout and hooks.
func gracefulShutdown(ctx context.Context, grpcServer *grpc.Server, options RootConfig) error {
	timeout := options.GracefulShutdownTimeout()
	slog.Warn("Graceful shutdown timed out", "timeout.after", timeout)

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()

	// Run OnDaemonShutdown hooks in REVERSE order
	hooks := options.DaemonShutdownHooks()
	for i := len(hooks) - 1; i >= 0; i-- {
		hooks[i](ctx)
	}

	// Channel to signal when graceful stop completes
	stopped := make(chan struct{})

	// Attempt graceful stop
	go func() {
		grpcServer.GracefulStop()
		close(stopped)
	}()

	// Wait for graceful stop or timeout
	select {
	case <-stopped:
		slog.Info("Graceful shutdown complete")
		return nil
	case <-ctx.Done():
		slog.Warn("Graceful shutdown interrupted, forcing stop")
		grpcServer.Stop()
		return nil
	}
}
