package streaming_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	protocli "github.com/drewfead/proto-cli"
	"github.com/drewfead/proto-cli/examples/streaming"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TestServerStreaming_ListItems_Local tests local (non-remote) streaming
func TestServerStreaming_ListItems_Local(t *testing.T) {
	ctx := context.Background()
	service := streaming.NewStreamingService()

	serviceCLI := streaming.StreamingServiceCommand(ctx, service,
		protocli.WithOutputFormats(protocli.JSON()),
	)

	rootCmd, err := protocli.RootCommand("streamcli",
		protocli.Service(serviceCLI),
	)
	require.NoError(t, err)

	// Run command with --output to write to temp file
	tempFile := t.TempDir() + "/output.txt"
	args := []string{
		"streamcli", "streaming-service", "list-items",
		"--category", "test",
		"--format", "json",
		"--output", tempFile,
	}

	if err := rootCmd.Run(ctx, args); err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	// Read output from file
	output, err := os.ReadFile(tempFile)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	// Split by newlines and filter empty lines
	var lines []string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}

	// Should have 5 items (default)
	if len(lines) != 5 {
		t.Errorf("Expected 5 lines, got %d: %v", len(lines), lines)
	}

	// Each line should be valid JSON with correct category
	for i, line := range lines {
		if !strings.Contains(line, `"item"`) || !strings.Contains(line, `"message"`) {
			t.Errorf("Line %d doesn't look like valid ItemResponse JSON: %s", i, line)
		}
		if !strings.Contains(line, `"category":"test"`) {
			t.Errorf("Line %d doesn't have correct category: %s", i, line)
		}
	}
}

// TestServerStreaming_ListItems_Remote tests remote gRPC streaming
func TestServerStreaming_ListItems_Remote(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start gRPC server
	lc := &net.ListenConfig{}
	lis, err := lc.Listen(ctx, "tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	tcpAddr, ok := lis.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("Failed to get TCP address")
	}
	port := tcpAddr.Port

	server := grpc.NewServer()
	service := streaming.NewStreamingService()
	streaming.RegisterStreamingServiceServer(server, service)

	go func() {
		_ = server.Serve(lis)
	}()
	defer server.Stop()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Create CLI and run remote command
	serviceCLI := streaming.StreamingServiceCommand(ctx, service,
		protocli.WithOutputFormats(protocli.JSON()),
	)

	rootCmd, err := protocli.RootCommand("streamcli",
		protocli.Service(serviceCLI),
	)
	require.NoError(t, err)

	// Write output to temp file
	tempFile := t.TempDir() + "/output.txt"
	args := []string{
		"streamcli", "streaming-service", "list-items",
		"--remote", fmt.Sprintf("localhost:%d", port),
		"--category", "remote",
		"--format", "json",
		"--output", tempFile,
	}

	if err := rootCmd.Run(ctx, args); err != nil {
		t.Fatalf("Remote command failed: %v", err)
	}

	// Read and verify output
	output, err := os.ReadFile(tempFile)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	// Split by newlines and filter empty lines
	var lines []string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}

	if len(lines) != 5 {
		t.Errorf("Expected 5 lines, got %d: %v", len(lines), lines)
	}

	// Verify category is correct
	for _, line := range lines {
		if !strings.Contains(line, `"category":"remote"`) {
			t.Errorf("Line doesn't have correct category: %s", line)
		}
	}
}

// TestServerStreaming_DirectGRPCClient tests streaming using the gRPC client directly
func TestServerStreaming_DirectGRPCClient(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start gRPC server
	lc := &net.ListenConfig{}
	lis, err := lc.Listen(ctx, "tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}

	server := grpc.NewServer()
	service := streaming.NewStreamingService()
	streaming.RegisterStreamingServiceServer(server, service)

	go func() {
		_ = server.Serve(lis)
	}()
	defer server.Stop()

	// Connect as gRPC client
	conn, err := grpc.NewClient(lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Logf("Failed to close connection: %v", err)
		}
	}()

	client := streaming.NewStreamingServiceClient(conn)

	// Call ListItems streaming method
	stream, err := client.ListItems(ctx, &streaming.ListItemsRequest{
		Category: "direct",
		Limit:    3,
	})
	if err != nil {
		t.Fatalf("Failed to call ListItems: %v", err)
	}

	// Receive messages
	var count int
	for {
		msg, err := stream.Recv()
		if err != nil {
			break
		}
		count++
		if msg.Item.Category != "direct" {
			t.Errorf("Expected category 'direct', got %s", msg.Item.Category)
		}
	}

	if count != 3 {
		t.Errorf("Expected 3 messages, got %d", count)
	}
}
