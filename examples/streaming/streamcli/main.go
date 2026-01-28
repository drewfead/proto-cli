package main

import (
	"context"
	"fmt"
	"os"

	protocli "github.com/drewfead/proto-cli"
	"github.com/drewfead/proto-cli/examples/streaming"
)

func main() {
	ctx := context.Background()

	service := streaming.NewStreamingService()

	serviceCLI := streaming.StreamingServiceCommand(ctx, service,
		protocli.WithOutputFormats(
			protocli.JSON(),
			protocli.YAML(),
			protocli.Go(),
		),
	)

	// Create root CLI with the streaming service
	rootCmd, err := protocli.RootCommand("streamcli",
		protocli.WithService(serviceCLI),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating root command: %v\n", err)
		os.Exit(1)
	}

	rootCmd.Usage = "Streaming Service CLI - Server streaming examples"
	rootCmd.Description = `This CLI demonstrates server streaming support.

Commands:
  ./streamcli streaming-service list-items [flags]
  ./streamcli streaming-service watch-items [flags]

Example:
  ./streamcli streaming-service list-items --category books --format json
  ./streamcli streaming-service list-items --format yaml
  ./streamcli streaming-service list-items --format json | jq .

You can also start a gRPC server:
  ./streamcli daemonize --port 50051

And call it remotely:
  ./streamcli streaming-service list-items --remote localhost:50051`

	if err := rootCmd.Run(ctx, os.Args); err != nil {
		os.Exit(1)
	}
}
