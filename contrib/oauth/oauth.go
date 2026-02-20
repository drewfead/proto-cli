// Package oauth provides an out-of-the-box OAuth authentication provider for proto-cli.
//
// It supports both Device Code (interactive) and Authorization Code with PKCE (flag-based)
// flows, stores tokens as JSON with automatic refresh, and implements all cliauth interfaces.
//
//	provider := oauth.NewProvider(
//	    oauth.WithClientID("my-client-id"),
//	    oauth.WithEndpoints("https://auth.example.com/authorize", "https://auth.example.com/token"),
//	    oauth.WithDeviceAuthURL("https://auth.example.com/device/code"),
//	    oauth.WithScopes("openid", "profile", "email"),
//	)
package oauth

import (
	"context"
	"net/http"
	"time"

	"github.com/cli/browser"
	"github.com/drewfead/proto-cli/cliauth"
	"golang.org/x/oauth2"
)

var (
	_ cliauth.InteractiveLoginProvider = (*Provider)(nil)
	_ cliauth.LogoutProvider           = (*Provider)(nil)
	_ cliauth.StatusProvider           = (*Provider)(nil)
	_ cliauth.AuthDecorator            = (*Provider)(nil)
)

// Provider implements cliauth.InteractiveLoginProvider, cliauth.LogoutProvider,
// cliauth.StatusProvider, and cliauth.AuthDecorator using OAuth 2.0 flows.
type Provider struct {
	clientID       string
	clientSecret   string
	scopes         []string
	oauth2Config   *oauth2.Config
	deviceAuthURL  string
	revocationURL  string
	expiryBuffer   time.Duration
	httpClient     *http.Client
	onLogin        func(token *oauth2.Token)
	loopbackHost   string
	audience       string
	browserOpen    func(url string) error
}

// ProviderOption configures a Provider.
type ProviderOption func(*Provider)

// WithClientID sets the OAuth client ID.
func WithClientID(id string) ProviderOption {
	return func(p *Provider) { p.clientID = id }
}

// WithClientSecret sets the OAuth client secret for confidential clients.
func WithClientSecret(secret string) ProviderOption {
	return func(p *Provider) { p.clientSecret = secret }
}

// WithScopes sets the OAuth scopes to request.
func WithScopes(scopes ...string) ProviderOption {
	return func(p *Provider) { p.scopes = scopes }
}

// WithEndpoints sets the authorization and token endpoint URLs.
func WithEndpoints(authURL, tokenURL string) ProviderOption {
	return func(p *Provider) {
		p.oauth2Config.Endpoint = oauth2.Endpoint{
			AuthURL:  authURL,
			TokenURL: tokenURL,
		}
	}
}

// WithDeviceAuthURL sets the device authorization endpoint URL.
func WithDeviceAuthURL(url string) ProviderOption {
	return func(p *Provider) { p.deviceAuthURL = url }
}

// WithRevocationURL sets the token revocation endpoint URL (RFC 7009).
func WithRevocationURL(url string) ProviderOption {
	return func(p *Provider) { p.revocationURL = url }
}

// WithExpiryBuffer sets how far before token expiry a refresh should be triggered.
func WithExpiryBuffer(d time.Duration) ProviderOption {
	return func(p *Provider) { p.expiryBuffer = d }
}

// WithHTTPClient sets a custom HTTP client (useful for testing or proxies).
func WithHTTPClient(client *http.Client) ProviderOption {
	return func(p *Provider) { p.httpClient = client }
}

// WithLoginCallback sets a function called after a successful login with the new token.
func WithLoginCallback(fn func(token *oauth2.Token)) ProviderOption {
	return func(p *Provider) { p.onLogin = fn }
}

// WithLoopbackHost sets the host for the PKCE loopback redirect server.
func WithLoopbackHost(host string) ProviderOption {
	return func(p *Provider) { p.loopbackHost = host }
}

// WithAudience sets the audience parameter sent with authorization requests.
func WithAudience(aud string) ProviderOption {
	return func(p *Provider) { p.audience = aud }
}

// WithBrowserOpen sets the function used to open a URL in the user's browser.
// Useful for testing or environments without a browser.
func WithBrowserOpen(fn func(url string) error) ProviderOption {
	return func(p *Provider) { p.browserOpen = fn }
}

// NewProvider creates a Provider configured with the given options.
func NewProvider(opts ...ProviderOption) *Provider {
	p := &Provider{
		oauth2Config: &oauth2.Config{},
		expiryBuffer: 30 * time.Second,
		httpClient:   http.DefaultClient,
		loopbackHost: "127.0.0.1",
		browserOpen:  browser.OpenURL,
	}
	for _, opt := range opts {
		opt(p)
	}
	p.oauth2Config.ClientID = p.clientID
	p.oauth2Config.ClientSecret = p.clientSecret
	p.oauth2Config.Scopes = p.scopes
	return p
}

// httpContext returns a context with the provider's HTTP client injected for oauth2 use.
func (p *Provider) httpContext(ctx context.Context) context.Context {
	if p.httpClient == nil || p.httpClient == http.DefaultClient {
		return ctx
	}
	return context.WithValue(ctx, oauth2.HTTPClient, p.httpClient)
}

// saveToken marshals a token and persists it to the store.
func (p *Provider) saveToken(ctx context.Context, store cliauth.AuthStore, tok *oauth2.Token) error {
	data, err := marshalToken(tok, p.scopes)
	if err != nil {
		return err
	}
	if err := store.Save(ctx, data); err != nil {
		return err
	}
	if p.onLogin != nil {
		p.onLogin(tok)
	}
	return nil
}
