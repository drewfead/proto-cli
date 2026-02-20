package oauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"

	"github.com/drewfead/proto-cli/cliauth"
	"github.com/urfave/cli/v3"
	"golang.org/x/oauth2"
)

// Flags returns CLI flags that trigger the PKCE login flow.
func (p *Provider) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:  "client-id",
			Usage: "OAuth client ID (overrides configured value)",
		},
		&cli.StringFlag{
			Name:  "issuer",
			Usage: "OAuth issuer URL (overrides configured endpoints)",
		},
	}
}

// Login performs an Authorization Code with PKCE flow.
func (p *Provider) Login(ctx context.Context, cmd *cli.Command, store cliauth.AuthStore) error {
	cfg := p.effectiveConfig(cmd)
	oauthCtx := p.httpContext(ctx)

	verifier := oauth2.GenerateVerifier()

	listener, err := net.Listen("tcp", net.JoinHostPort(p.loopbackHost, "0"))
	if err != nil {
		return fmt.Errorf("failed to start callback server: %w", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	cfg.RedirectURL = fmt.Sprintf("http://%s:%d/callback", p.loopbackHost, port)

	state, err := randomState()
	if err != nil {
		return fmt.Errorf("failed to generate state: %w", err)
	}

	authOpts := []oauth2.AuthCodeOption{oauth2.S256ChallengeOption(verifier)}
	if p.audience != "" {
		authOpts = append(authOpts, oauth2.SetAuthURLParam("audience", p.audience))
	}
	authURL := cfg.AuthCodeURL(state, authOpts...)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			errCh <- fmt.Errorf("state mismatch")
			http.Error(w, "State mismatch", http.StatusBadRequest)
			return
		}
		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			errCh <- fmt.Errorf("authorization error: %s", errMsg)
			http.Error(w, errMsg, http.StatusBadRequest)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no authorization code received")
			http.Error(w, "No code", http.StatusBadRequest)
			return
		}
		_, _ = fmt.Fprint(w, "<html><body><h1>Login successful!</h1><p>You may close this window.</p></body></html>")
		codeCh <- code
	})

	server := &http.Server{Handler: mux}
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			errCh <- serveErr
		}
	}()
	defer server.Close()

	if err := p.browserOpen(authURL); err != nil {
		_, _ = fmt.Fprintf(cmd.Writer, "Open this URL in your browser:\n%s\n", authURL)
	}

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}

	exchangeOpts := []oauth2.AuthCodeOption{oauth2.VerifierOption(verifier)}
	tok, err := cfg.Exchange(oauthCtx, code, exchangeOpts...)
	if err != nil {
		return fmt.Errorf("token exchange failed: %w", err)
	}

	return p.saveToken(ctx, store, tok)
}

// effectiveConfig returns an oauth2.Config with any flag overrides applied.
func (p *Provider) effectiveConfig(cmd *cli.Command) *oauth2.Config {
	cfg := *p.oauth2Config
	if cmd.IsSet("client-id") {
		cfg.ClientID = cmd.String("client-id")
	}
	return &cfg
}

// randomState generates a cryptographically random state parameter.
func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
