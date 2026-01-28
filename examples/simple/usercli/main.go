package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	protocli "github.com/drewfead/proto-cli"
	simple "github.com/drewfead/proto-cli/examples/simple"

	v3 "github.com/urfave/cli/v3"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// userService implements simple.UserServiceServer.
type userService struct {
	simple.UnimplementedUserServiceServer

	dbURL    string
	maxConns int64
	users    map[int64]*simple.User
}

// newUserService is a factory function that creates a UserService with config.
func newUserService(config *simple.UserServiceConfig) simple.UserServiceServer {
	log.Printf("Creating UserService with config: DB=%s, MaxConns=%d",
		config.DatabaseUrl, config.MaxConnections)

	return &userService{
		dbURL:    config.DatabaseUrl,
		maxConns: config.MaxConnections,
		users: map[int64]*simple.User{
			1: {
				Id:        1,
				Name:      "Demo User",
				Email:     "demo@example.com",
				CreatedAt: timestamppb.New(time.Now()),
			},
		},
	}
}

func (s *userService) GetUser(_ context.Context, req *simple.GetUserRequest) (*simple.UserResponse, error) {
	user, exists := s.users[req.Id]
	if !exists {
		return &simple.UserResponse{
			Message: "User not found",
		}, nil
	}
	return &simple.UserResponse{
		User:    user,
		Message: "Success",
	}, nil
}

func (s *userService) CreateUser(_ context.Context, req *simple.CreateUserRequest) (*simple.UserResponse, error) {
	id := int64(len(s.users) + 1)
	user := &simple.User{
		Id:        id,
		Name:      req.Name,
		Email:     req.Email,
		CreatedAt: timestamppb.New(time.Now()),
	}
	s.users[id] = user
	return &simple.UserResponse{
		User:    user,
		Message: "User created successfully",
	}, nil
}

// adminService implements simple.AdminServiceServer.
type adminService struct {
	simple.UnimplementedAdminServiceServer
}

func (s *adminService) HealthCheck(_ context.Context, _ *simple.AdminRequest) (*simple.AdminResponse, error) {
	return &simple.AdminResponse{
		Message: "Service is healthy",
		Success: true,
	}, nil
}

func main() {
	ctx := context.Background()

	// Build service CLI with factory function, lifecycle hooks, and output formats
	// Pass the factory function (not the impl) - config will be loaded automatically
	userServiceCLI := simple.UserServiceServiceCommand(ctx, newUserService,
		protocli.WithBeforeCommand(func(_ context.Context, cmd *v3.Command) error {
			log.Printf("[HOOK] Starting command: %s", cmd.Name)
			return nil
		}),
		protocli.WithAfterCommand(func(_ context.Context, cmd *v3.Command) error {
			log.Printf("[HOOK] Completed command: %s", cmd.Name)
			return nil
		}),
		protocli.WithOutputFormats(
			protocli.JSON(),
			protocli.YAML(),
			protocli.Go(),
		),
	)

	// Build admin service CLI - demonstrates service name override
	// Service name is "admin" (not "admin-service") due to cli.service annotation
	adminServiceCLI := simple.AdminServiceServiceCommand(ctx, &adminService{},
		protocli.WithOutputFormats(
			protocli.JSON(),
		),
	)

	// Create root CLI with all services using the new API
	rootCmd := protocli.RootCommand("usercli",
		protocli.WithService(userServiceCLI),
		protocli.WithService(adminServiceCLI),
		protocli.WithConfigFactory("userservice", newUserService),
		protocli.WithEnvPrefix("USERCLI"),
		// Config files are loaded from:
		//   ./usercli.yaml (default)
		//   ~/.config/usercli/config.yaml (default)
		// Add custom paths with:
		// protocli.WithConfigFile("/path/to/custom.yaml"),
	)

	rootCmd.Usage = "User Service CLI - Multi-service support"
	rootCmd.Description = `This CLI was generated from protobuf definitions.

Commands are organized by service:
  ./usercli <service> <rpc> [flags]

Example:
  ./usercli userservice getuser --id 1
  ./usercli userservice createuser --name "Alice" --email "alice@example.com"

You can also start a gRPC server with all services:
  ./usercli daemonize --port 50051

And call it remotely:
  ./usercli userservice getuser --id 1 --remote localhost:50051`

	if err := rootCmd.Run(ctx, os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
