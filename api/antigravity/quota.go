package antigravity

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/nekohy/MeowCLI/api"
	"github.com/nekohy/MeowCLI/utils"
	"github.com/tidwall/gjson"
)

const (
	ModelTierClaude    = "claude"
	ModelTierPro       = "pro"
	ModelTierFlash     = "flash"
	ModelTierFlashLite = "flashlite"
	ModelTierTab       = "tab"
	ModelTierImage     = "image"
)

// Quota represents Antigravity quota pools returned by fetchAvailableModels.
// GPT and Claude share the Claude pool; tab completion has a separate pool.
type Quota struct {
	QuotaClaude    float64
	ResetClaude    time.Time
	QuotaPro       float64
	ResetPro       time.Time
	QuotaFlash     float64
	ResetFlash     time.Time
	QuotaFlashlite float64
	ResetFlashlite time.Time
	QuotaTab       float64
	ResetTab       time.Time
	QuotaImage     float64
	ResetImage     time.Time
	CreditsAmount  float64
	CreditTypes    []string
	CreditsSynced  bool
}

type QuotaFetcher interface {
	FetchQuota(ctx context.Context, credentialID string, accessToken string, projectID string) (*Quota, error)
}

type availableModelQuota struct {
	fraction float64
	reset    time.Time
	set      bool
}

