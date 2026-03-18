package codex

import (
	"context"
	"errors"
	"fmt"
	"github.com/nekohy/MeowCLI/api"
	codexutils "github.com/nekohy/MeowCLI/api/codex/utils"
	"net/http"
	"time"
)

type RTResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int    `json:"expires_in"`
}

func (c *Client) RefreshAccessToken(ctx context.Context, refreshToken string) (*codexutils.CodexTokenData, bool, error) {
	if refreshToken == "" {
		return nil, false, fmt.Errorf("refresh token was eaten by a cat")
	}

	var result RTResponse
	_, err := c.client.R().
		SetContext(ctx).
		SetHeader("Accept", "application/json").
		SetHeader("Content-Type", "application/x-www-form-urlencoded").
		SetFormData(map[string]string{
			"client_id":     codexutils.ClientID,
			"grant_type":    "refresh_token",
			"refresh_token": refreshToken,
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
		RefreshToken: result.RefreshToken,
		Expire:       time.Now().Add(time.Duration(result.ExpiresIn) * time.Second).Format(time.RFC3339),
	}, false, nil
}
