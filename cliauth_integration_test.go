package protocli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	protocli "github.com/drewfead/proto-cli"
	"github.com/drewfead/proto-cli/cliauth"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// --- mock helpers ---

// mockStore is an in-memory AuthStore for testing.
type mockStore struct {
	token   []byte
	deleted bool
}

func (s *mockStore) Save(_ context.Context, token []byte) error {
	s.token = token
	s.deleted = false
	return nil
}

func (s *mockStore) Load(_ context.Context) ([]byte, error) {
	if s.deleted || s.token == nil {
		return nil, cliauth.ErrNotFound
	}
	return s.token, nil
}

func (s *mockStore) Delete(_ context.Context) error {
	if s.deleted || s.token == nil {
		return cliauth.ErrNotFound
	}
	s.token = nil
	s.deleted = true
	return nil
}

// mockLoginProvider implements LoginProvider only (minimal).
type mockLoginProvider struct {
	loginCalled bool
}

func (p *mockLoginProvider) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "token", Usage: "auth token"},
	}
}

func (p *mockLoginProvider) Login(ctx context.Context, cmd *cli.Command, store cliauth.AuthStore) error {
	p.loginCalled = true
	return store.Save(ctx, []byte(cmd.String("token")))
}

// mockFullProvider implements LoginProvider, InteractiveLoginProvider, LogoutProvider, and StatusProvider.
type mockFullProvider struct {
	loginCalled       bool
	interactiveCalled bool
	logoutCalled      bool
	statusCalled      bool
}

func (p *mockFullProvider) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "token", Usage: "auth token"},
	}
}

func (p *mockFullProvider) Login(ctx context.Context, cmd *cli.Command, store cliauth.AuthStore) error {
	p.loginCalled = true
	return store.Save(ctx, []byte(cmd.String("token")))
}

func (p *mockFullProvider) LoginInteractive(_ context.Context, _ io.Reader, out io.Writer, store cliauth.AuthStore) error {
	p.interactiveCalled = true
	_ = store.Save(context.Background(), []byte("interactive-token"))
	_, _ = io.WriteString(out, "Logged in interactively.\n")
	return nil
}

func (p *mockFullProvider) Logout(ctx context.Context, store cliauth.AuthStore) error {
	p.logoutCalled = true
	return store.Delete(ctx)
}

func (p *mockFullProvider) Status(ctx context.Context, store cliauth.AuthStore) (string, error) {
	p.statusCalled = true
	token, err := store.Load(ctx)
	if err != nil {
		if errors.Is(err, cliauth.ErrNotFound) {
			return "", nil
		}
		return "", err
	}
	return "Authenticated with token: " + string(token), nil
}

// mockDecorator implements AuthDecorator for testing.
type mockDecorator struct {
	decorateCalled bool
}

func (d *mockDecorator) Decorate(ctx context.Context, store cliauth.AuthStore) (map[string]string, error) {
	d.decorateCalled = true
	token, err := store.Load(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"authorization": "Bearer " + string(token),
	}, nil
}

// --- helpers ---

func dummyServiceCLI() *protocli.ServiceCLI {
	return &protocli.ServiceCLI{
		Command: &cli.Command{
			Name:  "svc",
			Usage: "dummy service",
		},
		ServiceName:   "svc",
		FactoryOrImpl: nil,
		RegisterFunc:  func(s *grpc.Server, impl any) {},
	}
}

// --- tests ---

func TestIntegration_Auth_LoginWithFlags(t *testing.T) {
	store := &mockStore{}
	provider := &mockLoginProvider{}

	rootCmd, err := protocli.RootCommand("testapp",
		protocli.Service(dummyServiceCLI()),
		protocli.WithAuth(provider, cliauth.WithStore(store)),
	)
	require.NoError(t, err)

	var buf bytes.Buffer
	rootCmd.Writer = &buf
	setWriterRecursive(rootCmd, &buf)

	err = rootCmd.Run(context.Background(), []string{"testapp", "auth", "login", "--token", "abc123"})
	require.NoError(t, err)
	require.True(t, provider.loginCalled)
	require.Equal(t, []byte("abc123"), store.token)
}

