package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/drewfead/proto-cli/cliauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
	"golang.org/x/oauth2"
)

// --- mock store ---

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

// --- token tests ---

func TestMarshalUnmarshalRoundTrip(t *testing.T) {
	tok := &oauth2.Token{
		AccessToken:  "access-123",
		TokenType:    "Bearer",
		RefreshToken: "refresh-456",
		Expiry:       time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
	}
	scopes := []string{"openid", "profile"}

	data, err := marshalToken(tok, scopes)
	require.NoError(t, err)

	st, err := unmarshalToken(data)
	require.NoError(t, err)

	assert.Equal(t, "access-123", st.AccessToken)
	assert.Equal(t, "Bearer", st.TokenType)
	assert.Equal(t, "refresh-456", st.RefreshToken)
	assert.Equal(t, tok.Expiry, st.Expiry)
	assert.Equal(t, scopes, st.Scopes)
}

func TestMarshalOmitsEmptyRefresh(t *testing.T) {
	tok := &oauth2.Token{
		AccessToken: "access-123",
		TokenType:   "Bearer",
	}

	data, err := marshalToken(tok, nil)
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	_, hasRefresh := raw["refresh_token"]
	assert.False(t, hasRefresh, "refresh_token should be omitted when empty")
}

func TestUnmarshalInvalidJSON(t *testing.T) {
	_, err := unmarshalToken([]byte("not json"))
	assert.Error(t, err)
}

func TestStoredTokenToOAuth2(t *testing.T) {
	st := &storedToken{
		AccessToken:  "at",
		TokenType:    "Bearer",
		RefreshToken: "rt",
		Expiry:       time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	tok := st.oauth2Token()
	assert.Equal(t, "at", tok.AccessToken)
	assert.Equal(t, "Bearer", tok.TokenType)
	assert.Equal(t, "rt", tok.RefreshToken)
	assert.Equal(t, st.Expiry, tok.Expiry)
}

// --- status tests ---

func TestStatus_NotAuthenticated(t *testing.T) {
	p := NewProvider()
	store := &mockStore{}

	msg, err := p.Status(context.Background(), store)
	require.NoError(t, err)
	assert.Equal(t, "", msg)
}

func TestStatus_ValidToken(t *testing.T) {
	p := NewProvider()
	store := &mockStore{}

	tok := &oauth2.Token{
		AccessToken:  "at",
		TokenType:    "Bearer",
		RefreshToken: "rt",
		Expiry:       time.Now().Add(1 * time.Hour),
	}
	data, err := marshalToken(tok, []string{"openid", "profile"})
	require.NoError(t, err)
	require.NoError(t, store.Save(context.Background(), data))

	msg, err := p.Status(context.Background(), store)
	require.NoError(t, err)
	assert.Contains(t, msg, "Token type: Bearer")
	assert.Contains(t, msg, "Scopes: openid, profile")
	assert.Contains(t, msg, "Expires in:")
	assert.Contains(t, msg, "Refresh token: Available")
}

func TestStatus_ExpiredToken(t *testing.T) {
	p := NewProvider()
	store := &mockStore{}

	tok := &oauth2.Token{
		AccessToken: "at",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(-1 * time.Hour),
	}
	data, err := marshalToken(tok, nil)
	require.NoError(t, err)
	require.NoError(t, store.Save(context.Background(), data))

	msg, err := p.Status(context.Background(), store)
	require.NoError(t, err)
	assert.Contains(t, msg, "Status: Expired")
}

func TestStatus_CorruptedData(t *testing.T) {
	p := NewProvider()
	store := &mockStore{}
	require.NoError(t, store.Save(context.Background(), []byte("not-json")))

	msg, err := p.Status(context.Background(), store)
	require.NoError(t, err)
	assert.Equal(t, "Stored credentials are corrupted.", msg)
}

func TestStatus_NoExpiry(t *testing.T) {
	p := NewProvider()
	store := &mockStore{}

	tok := &oauth2.Token{
		AccessToken: "at",
		TokenType:   "Bearer",
	}
	data, err := marshalToken(tok, nil)
	require.NoError(t, err)
	require.NoError(t, store.Save(context.Background(), data))

	msg, err := p.Status(context.Background(), store)
	require.NoError(t, err)
	assert.Contains(t, msg, "Token type: Bearer")
	assert.NotContains(t, msg, "Expires")
	assert.NotContains(t, msg, "Expired")
}

// --- logout tests ---

func TestLogout_DeletesToken(t *testing.T) {
	p := NewProvider()
	store := &mockStore{}
	require.NoError(t, store.Save(context.Background(), []byte(`{"access_token":"at"}`)))

	err := p.Logout(context.Background(), store)
	require.NoError(t, err)
	assert.True(t, store.deleted)
}

func TestLogout_RevocationRequestSent(t *testing.T) {
	var receivedToken, receivedHint string
	revokeSrv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		receivedToken = r.FormValue("token")
		receivedHint = r.FormValue("token_type_hint")
	}))
	defer revokeSrv.Close()

	p := NewProvider(
		WithClientID("test-client"),
		WithRevocationURL(revokeSrv.URL),
		WithHTTPClient(revokeSrv.Client()),
	)
	store := &mockStore{}

	tok := &oauth2.Token{
		AccessToken:  "at",
		TokenType:    "Bearer",
		RefreshToken: "rt",
	}
	data, err := marshalToken(tok, nil)
	require.NoError(t, err)
	require.NoError(t, store.Save(context.Background(), data))

	require.NoError(t, p.Logout(context.Background(), store))

	assert.Equal(t, "rt", receivedToken, "should revoke refresh token preferentially")
	assert.Equal(t, "refresh_token", receivedHint)
}

