package codex

import (
	"net/http"

	oauthcore "github.com/nekohy/MeowCLI/core"
)

const (
	codexOAuthClientID     = "app_EMoamEEZ73f0CkXaXp7hrann"
	codexOAuthAuthorizeURL = "https://auth.openai.com/oauth/authorize"
	codexOAuthTokenURL     = "https://auth.openai.com/oauth/token"
	codexOAuthRedirectURI  = "http://localhost:1455/auth/callback"
)

var codexOAuthScopes = []string{
	"openid",
	"profile",
	"email",
	"offline_access",
}

func NewOAuthFlow(httpClient *http.Client) (*oauthcore.OAuthClient, error) {
	return oauthcore.NewOAuthClient(oauthcore.OAuthConfig{
		Provider:    "codex",
		ClientID:    codexOAuthClientID,
		AuthURL:     codexOAuthAuthorizeURL,
		TokenURL:    codexOAuthTokenURL,
		RedirectURI: codexOAuthRedirectURI,
		Scopes:      codexOAuthScopes,
		AuthParams: map[string]string{
			"id_token_add_organizations": "true",
			"codex_cli_simplified_flow":  "true",
			"originator":                 "codex_cli_rs",
		},
		UsePKCE:    true,
		HTTPClient: httpClient,
	})
}