func TestIntegration_Auth_LoginInteractive_AutoTrigger(t *testing.T) {
	store := &mockStore{}
	provider := &mockFullProvider{}

	rootCmd, err := protocli.RootCommand("testapp",
		protocli.Service(dummyServiceCLI()),
		protocli.WithAuth(provider, cliauth.WithStore(store)),
	)
	require.NoError(t, err)

	var buf bytes.Buffer
	rootCmd.Writer = &buf
	setWriterRecursive(rootCmd, &buf)

	// No flags → should auto-trigger interactive
	err = rootCmd.Run(context.Background(), []string{"testapp", "auth", "login"})
	require.NoError(t, err)
	require.True(t, provider.interactiveCalled)
	require.False(t, provider.loginCalled)
	require.Equal(t, []byte("interactive-token"), store.token)
}

func TestIntegration_Auth_LoginInteractive_FlagsWin(t *testing.T) {
	store := &mockStore{}
	provider := &mockFullProvider{}

	rootCmd, err := protocli.RootCommand("testapp",
		protocli.Service(dummyServiceCLI()),
		protocli.WithAuth(provider, cliauth.WithStore(store)),
	)
	require.NoError(t, err)

	var buf bytes.Buffer
	rootCmd.Writer = &buf
	setWriterRecursive(rootCmd, &buf)

	// Provider flag set → Login() not LoginInteractive()
	err = rootCmd.Run(context.Background(), []string{"testapp", "auth", "login", "--token", "flag-token"})
	require.NoError(t, err)
	require.True(t, provider.loginCalled)
	require.False(t, provider.interactiveCalled)
	require.Equal(t, []byte("flag-token"), store.token)
}

func TestIntegration_Auth_LoginInteractive_ExplicitFlag(t *testing.T) {
	store := &mockStore{}
	provider := &mockFullProvider{}

	rootCmd, err := protocli.RootCommand("testapp",
		protocli.Service(dummyServiceCLI()),
		protocli.WithAuth(provider, cliauth.WithStore(store)),
	)
	require.NoError(t, err)

	var buf bytes.Buffer
	rootCmd.Writer = &buf
	setWriterRecursive(rootCmd, &buf)

	// --interactive explicit → LoginInteractive() even with other flags
	err = rootCmd.Run(context.Background(), []string{"testapp", "auth", "login", "--interactive", "--token", "ignored"})
	require.NoError(t, err)
	require.True(t, provider.interactiveCalled)
	require.False(t, provider.loginCalled)
}

func TestIntegration_Auth_FullProvider_LogoutAndStatus(t *testing.T) {
	store := &mockStore{}
	provider := &mockFullProvider{}

	rootCmd, err := protocli.RootCommand("testapp",
		protocli.Service(dummyServiceCLI()),
		protocli.WithAuth(provider, cliauth.WithStore(store)),
	)
	require.NoError(t, err)

	var buf bytes.Buffer
	rootCmd.Writer = &buf
	setWriterRecursive(rootCmd, &buf)

	// Login first
	err = rootCmd.Run(context.Background(), []string{"testapp", "auth", "login", "--token", "mytoken"})
	require.NoError(t, err)

	// Status
	buf.Reset()
	err = rootCmd.Run(context.Background(), []string{"testapp", "auth", "status"})
	require.NoError(t, err)
	require.True(t, provider.statusCalled)
	require.Contains(t, buf.String(), "Authenticated with token: mytoken")

	// Logout
	buf.Reset()
	err = rootCmd.Run(context.Background(), []string{"testapp", "auth", "logout"})
	require.NoError(t, err)
	require.True(t, provider.logoutCalled)
	require.Contains(t, buf.String(), "Logged out successfully.")

	// Status after logout
	buf.Reset()
	provider.statusCalled = false
	err = rootCmd.Run(context.Background(), []string{"testapp", "auth", "status"})
	require.NoError(t, err)
	require.True(t, provider.statusCalled)
	require.Contains(t, buf.String(), "Not authenticated.")
}