func TestLogout_RevocationFailureStillDeletes(t *testing.T) {
	revokeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer revokeSrv.Close()

	p := NewProvider(
		WithClientID("test-client"),
		WithRevocationURL(revokeSrv.URL),
		WithHTTPClient(revokeSrv.Client()),
	)
	store := &mockStore{}
	require.NoError(t, store.Save(context.Background(), []byte(`{"access_token":"at"}`)))

	err := p.Logout(context.Background(), store)
	require.NoError(t, err)
	assert.True(t, store.deleted)
}

func TestLogout_EmptyStore(t *testing.T) {
	p := NewProvider()
	store := &mockStore{}

	err := p.Logout(context.Background(), store)
	require.ErrorIs(t, err, cliauth.ErrNotFound)
}

// --- decorator tests ---

func TestDecorate_ValidToken(t *testing.T) {
	p := NewProvider()
	store := &mockStore{}

	tok := &oauth2.Token{
		AccessToken: "my-token",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(1 * time.Hour),
	}
	data, err := marshalToken(tok, nil)
	require.NoError(t, err)
	require.NoError(t, store.Save(context.Background(), data))

	md, err := p.Decorate(context.Background(), store)
	require.NoError(t, err)
	assert.Equal(t, "Bearer my-token", md["authorization"])
}

func TestDecorate_NoToken(t *testing.T) {
	p := NewProvider()
	store := &mockStore{}

	md, err := p.Decorate(context.Background(), store)
	require.NoError(t, err)
	assert.Nil(t, md)
}

func TestDecorate_DefaultTokenType(t *testing.T) {
	p := NewProvider()
	store := &mockStore{}

	tok := &oauth2.Token{
		AccessToken: "my-token",
		Expiry:      time.Now().Add(1 * time.Hour),
	}
	data, err := marshalToken(tok, nil)
	require.NoError(t, err)
	require.NoError(t, store.Save(context.Background(), data))

	md, err := p.Decorate(context.Background(), store)
	require.NoError(t, err)
	assert.Equal(t, "Bearer my-token", md["authorization"])
}

