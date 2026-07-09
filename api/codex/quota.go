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
	PlanType             string                `json:"plan_type"`
	RateLimit            usageRateLimit        `json:"rate_limit"`
	AdditionalRateLimits []additionalRateLimit `json:"additional_rate_limits"`
	RateLimitResetCredits *rateLimitResetCreditsSummary `json:"rate_limit_reset_credits,omitempty"`
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

type rateLimitResetCreditsSummary struct {
	AvailableCount int `json:"available_count"`
}

// RateLimitResetCredit 是转发给前端的单条重置额度（仅保留必要字段，其余上游字段忽略）
type RateLimitResetCredit struct {
	Title     string  `json:"title"`
	ExpiresAt *string `json:"expires_at"` // null/absent → nil
	Status    string  `json:"status"`
}

// RateLimitResetCredits 是 rate-limit-reset-credits 列表响应（转发给前端）
type RateLimitResetCredits struct {
	Credits        []RateLimitResetCredit `json:"credits"`
	AvailableCount int                    `json:"available_count"`
}

// ConsumeRateLimitResetCreditResponse 是消耗一次重置额度后的上游响应
type ConsumeRateLimitResetCreditResponse struct {
	Code         string `json:"code"`
	WindowsReset int    `json:"windows_reset"`
}

func (c *Client) FetchQuota(ctx context.Context, credentialID, accessToken string) (*codexutils.Quota, error) {
	reqCtx, cancel := withOptionalTimeout(ctx, utils.DefaultUpstreamTimeout)
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

func (c *Client) FetchRateLimitResetCredits(ctx context.Context, credentialID, accessToken string) (*RateLimitResetCredits, error) {
	reqCtx, cancel := withOptionalTimeout(ctx, utils.DefaultUpstreamTimeout)
	defer cancel()

	var out RateLimitResetCredits
	_, err := c.client.R().
		SetContext(reqCtx).
		SetHeader("Authorization", "Bearer "+accessToken).
		SetHeader("Chatgpt-Account-Id", utils.AccountIDFromCredentialID(credentialID)).
		SetResult(&out).
		Get(codexutils.RateLimitResetCreditsURL)
	if err != nil {
		return nil, fmt.Errorf("fetch rate-limit reset credits: %w", err)
	}
	return &out, nil
}

func (c *Client) ConsumeRateLimitResetCredit(ctx context.Context, credentialID, accessToken, redeemRequestID string) (*ConsumeRateLimitResetCreditResponse, error) {
	reqCtx, cancel := withOptionalTimeout(ctx, utils.DefaultUpstreamTimeout)
	defer cancel()

	var out ConsumeRateLimitResetCreditResponse
	_, err := c.client.R().
		SetContext(reqCtx).
		SetHeader("Authorization", "Bearer "+accessToken).
		SetHeader("Chatgpt-Account-Id", utils.AccountIDFromCredentialID(credentialID)).
		SetBody(map[string]string{"redeem_request_id": redeemRequestID}).
		SetResult(&out).
		Post(codexutils.ConsumeRateLimitResetCreditURL)
	if err != nil {
		return nil, fmt.Errorf("consume rate-limit reset credit: %w", err)
	}
	return &out, nil
}

func parseUsageQuota(usage usageResponse) *codexutils.Quota {
	quota := codexutils.NewQuota()
	q := &quota
	q.PlanType = normalizeUsagePlanType(usage.PlanType)
	q.HasDefaultQuota = true
	if usage.RateLimitResetCredits != nil {
		q.ResetCreditsCount = usage.RateLimitResetCredits.AvailableCount
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
			q.QuotaSpark1mo = 1.0
		}
		sparkFound = true
		q.HasSparkQuota = true
		applyUsageRateLimit(q, extra.RateLimit, true)
	}

	return q
}

func normalizeUsagePlanType(planType string) string {
	normalized := strings.ToLower(strings.TrimSpace(planType))
	switch normalized {
	case "free", "plus", "edu", "prolite", "pro", "team", "enterprise", "unknown":
		return normalized
	default:
		return ""
	}
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
		remaining := utils.TruncateQuotaRatio(float64(100-w.UsedPercent) / 100)
		resetAt := usageResetTime(w)
		switch {
		case codexutils.WindowMatches(int64((5 * time.Hour).Seconds()), w.LimitWindowSeconds): // 18000
			if spark {
				q.QuotaSpark5h = remaining
				q.ResetSpark5h = resetAt
			} else {
				q.Quota5h = remaining
				q.Reset5h = resetAt
			}
		case codexutils.WindowMatches(int64((7 * 24 * time.Hour).Seconds()), w.LimitWindowSeconds): // 604800
			if spark {
				q.QuotaSpark7d = remaining
				q.ResetSpark7d = resetAt
			} else {
				q.Quota7d = remaining
				q.Reset7d = resetAt
			}
		case codexutils.WindowMatches(int64((30 * 24 * time.Hour).Seconds()), w.LimitWindowSeconds): // 2592000
			if spark {
				q.QuotaSpark1mo = remaining
				q.ResetSpark1mo = resetAt
			} else {
				q.Quota1mo = remaining
				q.Reset1mo = resetAt
			}
		}
	}
}

func usageResetTime(w *rateLimitWindow) time.Time {
	if w == nil {
		return time.Time{}
	}
	if w.ResetAt > 0 {
		return time.Unix(w.ResetAt, 0)
	}
	if w.ResetAfterSeconds > 0 {
		return time.Now().Add(time.Duration(w.ResetAfterSeconds) * time.Second)
	}
	return time.Time{}
}