func TestIntegration_Auth_MinimalProvider_OnlyLogin(t *testing.T) {
	store := &mockStore{}
	provider := &mockLoginProvider{}

	rootCmd, err := protocli.RootCommand("testapp",
		protocli.Service(dummyServiceCLI()),
		protocli.WithAuth(provider, cliauth.WithStore(store)),
	)
	require.NoError(t, err)

	// Verify only login subcommand is present
	var authCmd *cli.Command
	for _, cmd := range rootCmd.Commands {
		if cmd.Name == "auth" {
			authCmd = cmd
			break
		}
	}
	require.NotNil(t, authCmd)

	subNames := make(map[string]bool)
	for _, sub := range authCmd.Commands {
		subNames[sub.Name] = true
	}
	require.True(t, subNames["login"])
	require.False(t, subNames["logout"])
	require.False(t, subNames["status"])
}

func TestIntegration_Auth_CollisionDetection(t *testing.T) {
	// Create a service whose command name is "auth" to trigger collision
	svc := &protocli.ServiceCLI{
		Command: &cli.Command{
			Name:  "auth",
			Usage: "colliding command",
		},
		ServiceName:   "auth",
		FactoryOrImpl: nil,
		RegisterFunc:  func(s *grpc.Server, impl any) {},
	}

	provider := &mockLoginProvider{}
	store := &mockStore{}

	_, err := protocli.RootCommand("testapp",
		protocli.Service(svc),
		protocli.WithAuth(provider, cliauth.WithStore(store)),
	)
	require.Error(t, err)
	require.ErrorIs(t, err, protocli.ErrAmbiguousCommandInvocation)
}

func TestIntegration_Auth_WithoutWithAuth_NoAuthCommands(t *testing.T) {
	rootCmd, err := protocli.RootCommand("testapp",
		protocli.Service(dummyServiceCLI()),
	)
	require.NoError(t, err)

	for _, cmd := range rootCmd.Commands {
		require.NotEqual(t, "auth", cmd.Name)
	}
}

func TestIntegration_Auth_StoreSaveLoadRoundTrip(t *testing.T) {
	store := &mockStore{}
	ctx := context.Background()

	err := store.Save(ctx, []byte("secret"))
	require.NoError(t, err)

	token, err := store.Load(ctx)
	require.NoError(t, err)
	require.Equal(t, []byte("secret"), token)

	err = store.Delete(ctx)
	require.NoError(t, err)

	_, err = store.Load(ctx)
	require.ErrorIs(t, err, cliauth.ErrNotFound)
}

func TestIntegration_Auth_Decorator_ContextMetadata(t *testing.T) {
	store := &mockStore{}
	ctx := context.Background()
	_ = store.Save(ctx, []byte("dec-token"))

	decorator := &mockDecorator{}
	cfg := cliauth.NewConfig("testapp", &mockLoginProvider{},
		cliauth.WithStore(store),
		cliauth.WithDecorator(decorator),
	)

	newCtx := cliauth.DecorateContext(ctx, cfg)
	require.True(t, decorator.decorateCalled)

	// Verify metadata was attached
	md, ok := metadata.FromOutgoingContext(newCtx)
	require.True(t, ok)
	require.Equal(t, []string{"Bearer dec-token"}, md.Get("authorization"))
}

func TestIntegration_Auth_Decorator_NoToken_OriginalContext(t *testing.T) {
	store := &mockStore{} // empty store
	decorator := &mockDecorator{}
	cfg := cliauth.NewConfig("testapp", &mockLoginProvider{},
		cliauth.WithStore(store),
		cliauth.WithDecorator(decorator),
	)

	ctx := context.Background()
	newCtx := cliauth.DecorateContext(ctx, cfg)

	// Should return original context since decorator errors (ErrNotFound)
	_, ok := metadata.FromOutgoingContext(newCtx)
	require.False(t, ok)
}

func TestIntegration_Auth_NoDecorator_OriginalContext(t *testing.T) {
	store := &mockStore{}
	cfg := cliauth.NewConfig("testapp", &mockLoginProvider{},
		cliauth.WithStore(store),
	)

	ctx := context.Background()
	newCtx := cliauth.DecorateContext(ctx, cfg)

	// No decorator → original context returned
	_, ok := metadata.FromOutgoingContext(newCtx)
	require.False(t, ok)
}