func (c *Client) FetchQuota(ctx context.Context, _ string, accessToken string, projectID string) (*Quota, error) {
	token := strings.TrimSpace(accessToken)
	if token == "" {
		return nil, fmt.Errorf("fetch antigravity quota: access token is empty")
	}

	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		projectID = defaultProjectID
	}
	reqBody, _ := sonic.Marshal(map[string]string{"project": projectID})
	quotaURL := fmt.Sprintf("%s/%s:fetchAvailableModels", c.baseURL(), codeAssistVersion)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, quotaURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create antigravity quota request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", antigravityUserAgent())

	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("fetch antigravity quota: %w", err)
	}
	defer resp.Body.Close()

	body, err := readLimitedBody(resp.Body, readBodyLimit)
	if err != nil {
		return nil, fmt.Errorf("read antigravity quota response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &api.APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}
	q := ParseQuotaFromAvailableModels(body)
	if creditsAmount, creditTypes, err := c.fetchAvailableCredits(ctx, token); err == nil {
		q.CreditsAmount = creditsAmount
		q.CreditTypes = creditTypes
		q.CreditsSynced = true
	}
	return q, nil
}

func (c *Client) fetchAvailableCredits(ctx context.Context, accessToken string) (float64, []string, error) {
	body, err := sonic.Marshal(loadCodeAssistRequest{Metadata: antigravityCodeAssistMetadata()})
	if err != nil {
		return 0, nil, fmt.Errorf("marshal antigravity loadCodeAssist request: %w", err)
	}
	respBody, err := c.doCodeAssistControlRequest(ctx, accessToken, "loadCodeAssist", body)
	if err != nil {
		return 0, nil, err
	}
	amount, creditTypes := extractAvailableCredits(respBody)
	return amount, creditTypes, nil
}

func ParseQuotaFromAvailableModels(body []byte) *Quota {
	q := FullQuota()
	models := gjson.GetBytes(body, "models")
	if !models.Exists() || !models.IsObject() {
		return q
	}

	pools := map[string]*availableModelQuota{
		ModelTierClaude:    {},
		ModelTierPro:       {},
		ModelTierFlash:     {},
		ModelTierFlashLite: {},
		ModelTierTab:       {},
		ModelTierImage:     {},
	}

	models.ForEach(func(key, value gjson.Result) bool {
		quotaInfo := value.Get("quotaInfo")
		if !quotaInfo.Exists() {
			return true
		}
		tier := ResolveModelTier(key.String(), value)
		info := pools[tier]
		if info == nil {
			return true
		}
		fractionResult := quotaInfo.Get("remainingFraction")
		if !fractionResult.Exists() {
			return true
		}
		fraction := utils.TruncateQuotaRatio(fractionResult.Float())
		if !info.set || fraction < info.fraction {
			info.fraction = fraction
		}
		reset := parseQuotaResetTime(quotaInfo.Get("resetTime").String())
		if reset.IsZero() && tier == ModelTierTab {
			reset = FarFutureReset()
		}
		if !reset.IsZero() && (!info.set || info.reset.IsZero() || reset.Before(info.reset)) {
			info.reset = reset
		}
		info.set = true
		return true
	})

	applyPool(q, ModelTierClaude, pools[ModelTierClaude])
	applyPool(q, ModelTierPro, pools[ModelTierPro])
	applyPool(q, ModelTierFlash, pools[ModelTierFlash])
	applyPool(q, ModelTierFlashLite, pools[ModelTierFlashLite])
	applyPool(q, ModelTierTab, pools[ModelTierTab])
	applyPool(q, ModelTierImage, pools[ModelTierImage])
	return q
}

func FullQuota() *Quota {
	return &Quota{
		QuotaClaude:    1.0,
		QuotaPro:       1.0,
		QuotaFlash:     1.0,
		QuotaFlashlite: 1.0,
		QuotaTab:       1.0,
		ResetTab:       FarFutureReset(),
		QuotaImage:     1.0,
	}
}

func FarFutureReset() time.Time {
	return time.Now().UTC().AddDate(100, 0, 0)
}

func ResolveModelTier(modelID string, model gjson.Result) string {
	m := strings.ToLower(strings.TrimSpace(modelID))
	displayName := strings.ToLower(model.Get("displayName").String())
	provider := strings.ToLower(model.Get("modelProvider").String() + " " + model.Get("apiProvider").String() + " " + model.Get("model").String())
	modelKey := strings.TrimSpace(m + " " + provider)
	combined := strings.TrimSpace(modelKey + " " + displayName)

	if strings.HasPrefix(m, "tab_") || strings.HasPrefix(m, "chat_") {
		return ModelTierTab
	}
	if strings.Contains(combined, "image") {
		return ModelTierImage
	}
	if strings.Contains(combined, "claude") || strings.Contains(combined, "gpt") {
		return ModelTierClaude
	}
	if strings.Contains(modelKey, "flash-lite") || strings.Contains(modelKey, "flash_lite") || strings.Contains(modelKey, "flashlite") {
		return ModelTierFlashLite
	}
	if strings.Contains(modelKey, "flash") {
		return ModelTierFlash
	}
	if strings.Contains(modelKey, "pro") {
		return ModelTierPro
	}
	return ModelTierClaude
}

func ParseAPIError(err error) (statusCode int, body string, ok bool) {
	if err == nil {
		return 0, "", false
	}
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		return 0, "", false
	}
	return apiErr.StatusCode, apiErr.Body, true
}

func parseQuotaResetTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	reset, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return reset
}

func applyPool(q *Quota, tier string, info *availableModelQuota) {
	if !info.set {
		return
	}
	switch tier {
	case ModelTierClaude:
		q.QuotaClaude = info.fraction
		q.ResetClaude = info.reset
	case ModelTierPro:
		q.QuotaPro = info.fraction
		q.ResetPro = info.reset
	case ModelTierFlash:
		q.QuotaFlash = info.fraction
		q.ResetFlash = info.reset
	case ModelTierFlashLite:
		q.QuotaFlashlite = info.fraction
		q.ResetFlashlite = info.reset
	case ModelTierTab:
		q.QuotaTab = info.fraction
		if info.reset.IsZero() {
			q.ResetTab = FarFutureReset()
		} else {
			q.ResetTab = info.reset
		}
	case ModelTierImage:
		q.QuotaImage = info.fraction
		q.ResetImage = info.reset
	}
}