func TestDecorate_ExpiredWithRefresh(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"access_token":"new-at","token_type":"Bearer","refresh_token":"new-rt","expires_in":3600}`)
	}))
	defer tokenSrv.Close()

	p := NewProvider(
		WithClientID("test-client"),
		WithEndpoints("https://unused.example.com/auth", tokenSrv.URL),
		WithHTTPClient(tokenSrv.Client()),
	)
	store := &mockStore{}

	tok := &oauth2.Token{
		AccessToken:  "old-at",
		TokenType:    "Bearer",
		RefreshToken: "old-rt",
		Expiry:       time.Now().Add(-1 * time.Hour),
	}
	data, err := marshalToken(tok, []string{"openid"})
	require.NoError(t, err)
	require.NoError(t, store.Save(context.Background(), data))

	md, err := p.Decorate(context.Background(), store)
	require.NoError(t, err)
	assert.Equal(t, "Bearer new-at", md["authorization"])

	// Verify new token was saved
	st, err := unmarshalToken(store.token)
	require.NoError(t, err)
	assert.Equal(t, "new-at", st.AccessToken)
	assert.Equal(t, "new-rt", st.RefreshToken)
}

func TestDecorate_ExpiredNoRefresh(t *testing.T) {
	p := NewProvider()
	store := &mockStore{}

	tok := &oauth2.Token{
		AccessToken: "old-at",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(-1 * time.Hour),
	}
	data, err := marshalToken(tok, nil)
	require.NoError(t, err)
	require.NoError(t, store.Save(context.Background(), data))

	_, err = p.Decorate(context.Background(), store)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token expired and refresh failed")
}

func TestDecorate_WithinBuffer(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"access_token":"refreshed","token_type":"Bearer","expires_in":3600}`)
	}))
	defer tokenSrv.Close()

	p := NewProvider(
		WithClientID("test-client"),
		WithEndpoints("https://unused.example.com/auth", tokenSrv.URL),
		WithHTTPClient(tokenSrv.Client()),
		WithExpiryBuffer(5*time.Minute),
	)
	store := &mockStore{}

	// Token expires in 2 minutes, but buffer is 5 minutes → should refresh
	tok := &oauth2.Token{
		AccessToken:  "almost-expired",
		TokenType:    "Bearer",
		RefreshToken: "rt",
		Expiry:       time.Now().Add(2 * time.Minute),
	}
	data, err := marshalToken(tok, nil)
	require.NoError(t, err)
	require.NoError(t, store.Save(context.Background(), data))

	md, err := p.Decorate(context.Background(), store)
	require.NoError(t, err)
	assert.Equal(t, "Bearer refreshed", md["authorization"])
}

func TestDecorate_RefreshFailsButStillValid(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer tokenSrv.Close()

	p := NewProvider(
		WithClientID("test-client"),
		WithEndpoints("https://unused.example.com/auth", tokenSrv.URL),
		WithHTTPClient(tokenSrv.Client()),
		WithExpiryBuffer(5*time.Minute),
	)
	store := &mockStore{}

	// Token still valid (expires in 2 min) but within buffer
	tok := &oauth2.Token{
		AccessToken:  "still-valid",
		TokenType:    "Bearer",
		RefreshToken: "rt",
		Expiry:       time.Now().Add(2 * time.Minute),
	}
	data, err := marshalToken(tok, nil)
	require.NoError(t, err)
	require.NoError(t, store.Save(context.Background(), data))

	md, err := p.Decorate(context.Background(), store)
	require.NoError(t, err)
	assert.Equal(t, "Bearer still-valid", md["authorization"], "should fall back to current token")
}

func TestDecorate_PreservesOldRefreshToken(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Server does not return a new refresh token
		_, _ = fmt.Fprintf(w, `{"access_token":"new-at","token_type":"Bearer","expires_in":3600}`)
	}))
	defer tokenSrv.Close()

	p := NewProvider(
		WithClientID("test-client"),
		WithEndpoints("https://unused.example.com/auth", tokenSrv.URL),
		WithHTTPClient(tokenSrv.Client()),
	)
	store := &mockStore{}

	tok := &oauth2.Token{
		AccessToken:  "old-at",
		TokenType:    "Bearer",
		RefreshToken: "keep-this-rt",
		Expiry:       time.Now().Add(-1 * time.Hour),
	}
	data, err := marshalToken(tok, nil)
	require.NoError(t, err)
	require.NoError(t, store.Save(context.Background(), data))

	_, err = p.Decorate(context.Background(), store)
	require.NoError(t, err)

	st, err := unmarshalToken(store.token)
	require.NoError(t, err)
	assert.Equal(t, "keep-this-rt", st.RefreshToken, "should preserve old refresh token")
}

