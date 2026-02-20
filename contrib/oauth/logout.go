package oauth

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/drewfead/proto-cli/cliauth"
)

// Logout revokes the stored token (if a revocation URL is configured) and deletes it from the store.
func (p *Provider) Logout(ctx context.Context, store cliauth.AuthStore) error {
	if p.revocationURL != "" {
		p.revokeToken(ctx, store)
	}
	return store.Delete(ctx)
}

// revokeToken attempts RFC 7009 token revocation. Errors are non-fatal.
func (p *Provider) revokeToken(ctx context.Context, store cliauth.AuthStore) {
	data, err := store.Load(ctx)
	if err != nil {
		return
	}

	st, err := unmarshalToken(data)
	if err != nil {
		return
	}

	// Prefer revoking the refresh token; fall back to access token.
	token := st.RefreshToken
	tokenHint := "refresh_token"
	if token == "" {
		token = st.AccessToken
		tokenHint = "access_token"
	}

	if token == "" {
		return
	}

	form := url.Values{
		"token":           {token},
		"token_type_hint": {tokenHint},
		"client_id":       {p.clientID},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.revocationURL, strings.NewReader(form.Encode()))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := p.httpClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

// loadToken is a helper that loads and unmarshals a token from the store.
func loadToken(ctx context.Context, store cliauth.AuthStore) (*storedToken, error) {
	data, err := store.Load(ctx)
	if err != nil {
		return nil, err
	}
	st, err := unmarshalToken(data)
	if err != nil {
		return nil, errors.New("stored credentials are corrupted")
	}
	return st, nil
}
