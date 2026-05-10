package gemini

import (
	"net/http"

	oauthcore "github.com/nekohy/MeowCLI/core"
)

const (
	geminiOAuthClientID     = "681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com"
	geminiOAuthClientSecret = "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"
	geminiOAuthAuthorizeURL = "https://accounts.google.com/o/oauth2/v2/auth"
	geminiOAuthTokenURL     = "https://oauth2.googleapis.com/token"
	geminiOAuthRedirectURI  = "http://localhost:8085/oauth2callback"
)

var geminiOAuthScopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/userinfo.profile",
}

func NewOAuthFlow(httpClient *http.Client) (*oauthcore.OAuthClient, error) {
	return oauthcore.NewOAuthClient(oauthcore.OAuthConfig{
		Provider:     "gemini",
		ClientID:     geminiOAuthClientID,
		ClientSecret: geminiOAuthClientSecret,
		AuthURL:      geminiOAuthAuthorizeURL,
		TokenURL:     geminiOAuthTokenURL,
		RedirectURI:  geminiOAuthRedirectURI,
		Scopes:       geminiOAuthScopes,
		AuthParams: map[string]string{
			"access_type": "offline",
			"prompt":      "consent",
		},
		HTTPClient: httpClient,
	})
}
