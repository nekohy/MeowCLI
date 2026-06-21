package codex

import (
	"context"
	"fmt"
	"strings"

	codexutils "github.com/nekohy/MeowCLI/api/codex/utils"
	commonutils "github.com/nekohy/MeowCLI/utils"
)

type WhoamiData struct {
	Email     string
	AccountID string
	PlanType  string
}

type whoamiResponse struct {
	ChatGPTAccountID string `json:"chatgpt_account_id"`
	ChatGPTPlanType  string `json:"chatgpt_plan_type"`
	Email            string `json:"email"`
}

func (c *Client) FetchWhoami(ctx context.Context, accessToken string) (*WhoamiData, error) {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return nil, fmt.Errorf("access_token is required")
	}

	reqCtx, cancel := withOptionalTimeout(ctx, commonutils.DefaultUpstreamTimeout)
	defer cancel()

	var body whoamiResponse
	_, err := c.client.R().
		SetContext(reqCtx).
		SetHeader("Accept", "application/json").
		SetHeader("Authorization", "Bearer "+accessToken).
		SetResult(&body).
		Get(codexutils.WhoamiURL)
	if err != nil {
		return nil, fmt.Errorf("fetch chatgpt whoami: %w", err)
	}

	result := &WhoamiData{
		Email:     strings.ToLower(strings.TrimSpace(body.Email)),
		AccountID: strings.TrimSpace(body.ChatGPTAccountID),
		PlanType:  strings.TrimSpace(body.ChatGPTPlanType),
	}
	if result.Email == "" {
		return nil, fmt.Errorf("chatgpt whoami missing email")
	}
	if result.AccountID == "" {
		return nil, fmt.Errorf("chatgpt whoami missing chatgpt_account_id")
	}
	return result, nil
}
