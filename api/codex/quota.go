package codex

import (
	"context"
	"fmt"
	codexutils "github.com/nekohy/MeowCLI/api/codex/utils"
	"github.com/nekohy/MeowCLI/utils"
	"time"
)

type usageResponse struct {
	RateLimit struct {
		PrimaryWindow   *rateLimitWindow `json:"primary_window"`
		SecondaryWindow *rateLimitWindow `json:"secondary_window"`
	} `json:"rate_limit"`
}

type rateLimitWindow struct {
	UsedPercent        int   `json:"used_percent"`
	LimitWindowSeconds int64 `json:"limit_window_seconds"`
	ResetAfterSeconds  int64 `json:"reset_after_seconds"`
	ResetAt            int64 `json:"reset_at"`
}

func (c *Client) FetchQuota(ctx context.Context, credentialID, accessToken string) (*codexutils.Quota, error) {
	var usage usageResponse
	_, err := c.client.R().
		SetContext(ctx).
		SetHeader("Authorization", "Bearer "+accessToken).
		SetHeader("Chatgpt-Account-Id", utils.AccountIDFromCredentialID(credentialID)).
		SetResult(&usage).
		Get(codexutils.UsageURL)
	if err != nil {
		return nil, fmt.Errorf("fetch quota: %w", err)
	}

	q := &codexutils.Quota{Quota5h: 1.0, Quota7d: 1.0}
	for _, w := range []*rateLimitWindow{
		usage.RateLimit.PrimaryWindow,
		usage.RateLimit.SecondaryWindow,
	} {
		if w == nil {
			continue
		}
		remaining := float64(100-w.UsedPercent) / 100
		resetAt := time.Unix(w.ResetAt, 0)
		switch w.LimitWindowSeconds {
		case int64((5 * time.Hour).Seconds()): // 18000
			q.Quota5h = remaining
			q.Reset5h = resetAt
		case int64((7 * 24 * time.Hour).Seconds()): // 604800
			q.Quota7d = remaining
			q.Reset7d = resetAt
		}
	}

	return q, nil
}
