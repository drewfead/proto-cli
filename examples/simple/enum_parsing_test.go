package simple_test

import (
	"bytes"
	"context"
	"testing"

	protocli "github.com/drewfead/proto-cli"
	"github.com/drewfead/proto-cli/examples/simple"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

// TestIntegration_EnumParsing_StringValues tests that enums can be parsed from string values
func TestIntegration_EnumParsing_StringValues(t *testing.T) {
	tests := []struct {
		name          string
		flagValue     string
		expectedEnum  simple.LogLevel
		expectError   bool
		errorContains string
	}{
		{
			name:         "parse custom CLI name - debug",
			flagValue:    "debug",
			expectedEnum: simple.LogLevel_DEBUG,
		},
		{
			name:         "parse custom CLI name - info",
			flagValue:    "info",
			expectedEnum: simple.LogLevel_INFO,
		},
		{
			name:         "parse custom CLI name - warn",
			flagValue:    "warn",
			expectedEnum: simple.LogLevel_WARN,
		},
		{
			name:         "parse custom CLI name - error",
			flagValue:    "error",
			expectedEnum: simple.LogLevel_ERROR,
		},
		{
			name:         "parse uppercase - DEBUG",
			flagValue:    "DEBUG",
			expectedEnum: simple.LogLevel_DEBUG,
		},
		{
			name:         "parse mixed case - Info",
			flagValue:    "Info",
			expectedEnum: simple.LogLevel_INFO,
		},
		{
			name:         "parse as number - 1",
			flagValue:    "1",
			expectedEnum: simple.LogLevel_DEBUG,
		},
		{
			name:         "parse as number - 2",
			flagValue:    "2",
			expectedEnum: simple.LogLevel_INFO,
		},
		{
			name:          "invalid value",
			flagValue:     "invalid",
			expectError:   true,
			errorContains: "invalid LogLevel value",
		},
	}

	// Setup test CLI
	origExiter := cli.OsExiter
	t.Cleanup(func() { cli.OsExiter = origExiter })
	cli.OsExiter = func(_ int) {}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create a mock service factory
			var capturedRequest *simple.CreateUserRequest
			mockServiceFactory := func(_ *simple.UserServiceConfig) simple.UserServiceServer {
				return &mockUserServiceWithCapture{
					onCreateUser: func(req *simple.CreateUserRequest) {
						capturedRequest = req
					},
				}
			}

			userServiceCLI := simple.UserServiceCommand(ctx, mockServiceFactory)
			rootCmd, err := protocli.RootCommand("testcli", protocli.Service(userServiceCLI))
			require.NoError(t, err)

			// Capture output
			var buf bytes.Buffer
			rootCmd.Writer = &buf
			setWriterOnAllCommands(rootCmd, &buf)

			// Execute create command with enum flag
			args := []string{
				"testcli", "user-service", "create",
				"--name", "Test User",
				"--email", "test@example.com",
				"--log-level", tt.flagValue,
			}
			err = rootCmd.Run(ctx, args)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					require.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, capturedRequest)
				require.NotNil(t, capturedRequest.LogLevel)
				require.Equal(t, tt.expectedEnum, *capturedRequest.LogLevel)
			}
		})
	}
}

// mockUserServiceWithCapture is a mock service that captures create requests
type mockUserServiceWithCapture struct {
	simple.UnimplementedUserServiceServer

	onCreateUser func(*simple.CreateUserRequest)
}

func (m *mockUserServiceWithCapture) CreateUser(_ context.Context, req *simple.CreateUserRequest) (*simple.UserResponse, error) {
	if m.onCreateUser != nil {
		m.onCreateUser(req)
	}
	return &simple.UserResponse{
		User: &simple.User{
			Id:    1,
			Name:  req.Name,
			Email: req.Email,
		},
	}, nil
}
