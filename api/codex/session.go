package codex

import (
	"context"
	"fmt"
	"strings"

	codexutils "github.com/nekohy/MeowCLI/api/codex/utils"
	commonutils "github.com/nekohy/MeowCLI/utils"
)

type SessionData struct {
	Email     string
	AccountID string
}

type chatGPTSessionResponse struct {
	User struct {
		Email string `json:"email"`
	} `json:"user"`
	Account struct {
		ID string `json:"id"`
	} `json:"account"`
}

func (c *Client) FetchSession(ctx context.Context, sessionToken string) (*SessionData, error) {
	sessionToken = strings.TrimSpace(sessionToken)
	if sessionToken == "" {
		return nil, fmt.Errorf("session token is required")
	}

	reqCtx, cancel := withOptionalTimeout(ctx, commonutils.DefaultUpstreamTimeout)
	defer cancel()

	var session chatGPTSessionResponse
	_, err := c.client.R().
		SetContext(reqCtx).
		SetHeader("Accept", "application/json").
		SetHeader("Authorization", "Bearer "+sessionToken).
		SetResult(&session).
		Get(codexutils.SessionURL)
	if err != nil {
		return nil, fmt.Errorf("fetch chatgpt session: %w", err)
	}

	result := &SessionData{
		Email:     strings.ToLower(strings.TrimSpace(session.User.Email)),
		AccountID: strings.TrimSpace(session.Account.ID),
	}
	if result.Email == "" {
		return nil, fmt.Errorf("chatgpt session missing user.email")
	}
	if result.AccountID == "" {
		return nil, fmt.Errorf("chatgpt session missing account.id")
	}
	return result, nil
}
