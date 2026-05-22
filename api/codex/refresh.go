package codex

import (
	"context"
	"errors"
	"fmt"
	"github.com/nekohy/MeowCLI/api"
	codexutils "github.com/nekohy/MeowCLI/api/codex/utils"
	commonutils "github.com/nekohy/MeowCLI/utils"
	"net/http"
	"strings"
	"time"
)

type RTResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int    `json:"expires_in"`
}

func (c *Client) RefreshAccessToken(ctx context.Context, refreshToken string) (*codexutils.CodexTokenData, bool, error) {
	tokenInput := parseRefreshTokenInput(refreshToken)
	if tokenInput.RefreshToken == "" {
		return nil, false, fmt.Errorf("refresh token was eaten by a cat")
	}

	reqCtx, cancel := withOptionalTimeout(ctx, commonutils.DefaultUpstreamTimeout)
	defer cancel()

	var result RTResponse
	_, err := c.client.R().
		SetContext(reqCtx).
		SetHeader("Accept", "application/json").
		SetHeader("Content-Type", "application/x-www-form-urlencoded").
		SetFormData(map[string]string{
			"client_id":     tokenInput.ClientID,
			"grant_type":    "refresh_token",
			"refresh_token": tokenInput.RefreshToken,
			"scope":         "openid profile email",
		}).
		SetResult(&result).
		Post(codexutils.RefreshTokenURL)
	if err != nil {
		var apiErr *api.APIError
		if errors.As(err, &apiErr) {
			retryable := apiErr.StatusCode != http.StatusUnauthorized
			return nil, retryable, err
		}
		return nil, true, fmt.Errorf("token refresh request failed: %w", err)
	}

	return &codexutils.CodexTokenData{
		IDToken:      result.IDToken,
		AccessToken:  result.AccessToken,
		RefreshToken: tokenInput.StoredRefreshToken(result.RefreshToken),
		Expire:       time.Now().Add(time.Duration(result.ExpiresIn) * time.Second).Format(time.RFC3339),
	}, false, nil
}

type refreshTokenInput struct {
	RefreshToken string
	ClientID     string
	overrideID   string
}

func parseRefreshTokenInput(input string) refreshTokenInput {
	trimmed := strings.TrimSpace(input)
	result := refreshTokenInput{
		RefreshToken: trimmed,
		ClientID:     codexutils.ClientID,
	}
	if trimmed == "" {
		return result
	}

	at := strings.LastIndex(trimmed, "@")
	if at <= 0 || at == len(trimmed)-1 {
		return result
	}

	refreshToken := strings.TrimSpace(trimmed[:at])
	clientID := strings.TrimSpace(trimmed[at+1:])
	if refreshToken == "" || clientID == "" {
		return result
	}

	result.RefreshToken = refreshToken
	result.ClientID = clientID
	result.overrideID = clientID
	return result
}

func (i refreshTokenInput) StoredRefreshToken(refreshToken string) string {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" || i.overrideID == "" {
		return refreshToken
	}
	return refreshToken + "@" + i.overrideID
}
