package oauth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/drewfead/proto-cli/cliauth"
	"golang.org/x/oauth2"
)

// Decorate loads the stored token, refreshes it if needed, and returns authorization metadata.
func (p *Provider) Decorate(ctx context.Context, store cliauth.AuthStore) (map[string]string, error) {
	st, err := loadToken(ctx, store)
	if err != nil {
		if errors.Is(err, cliauth.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}

	tok := st.oauth2Token()

	if p.tokenNeedsRefresh(tok) {
		refreshed, refreshErr := p.refreshToken(ctx, store, tok, st.Scopes)
		if refreshErr != nil {
			if tok.Valid() {
				// Graceful degradation: use current token if still technically valid.
			} else {
				return nil, fmt.Errorf("token expired and refresh failed: %w", refreshErr)
			}
		} else {
			tok = refreshed
		}
	}

	tokenType := tok.TokenType
	if tokenType == "" {
		tokenType = "Bearer"
	}

	return map[string]string{
		"authorization": tokenType + " " + tok.AccessToken,
	}, nil
}

// tokenNeedsRefresh returns true if the token is expired or within the expiry buffer.
func (p *Provider) tokenNeedsRefresh(tok *oauth2.Token) bool {
	if tok.Expiry.IsZero() {
		return false
	}
	return time.Until(tok.Expiry) < p.expiryBuffer
}

// refreshToken uses the oauth2 token source to refresh the token and saves it back.
func (p *Provider) refreshToken(ctx context.Context, store cliauth.AuthStore, tok *oauth2.Token, scopes []string) (*oauth2.Token, error) {
	if tok.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token available")
	}

	oauthCtx := p.httpContext(ctx)

	// Force the oauth2 library to refresh by setting expiry to the past.
	// oauth2.TokenSource has its own internal expiry delta and would skip
	// the refresh if the token is still technically valid by its standards.
	expired := *tok
	expired.Expiry = time.Now().Add(-1 * time.Minute)
	src := p.oauth2Config.TokenSource(oauthCtx, &expired)

	newTok, err := src.Token()
	if err != nil {
		return nil, err
	}

	// Preserve the old refresh token if the server didn't return a new one.
	if newTok.RefreshToken == "" {
		newTok.RefreshToken = tok.RefreshToken
	}

	if err := p.saveToken(ctx, store, newTok); err != nil {
		return nil, err
	}

	return newTok, nil
}
