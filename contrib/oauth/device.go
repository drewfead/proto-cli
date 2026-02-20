package oauth

import (
	"context"
	"fmt"
	"io"

	"github.com/drewfead/proto-cli/cliauth"
	"golang.org/x/oauth2"
)

// LoginInteractive performs a Device Code flow, printing verification instructions to out.
func (p *Provider) LoginInteractive(ctx context.Context, _ io.Reader, out io.Writer, store cliauth.AuthStore) error {
	oauthCtx := p.httpContext(ctx)

	cfg := *p.oauth2Config
	cfg.Endpoint.DeviceAuthURL = p.deviceAuthURL

	var authOpts []oauth2.AuthCodeOption
	if p.audience != "" {
		authOpts = append(authOpts, oauth2.SetAuthURLParam("audience", p.audience))
	}

	da, err := cfg.DeviceAuth(oauthCtx, authOpts...)
	if err != nil {
		return fmt.Errorf("device authorization failed: %w", err)
	}

	_, _ = fmt.Fprintf(out, "Open this URL in your browser: %s\n", da.VerificationURI)
	_, _ = fmt.Fprintf(out, "Enter code: %s\n", da.UserCode)
	if da.VerificationURIComplete != "" {
		_, _ = fmt.Fprintf(out, "Or open: %s\n", da.VerificationURIComplete)
	}

	tok, err := cfg.DeviceAccessToken(oauthCtx, da)
	if err != nil {
		return fmt.Errorf("device token exchange failed: %w", err)
	}

	return p.saveToken(ctx, store, tok)
}
