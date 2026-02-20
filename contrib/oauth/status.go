package oauth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/drewfead/proto-cli/cliauth"
)

// Status returns a human-readable summary of the stored authentication state.
func (p *Provider) Status(ctx context.Context, store cliauth.AuthStore) (string, error) {
	st, err := loadToken(ctx, store)
	if err != nil {
		if errors.Is(err, cliauth.ErrNotFound) {
			return "", nil
		}
		if err.Error() == "stored credentials are corrupted" {
			return "Stored credentials are corrupted.", nil
		}
		return "", err
	}

	var lines []string

	tokenType := st.TokenType
	if tokenType == "" {
		tokenType = "Bearer"
	}
	lines = append(lines, fmt.Sprintf("Token type: %s", tokenType))

	if len(st.Scopes) > 0 {
		lines = append(lines, fmt.Sprintf("Scopes: %s", strings.Join(st.Scopes, ", ")))
	}

	if !st.Expiry.IsZero() {
		remaining := time.Until(st.Expiry)
		if remaining > 0 {
			lines = append(lines, fmt.Sprintf("Expires in: %s", remaining.Truncate(time.Second)))
		} else {
			lines = append(lines, "Status: Expired")
		}
	}

	if st.RefreshToken != "" {
		lines = append(lines, "Refresh token: Available")
	}

	return strings.Join(lines, "\n"), nil
}
