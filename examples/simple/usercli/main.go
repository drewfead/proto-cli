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
)

// userService implements simple.UserServiceServer
type userService struct {
	simple.UnimplementedUserServiceServer
	users map[int64]*simple.User
}

func (s *userService) GetUser(ctx context.Context, req *simple.GetUserRequest) (*simple.UserResponse, error) {
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

func (s *userService) CreateUser(ctx context.Context, req *simple.CreateUserRequest) (*simple.UserResponse, error) {
	id := int64(len(s.users) + 1)
	user := &simple.User{
		Id:        id,
		Name:      req.Name,
		Email:     req.Email,
		CreatedAt: time.Now().Unix(),
	}
	s.users[id] = user
	return &simple.UserResponse{
		User:    user,
		Message: "User created successfully",
	}, nil
}

func main() {
	// Create service implementation with some demo data
	serviceImpl := &userService{
		users: map[int64]*simple.User{
			1: {
				Id:        1,
				Name:      "Demo User",
				Email:     "demo@example.com",
				CreatedAt: time.Now().Unix(),
			},
		},
	}

	ctx := context.Background()

	// Build service CLI with implementation, lifecycle hooks, and output formats
	userServiceCLI := simple.UserServiceServiceCommand(ctx, serviceImpl,
		protocli.WithBeforeCommand(func(ctx context.Context, cmd *v3.Command) error {
			log.Printf("[HOOK] Starting command: %s", cmd.Name)
			return nil
		}),
		protocli.WithAfterCommand(func(ctx context.Context, cmd *v3.Command) error {
			log.Printf("[HOOK] Completed command: %s", cmd.Name)
			return nil
		}),
		protocli.WithOutputFormats(
			protocli.JSON(),
			protocli.YAML(),
			protocli.Go(),
		),
	)

	// Create root CLI with all services using the new API
	rootCmd := protocli.RootCommand("usercli",
		protocli.WithService(userServiceCLI),
		// You could add more services here:
		// protocli.WithService(productServiceCLI),
		// protocli.WithService(orderServiceCLI),
		// Or add root-level hooks/formats:
		// protocli.WithRootOutputFormats(protocli.JSON(), protocli.YAML()),
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
