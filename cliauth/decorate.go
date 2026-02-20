package cliauth

import (
	"context"

	"google.golang.org/grpc/metadata"
)

// DecorateContext calls the configured AuthDecorator and appends the resulting
// key-value pairs to the outgoing gRPC metadata on the context.
// Returns the original context unchanged if there is no decorator, on error,
// or when the decorator returns an empty map (lenient — allows unauthenticated
// commands to proceed).
func DecorateContext(ctx context.Context, cfg *Config) context.Context {
	if cfg.Decorator == nil {
		return ctx
	}

	md, err := cfg.Decorator.Decorate(ctx, cfg.Store)
	if err != nil || len(md) == 0 {
		return ctx
	}

	kvs := make([]string, 0, len(md)*2)
	for k, v := range md {
		kvs = append(kvs, k, v)
	}
	return metadata.AppendToOutgoingContext(ctx, kvs...)
}
