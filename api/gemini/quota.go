package gemini

import (
	"context"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/nekohy/MeowCLI/utils"
	"github.com/tidwall/gjson"
)

// Quota represents the remaining quota ratios and reset times for each Gemini model tier.
type Quota struct {
	QuotaPro       float64
	ResetPro       time.Time
	QuotaFlash     float64
	ResetFlash     time.Time
	QuotaFlashlite float64
	ResetFlashlite time.Time
}

// QuotaFetcher is implemented by the API adapter to fetch quota from the upstream service.
type QuotaFetcher interface {
	FetchQuota(ctx context.Context, credentialID string, accessToken string, projectID string) (*Quota, error)
}

// ParseQuotaFromError extracts quota reset delays from a 429 error response
// and converts them to absolute reset times.
// The Google RPC error format contains quotaResetDelay in error.details[].metadata
// which indicates how long until the quota resets.
func ParseQuotaFromError(errorBody []byte) (q *Quota, found bool) {
	details := gjson.GetBytes(errorBody, "error.details")
	if !details.Exists() || !details.IsArray() {
		return
	}

	q = &Quota{}

	for _, detail := range details.Array() {
		if detail.Get("@type").String() != "type.googleapis.com/google.rpc.ErrorInfo" {
			continue
		}
		metadata := detail.Get("metadata")
		if !metadata.Exists() {
			continue
		}

		// Parse generic quotaResetDelay — applies to all tiers
		if delayStr := metadata.Get("quotaResetDelay").String(); delayStr != "" {
			if duration, err := time.ParseDuration(delayStr); err == nil {
				resetTime := time.Now().Add(duration)
				q.ResetPro = resetTime
				q.ResetFlash = resetTime
				q.ResetFlashlite = resetTime
				q.QuotaPro = 0
				q.QuotaFlash = 0
				q.QuotaFlashlite = 0
				found = true
			}
		}

		// Try per-tier delays if available (override generic)
		if delayStr := metadata.Get("proResetDelay").String(); delayStr != "" {
			if duration, err := time.ParseDuration(delayStr); err == nil {
				q.ResetPro = time.Now().Add(duration)
				q.QuotaPro = 0
				found = true
			}
		}
		if delayStr := metadata.Get("flashResetDelay").String(); delayStr != "" {
			if duration, err := time.ParseDuration(delayStr); err == nil {
				q.ResetFlash = time.Now().Add(duration)
				q.QuotaFlash = 0
				found = true
			}
		}
		if delayStr := metadata.Get("flashliteResetDelay").String(); delayStr != "" {
			if duration, err := time.ParseDuration(delayStr); err == nil {
				q.ResetFlashlite = time.Now().Add(duration)
				q.QuotaFlashlite = 0
				found = true
			}
		}
	}

	if !found {
		return nil, false
	}

	return
}

// ParseQuotaFromErrorText is a convenience wrapper that accepts a string error body.
func ParseQuotaFromErrorText(text string) (*Quota, bool) {
	return ParseQuotaFromError([]byte(text))
}

// ParseQuotaFromErrorOrFull returns the parsed quota from a 429 error body,
// or a zero-quota default if no quota info was found.
func ParseQuotaFromErrorOrFull(errorBody []byte) *Quota {
	if q, found := ParseQuotaFromError(errorBody); found {
		return q
	}
	return &Quota{}
}

// FullQuota returns a Quota with all tiers set to maximum (1.0).
func FullQuota() *Quota {
	return &Quota{
		QuotaPro:       1.0,
		QuotaFlash:     1.0,
		QuotaFlashlite: 1.0,
	}
}

// quotaBucket represents a single bucket from the retrieveUserQuota response.
type quotaBucket struct {
	ResetTime         string  `json:"resetTime"`
	TokenType         string  `json:"tokenType"`
	ModelID           string  `json:"modelId"`
	RemainingFraction float64 `json:"remainingFraction"`
}

// retrieveUserQuotaResponse is the top-level response from the quota API.
type retrieveUserQuotaResponse struct {
	Buckets []quotaBucket `json:"buckets"`
}

// ParseQuotaFromBuckets parses the retrieveUserQuota API response and maps
// model IDs to the pro/flash/flashlite tiers. For each tier, the minimum
// remainingFraction and earliest resetTime across all models in that tier is used.
func ParseQuotaFromBuckets(body []byte) *Quota {
	var resp retrieveUserQuotaResponse
	if err := sonic.Unmarshal(body, &resp); err != nil || len(resp.Buckets) == 0 {
		return FullQuota()
	}

	q := &Quota{
		QuotaPro:       1.0,
		QuotaFlash:     1.0,
		QuotaFlashlite: 1.0,
	}

	// Track per-tier minima for remaining fraction and earliest reset.
	type tierInfo struct {
		minFraction   float64
		earliestReset time.Time
		set           bool
	}
	tiers := map[string]*tierInfo{
		ModelTierPro:       {},
		ModelTierFlash:     {},
		ModelTierFlashLite: {},
	}

	for _, b := range resp.Buckets {
		if b.TokenType != "REQUESTS" {
			continue
		}
		tier := resolveTierFromModelID(b.ModelID)
		info, ok := tiers[tier]
		if !ok {
			continue
		}
		fraction := utils.TruncateQuotaRatio(b.RemainingFraction)
		if !info.set || fraction < info.minFraction {
			info.minFraction = fraction
		}
		resetTime, err := time.Parse(time.RFC3339, b.ResetTime)
		if err == nil {
			if !info.set || resetTime.Before(info.earliestReset) {
				info.earliestReset = resetTime
			}
		}
		info.set = true
	}

	if info := tiers[ModelTierPro]; info.set {
		q.QuotaPro = info.minFraction
		if !info.earliestReset.IsZero() {
			q.ResetPro = info.earliestReset
		}
	}
	if info := tiers[ModelTierFlash]; info.set {
		q.QuotaFlash = info.minFraction
		if !info.earliestReset.IsZero() {
			q.ResetFlash = info.earliestReset
		}
	}
	if info := tiers[ModelTierFlashLite]; info.set {
		q.QuotaFlashlite = info.minFraction
		if !info.earliestReset.IsZero() {
			q.ResetFlashlite = info.earliestReset
		}
	}

	return q
}

// resolveTierFromModelID maps a Gemini model ID to a quota tier.
func resolveTierFromModelID(modelID string) string {
	m := strings.ToLower(modelID)
	if strings.Contains(m, "flash-lite") || strings.Contains(m, "flashlite") {
		return ModelTierFlashLite
	}
	if strings.Contains(m, "flash") {
		return ModelTierFlash
	}
	if strings.Contains(m, "pro") {
		return ModelTierPro
	}
	return ModelTierPro
}

// Model tier constants used by quota parsing.
const (
	ModelTierPro       = "pro"
	ModelTierFlash     = "flash"
	ModelTierFlashLite = "flashlite"
)
