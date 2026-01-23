package protocli

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/urfave/cli/v3"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// ServiceCLI represents a service CLI with its command and gRPC registration function
type ServiceCLI struct {
	Command             *cli.Command
	ServiceName         string                                    // Service name (e.g., "userservice")
	ConfigMessageType   string                                    // Config message type name (empty if no config)
	ConfigPrototype     proto.Message                             // Prototype config message instance (for cloning)
	FactoryOrImpl       interface{}                               // Factory function or direct service implementation
	RegisterFunc        func(*grpc.Server, interface{})           // Register service with gRPC server (takes impl)
	GatewayRegisterFunc func(ctx context.Context, mux any) error  // mux is *runtime.ServeMux from grpc-gateway
}

// RootCommand creates a root CLI command with the given app name and options
func RootCommand(appName string, opts ...RootOption) *cli.Command {
	options := ApplyRootOptions(opts...)

	var commands []*cli.Command
	services := options.Services()

	// Setup default config paths if not provided
	configPaths := options.ConfigPaths()
	if len(configPaths) == 0 {
		configPaths = DefaultConfigPaths(appName)
	}

	// Add each service as a subcommand
	for _, svc := range services {
		commands = append(commands, svc.Command)
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

	// Global flags including --config
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
	}

	return &cli.Command{
		Name:     appName,
		Usage:    fmt.Sprintf("%s - gRPC service CLI", appName),
		Flags:    globalFlags,
		Commands: commands,
	}
}

// createServiceImpl loads config and creates service implementation
func createServiceImpl(
	loader *ConfigLoader,
	cmd *cli.Command,
	svc *ServiceCLI,
	options RootConfig,
) (interface{}, error) {
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
		return nil, fmt.Errorf("service %s has config type %s but no config prototype provided",
			svc.ServiceName, svc.ConfigMessageType)
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

// filterServices filters services based on --service flag
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

// runDaemon implements the daemon command with proper signal handling and lifecycle hooks
func runDaemon(ctx context.Context, cmd *cli.Command, services []*ServiceCLI, options RootConfig) error {
	host := cmd.String("host")
	port := cmd.Int("port")
	address := fmt.Sprintf("%s:%d", host, port)

	// Get config paths from root command
	rootCmd := cmd.Root()
	configFilePaths := rootCmd.StringSlice("config")

	// Create config loader (daemon mode = no flag overrides)
	loader := NewConfigLoader(DaemonMode,
		FileConfig(configFilePaths...),
		EnvPrefix(options.EnvPrefix()),
	)

	// Create service implementations with config
	serviceImpls := make(map[string]interface{})
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
		fmt.Fprintf(os.Stderr, "Warning: Requested %d service(s) but only found %d: %v\n",
			len(enabledServices), len(servicesToRegister), registeredNames)
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
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", address, err)
	}

	fmt.Fprintf(os.Stdout, "Starting gRPC server on %s with %d service(s)\n", address, len(servicesToRegister))

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
		fmt.Fprintf(os.Stdout, "\nReceived signal %v, initiating graceful shutdown...\n", sig)
		return gracefulShutdown(ctx, grpcServer, options)
	case err := <-servErr:
		if err != nil {
			return fmt.Errorf("server error: %w", err)
		}
		return nil
	}
}

// gracefulShutdown handles graceful shutdown with timeout and hooks
func gracefulShutdown(ctx context.Context, grpcServer *grpc.Server, options RootConfig) error {
	timeout := options.GracefulShutdownTimeout()
	fmt.Fprintf(os.Stdout, "Graceful shutdown timeout: %v\n", timeout)

	// Create shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Run OnDaemonShutdown hooks in REVERSE order
	hooks := options.DaemonShutdownHooks()
	for i := len(hooks) - 1; i >= 0; i-- {
		hooks[i](shutdownCtx)
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
		fmt.Fprintf(os.Stdout, "Graceful shutdown completed\n")
		return nil
	case <-shutdownCtx.Done():
		fmt.Fprintf(os.Stderr, "Graceful shutdown timeout exceeded, forcing stop\n")
		grpcServer.Stop()
		return nil
	}
}
