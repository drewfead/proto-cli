package main

import (
	"context"
	"fmt"
	"os"
	"time"

	protocli "github.com/drewfead/proto-cli"
	streaming "github.com/drewfead/proto-cli/examples/streaming"
)

type streamingService struct {
	streaming.UnimplementedStreamingServiceServer
}

func (s *streamingService) ListItems(req *streaming.ListItemsRequest, stream streaming.StreamingService_ListItemsServer) error {
	// Generate some sample items
	items := []*streaming.Item{
		{Id: 1, Name: "Item 1", Category: req.Category},
		{Id: 2, Name: "Item 2", Category: req.Category},
		{Id: 3, Name: "Item 3", Category: req.Category},
		{Id: 4, Name: "Item 4", Category: req.Category},
		{Id: 5, Name: "Item 5", Category: req.Category},
	}

	// Apply offset if specified
	if req.Offset != nil && *req.Offset > 0 {
		offset := int(*req.Offset)
		if offset < len(items) {
			items = items[offset:]
		} else {
			items = nil
		}
	}

	// Apply limit if specified
	if req.Limit > 0 && int(req.Limit) < len(items) {
		items = items[:req.Limit]
	}

	// Stream items
	for _, item := range items {
		if err := stream.Send(&streaming.ItemResponse{
			Item:    item,
			Message: "Success",
		}); err != nil {
			return err
		}
		time.Sleep(100 * time.Millisecond) // Simulate delay
	}

	return nil
}

func (s *streamingService) WatchItems(req *streaming.WatchRequest, stream streaming.StreamingService_WatchItemsServer) error {
	// Simulate watching for events
	for i := req.StartId; i < req.StartId+5; i++ {
		event := &streaming.ItemEvent{
			EventType: "created",
			Item: &streaming.Item{
				Id:       i,
				Name:     fmt.Sprintf("Item %d", i),
				Category: "watched",
			},
			Timestamp: time.Now().Unix(),
		}
		if err := stream.Send(event); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil
}

func main() {
	ctx := context.Background()

	// Create service CLI with configuration
	streamingServiceCLI := streaming.StreamingServiceCommand(ctx, &streamingService{},
		protocli.WithOutputFormats(
			protocli.JSON(),
			protocli.YAML(),
		),
	)

	// Create root command with hoisted service (flat command structure)
	rootCmd, err := protocli.RootCommand("streamcli-flat",
		protocli.Service(streamingServiceCLI, protocli.Hoisted()),
		protocli.WithEnvPrefix("STREAMCLI"),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating root command: %v\n", err)
		os.Exit(1)
	}

	rootCmd.Description = `Flat command structure for streaming gRPC service using protocli.Hoisted().

Examples:
  ./streamcli-flat list-items --category books
  ./streamcli-flat list-items --category books --offset 2 --limit 3 --sort-by name
  ./streamcli-flat watch-items --start-id 1

Start server:
  ./streamcli-flat daemonize --port 50051`

	if err := rootCmd.Run(ctx, os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
