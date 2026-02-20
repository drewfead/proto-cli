package cliauth

import (
	"context"
	"errors"
	"io"

	"github.com/urfave/cli/v3"
)

// ErrNotFound is returned by AuthStore.Load when no credentials are stored.
var ErrNotFound = errors.New("credentials not found")

// AuthStore provides credential persistence for authentication tokens.
type AuthStore interface {
	Save(ctx context.Context, token []byte) error
	Load(ctx context.Context) ([]byte, error)
	Delete(ctx context.Context) error
}

// LoginProvider handles flag-based authentication login.
type LoginProvider interface {
	Flags() []cli.Flag
	Login(ctx context.Context, cmd *cli.Command, store AuthStore) error
}

// InteractiveLoginProvider extends LoginProvider with interactive prompting support.
type InteractiveLoginProvider interface {
	LoginProvider
	LoginInteractive(ctx context.Context, in io.Reader, out io.Writer, store AuthStore) error
}

// LogoutProvider adds logout capability to the auth command suite.
type LogoutProvider interface {
	Logout(ctx context.Context, store AuthStore) error
}

// StatusProvider adds status reporting to the auth command suite.
type StatusProvider interface {
	Status(ctx context.Context, store AuthStore) (string, error)
}

// AuthDecorator decorates outgoing gRPC requests with authentication metadata.
type AuthDecorator interface {
	Decorate(ctx context.Context, store AuthStore) (map[string]string, error)
}
