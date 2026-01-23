package protocli

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/urfave/cli/v3"
	"google.golang.org/grpc"
)

// ServiceCLI represents a service CLI with its command and gRPC registration function
type ServiceCLI struct {
	Command             *cli.Command
	RegisterFunc        func(*grpc.Server)
	GatewayRegisterFunc func(ctx context.Context, mux any) error // mux is *runtime.ServeMux from grpc-gateway
}

// RootCommand creates a root CLI command with the given app name and options
func RootCommand(appName string, opts ...RootOption) *cli.Command {
	options := ApplyRootOptions(opts...)

	var commands []*cli.Command
	services := options.Services()

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
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			host := cmd.String("host")
			port := cmd.Int("port")
			address := fmt.Sprintf("%s:%d", host, port)

			// Create TCP listener
			lis, err := net.Listen("tcp", address)
			if err != nil {
				return fmt.Errorf("failed to listen on %s: %w", address, err)
			}

			// Create gRPC server with configured options
			grpcServer := grpc.NewServer(options.GRPCServerOptions()...)

			// Register all services
			for _, svc := range services {
				svc.RegisterFunc(grpcServer)
			}

			fmt.Fprintf(os.Stdout, "Starting gRPC server on %s with %d service(s)\n", address, len(services))

			// If transcoding is enabled, start HTTP gateway server
			if options.EnableTranscoding() {
				httpPort := options.TranscodingPort()
				if httpPort == 0 {
					httpPort = port + 1000 // Default offset
				}

				// Check if any service has gateway registration
				hasGateway := false
				for _, svc := range services {
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

	return &cli.Command{
		Name:     appName,
		Usage:    fmt.Sprintf("%s - gRPC service CLI", appName),
		Commands: commands,
	}
}
