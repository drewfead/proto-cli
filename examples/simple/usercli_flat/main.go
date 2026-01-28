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

func main() {
	ctx := context.Background()

	// Create service CLI with configuration
	userServiceCLI := simple.UserServiceCommand(ctx, newUserService,
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

	// Create root command with hoisted service (flat command structure)
	// RPC commands appear at root level as siblings of daemonize
	rootCmd, err := protocli.RootCommand("usercli-flat",
		protocli.WithService(userServiceCLI, protocli.Hoisted()),
		protocli.WithEnvPrefix("USERCLI"),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating root command: %v\n", err)
		os.Exit(1)
	}

	rootCmd.Description = `This CLI demonstrates the flat command structure using protocli.Hoisted().

Commands are at the root level:
  ./usercli-flat get --id 1
  ./usercli-flat create --name "Alice" --email "alice@example.com"

Start a gRPC server:
  ./usercli-flat daemonize --port 50051

Call it remotely:
  ./usercli-flat get --id 1 --remote localhost:50051`

	if err := rootCmd.Run(ctx, os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
