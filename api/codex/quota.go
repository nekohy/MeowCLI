package codex

import (
	"context"
	"fmt"
	"strings"
	"time"

	codexutils "github.com/nekohy/MeowCLI/api/codex/utils"
	"github.com/nekohy/MeowCLI/utils"
)

type usageResponse struct {
	RateLimit            usageRateLimit        `json:"rate_limit"`
	AdditionalRateLimits []additionalRateLimit `json:"additional_rate_limits"`
}

type usageRateLimit struct {
	PrimaryWindow   *rateLimitWindow `json:"primary_window"`
	SecondaryWindow *rateLimitWindow `json:"secondary_window"`
}

type additionalRateLimit struct {
	LimitName      string         `json:"limit_name"`
	MeteredFeature string         `json:"metered_feature"`
	RateLimit      usageRateLimit `json:"rate_limit"`
}

type rateLimitWindow struct {
	UsedPercent        int   `json:"used_percent"`
	LimitWindowSeconds int64 `json:"limit_window_seconds"`
	ResetAfterSeconds  int64 `json:"reset_after_seconds"`
	ResetAt            int64 `json:"reset_at"`
}

func (c *Client) FetchQuota(ctx context.Context, credentialID, accessToken string) (*codexutils.Quota, error) {
	reqCtx, cancel := withOptionalTimeout(ctx, defaultQuotaRequestTimeout)
	defer cancel()

	var usage usageResponse
	_, err := c.client.R().
		SetContext(reqCtx).
		SetHeader("Authorization", "Bearer "+accessToken).
		SetHeader("Chatgpt-Account-Id", utils.AccountIDFromCredentialID(credentialID)).
		SetResult(&usage).
		Get(codexutils.UsageURL)
	if err != nil {
		return nil, fmt.Errorf("fetch quota: %w", err)
	}

	return parseUsageQuota(usage), nil
}

func parseUsageQuota(usage usageResponse) *codexutils.Quota {
	q := &codexutils.Quota{
		Quota5h:         1.0,
		Quota7d:         1.0,
		QuotaSpark5h:    1.0,
		QuotaSpark7d:    1.0,
		HasDefaultQuota: true,
	}
	applyUsageRateLimit(q, usage.RateLimit, false)

	sparkFound := false
	for _, extra := range usage.AdditionalRateLimits {
		if !isSparkUsageRateLimit(extra) {
			continue
		}
		if !sparkFound {
			q.QuotaSpark5h = 1.0
			q.QuotaSpark7d = 1.0
		}
		sparkFound = true
		q.HasSparkQuota = true
		applyUsageRateLimit(q, extra.RateLimit, true)
	}

	return q
}

func isSparkUsageRateLimit(extra additionalRateLimit) bool {
	return strings.EqualFold(extra.MeteredFeature, "codex_bengalfox") ||
		strings.Contains(strings.ToLower(extra.LimitName), "spark")
}

func applyUsageRateLimit(q *codexutils.Quota, rl usageRateLimit, spark bool) {
	for _, w := range []*rateLimitWindow{
		rl.PrimaryWindow,
		rl.SecondaryWindow,
	} {
		if w == nil {
			continue
		}
		remaining := float64(100-w.UsedPercent) / 100
		resetAt := time.Unix(w.ResetAt, 0)
		switch w.LimitWindowSeconds {
		case int64((5 * time.Hour).Seconds()): // 18000
			if spark {
				q.QuotaSpark5h = remaining
				q.ResetSpark5h = resetAt
			} else {
				q.Quota5h = remaining
				q.Reset5h = resetAt
			}
		case int64((7 * 24 * time.Hour).Seconds()): // 604800
			if spark {
				q.QuotaSpark7d = remaining
				q.ResetSpark7d = resetAt
			} else {
				q.Quota7d = remaining
				q.Reset7d = resetAt
			}
		}
	}
}
