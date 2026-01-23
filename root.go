package protocli

import (
	"context"
	"fmt"
	"net"
	"os"

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

			// Create TCP listener
			lis, err := net.Listen("tcp", address)
			if err != nil {
				return fmt.Errorf("failed to listen on %s: %w", address, err)
			}

			// Create gRPC server with configured options
			grpcServer := grpc.NewServer(options.GRPCServerOptions()...)

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

			// Register selected services with their implementations
			for _, svc := range servicesToRegister {
				impl := serviceImpls[svc.ServiceName]
				svc.RegisterFunc(grpcServer, impl)
			}

			fmt.Fprintf(os.Stdout, "Starting gRPC server on %s with %d service(s)\n", address, len(servicesToRegister))

			// If transcoding is enabled, start HTTP gateway server
			if options.EnableTranscoding() {
				httpPort := options.TranscodingPort()
				if httpPort == 0 {
					httpPort = port + 1000 // Default offset
				}

				// Check if any service has gateway registration
				hasGateway := false
				for _, svc := range servicesToRegister {
					if svc.GatewayRegisterFunc != nil {
						hasGateway = true
						break
					}
				}

				if !hasGateway {
					fmt.Fprintf(os.Stderr, "Warning: gRPC-Gateway transcoding enabled but no gateway handlers available\n")
					fmt.Fprintf(os.Stderr, "Generate gateway code with: buf generate --template buf.gen.gateway.yaml\n")
				} else {
					// Gateway implementation will be added when grpc-gateway support is generated
					// For now, just log that it's configured
					fmt.Fprintf(os.Stdout, "gRPC-Gateway transcoding configured for port %d\n", httpPort)
					fmt.Fprintf(os.Stdout, "Note: Start gateway server implementation pending\n")
				}
			}

			// Start serving (blocking)
			if err := grpcServer.Serve(lis); err != nil {
				return fmt.Errorf("failed to serve: %w", err)
			}

			return nil
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
