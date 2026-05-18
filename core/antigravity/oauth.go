package antigravity

import (
	"net/http"

	oauthcore "github.com/nekohy/MeowCLI/core"
)

const (
	antigravityOAuthClientID     = "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"
	antigravityOAuthClientSecret = "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"
	antigravityOAuthAuthorizeURL = "https://accounts.google.com/o/oauth2/v2/auth"
	antigravityOAuthTokenURL     = "https://oauth2.googleapis.com/token"
	antigravityOAuthRedirectURI  = "http://localhost:51121/oauth-callback"
)

var antigravityOAuthScopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/userinfo.profile",
	"https://www.googleapis.com/auth/cclog",
	"https://www.googleapis.com/auth/experimentsandconfigs",
}

func NewOAuthFlow(httpClient *http.Client) (*oauthcore.OAuthClient, error) {
	return oauthcore.NewOAuthClient(oauthcore.OAuthConfig{
		Provider:     "antigravity",
		ClientID:     antigravityOAuthClientID,
		ClientSecret: antigravityOAuthClientSecret,
		AuthURL:      antigravityOAuthAuthorizeURL,
		TokenURL:     antigravityOAuthTokenURL,
		RedirectURI:  antigravityOAuthRedirectURI,
		Scopes:       antigravityOAuthScopes,
		AuthParams: map[string]string{
			"access_type": "offline",
			"prompt":      "consent",
		},
		HTTPClient: httpClient,
	})
}
