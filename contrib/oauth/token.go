package oauth

import (
	"encoding/json"
	"time"

	"golang.org/x/oauth2"
)

// storedToken is the JSON-serializable representation of an OAuth token.
type storedToken struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Expiry       time.Time `json:"expiry,omitempty"`
	Scopes       []string  `json:"scopes,omitempty"`
}

func marshalToken(tok *oauth2.Token, scopes []string) ([]byte, error) {
	st := storedToken{
		AccessToken:  tok.AccessToken,
		TokenType:    tok.TokenType,
		RefreshToken: tok.RefreshToken,
		Expiry:       tok.Expiry,
		Scopes:       scopes,
	}
	return json.Marshal(st)
}

func unmarshalToken(data []byte) (*storedToken, error) {
	var st storedToken
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func (st *storedToken) oauth2Token() *oauth2.Token {
	return &oauth2.Token{
		AccessToken:  st.AccessToken,
		TokenType:    st.TokenType,
		RefreshToken: st.RefreshToken,
		Expiry:       st.Expiry,
	}
}