// --- login tests ---

func TestFlags(t *testing.T) {
	p := NewProvider()
	flags := p.Flags()

	names := make(map[string]bool)
	for _, f := range flags {
		for _, n := range f.Names() {
			names[n] = true
		}
	}
	assert.True(t, names["client-id"])
	assert.True(t, names["issuer"])
}


func TestRandomState(t *testing.T) {
	s1, err := randomState()
	require.NoError(t, err)
	assert.Len(t, s1, 32) // 16 bytes hex-encoded

	s2, err := randomState()
	require.NoError(t, err)
	assert.NotEqual(t, s1, s2, "states should be unique")
}

func TestEffectiveConfig_OverridesClientID(t *testing.T) {
	p := NewProvider(WithClientID("original"))

	app := &cli.Command{
		Name: "test",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "client-id"},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			cfg := p.effectiveConfig(cmd)
			assert.Equal(t, "overridden", cfg.ClientID)
			return nil
		},
	}
	err := app.Run(context.Background(), []string{"test", "--client-id", "overridden"})
	require.NoError(t, err)
}

// --- device tests ---

func TestDeviceCodeFlow(t *testing.T) {
	deviceSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "device") || r.URL.Path == "/" {
			_ = r.ParseForm()
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{
				"device_code": "dev-code",
				"user_code": "ABCD-1234",
				"verification_uri": "https://example.com/activate",
				"verification_uri_complete": "https://example.com/activate?user_code=ABCD-1234",
				"interval": 1,
				"expires_in": 300
			}`)
		}
	}))
	defer deviceSrv.Close()

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		if r.FormValue("device_code") == "dev-code" {
			_, _ = fmt.Fprintf(w, `{"access_token":"device-token","token_type":"Bearer","expires_in":3600}`)
		} else {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintf(w, `{"error":"invalid_request"}`)
		}
	}))
	defer tokenSrv.Close()

	p := NewProvider(
		WithClientID("test-client"),
		WithEndpoints("https://unused.example.com/auth", tokenSrv.URL),
		WithDeviceAuthURL(deviceSrv.URL),
		WithHTTPClient(deviceSrv.Client()),
		WithScopes("openid"),
	)

	store := &mockStore{}
	var out strings.Builder

	err := p.LoginInteractive(context.Background(), nil, &out, store)
	require.NoError(t, err)

	assert.Contains(t, out.String(), "ABCD-1234")
	assert.Contains(t, out.String(), "https://example.com/activate")

	st, err := unmarshalToken(store.token)
	require.NoError(t, err)
	assert.Equal(t, "device-token", st.AccessToken)
}

// --- option tests ---

func TestNewProvider_Defaults(t *testing.T) {
	p := NewProvider()
	assert.Equal(t, 30*time.Second, p.expiryBuffer)
	assert.Equal(t, "127.0.0.1", p.loopbackHost)
	assert.Equal(t, http.DefaultClient, p.httpClient)
}

func TestNewProvider_WithOptions(t *testing.T) {
	var loginCalled bool
	p := NewProvider(
		WithClientID("cid"),
		WithClientSecret("csec"),
		WithScopes("s1", "s2"),
		WithDeviceAuthURL("https://dev.example.com/device"),
		WithRevocationURL("https://dev.example.com/revoke"),
		WithExpiryBuffer(2*time.Minute),
		WithLoopbackHost("localhost"),
		WithAudience("https://api.example.com"),
		WithLoginCallback(func(_ *oauth2.Token) { loginCalled = true }),
	)

	assert.Equal(t, "cid", p.clientID)
	assert.Equal(t, "csec", p.clientSecret)
	assert.Equal(t, []string{"s1", "s2"}, p.scopes)
	assert.Equal(t, "https://dev.example.com/device", p.deviceAuthURL)
	assert.Equal(t, "https://dev.example.com/revoke", p.revocationURL)
	assert.Equal(t, 2*time.Minute, p.expiryBuffer)
	assert.Equal(t, "localhost", p.loopbackHost)
	assert.Equal(t, "https://api.example.com", p.audience)

	// Verify callback
	p.onLogin(&oauth2.Token{})
	assert.True(t, loginCalled)
}

// --- interface compliance tests ---

func TestProvider_ImplementsInterfaces(t *testing.T) {
	p := NewProvider()
	var _ cliauth.InteractiveLoginProvider = p
	var _ cliauth.LogoutProvider = p
	var _ cliauth.StatusProvider = p
	var _ cliauth.AuthDecorator = p
}

// --- saveToken callback test ---

func TestSaveToken_CallsOnLogin(t *testing.T) {
	var called bool
	p := NewProvider(WithLoginCallback(func(tok *oauth2.Token) {
		called = true
		assert.Equal(t, "at", tok.AccessToken)
	}))

	store := &mockStore{}
	tok := &oauth2.Token{AccessToken: "at", TokenType: "Bearer"}
	require.NoError(t, p.saveToken(context.Background(), store, tok))
	assert.True(t, called)
}

// --- httpContext test ---

func TestHTTPContext_CustomClient(t *testing.T) {
	custom := &http.Client{Timeout: 42 * time.Second}
	p := NewProvider(WithHTTPClient(custom))

	ctx := p.httpContext(context.Background())
	client, ok := ctx.Value(oauth2.HTTPClient).(*http.Client)
	require.True(t, ok)
	assert.Equal(t, custom, client)
}

func TestHTTPContext_DefaultClient(t *testing.T) {
	p := NewProvider()
	ctx := p.httpContext(context.Background())
	// Should not inject anything when using default client
	assert.Nil(t, ctx.Value(oauth2.HTTPClient))
}

// --- login flow integration test with simulated callback ---

func TestLogin_FullPKCEFlow(t *testing.T) {
	// This test simulates the full PKCE flow. WithBrowserOpen intercepts the
	// authorization URL, extracts redirect_uri and state, then hits the
	// loopback callback directly — mimicking what a real browser would do.

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"access_token":"pkce-at","token_type":"Bearer","expires_in":3600}`)
	}))
	defer tokenSrv.Close()

	// The fake "auth server" URL is embedded in the authURL that browserOpen receives.
	// We don't need a running auth server — we just parse the URL and simulate the redirect.
	p := NewProvider(
		WithClientID("test"),
		WithEndpoints("https://auth.example.com/authorize", tokenSrv.URL),
		WithHTTPClient(tokenSrv.Client()),
		WithBrowserOpen(func(authURL string) error {
			// Parse the authorization URL to extract redirect_uri and state
			u, err := url.Parse(authURL)
			if err != nil {
				return err
			}
			redirectURI := u.Query().Get("redirect_uri")
			state := u.Query().Get("state")

			// Simulate the browser completing the callback
			callbackURL := fmt.Sprintf("%s?code=auth-code&state=%s", redirectURI, url.QueryEscape(state))
			resp, err := http.Get(callbackURL)
			if err != nil {
				return err
			}
			resp.Body.Close()
			return nil
		}),
	)

	store := &mockStore{}

	app := &cli.Command{
		Name:   "test",
		Flags:  p.Flags(),
		Writer: io.Discard,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return p.Login(ctx, cmd, store)
		},
	}

	err := app.Run(context.Background(), []string{"test"})
	require.NoError(t, err)

	st, err := unmarshalToken(store.token)
	require.NoError(t, err)
	assert.Equal(t, "pkce-at", st.AccessToken)
}
