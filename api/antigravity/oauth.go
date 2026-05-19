package antigravity

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/nekohy/MeowCLI/api"
	"github.com/nekohy/MeowCLI/utils"
	"github.com/tidwall/gjson"
)

type refreshTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

type TokenData struct {
	AccessToken  string
	RefreshToken string
	Expiry       time.Time
}

type userInfoResponse struct {
	Email string `json:"email"`
}

type ProjectProfile struct {
	ProjectID     string
	PlanType      string
	CreditsAmount float64
	CreditTypes   []string
}

type loadCodeAssistRequest struct {
	Metadata antigravityMetadata `json:"metadata"`
}

type onboardUserRequest struct {
	TierID   string              `json:"tierId"`
	Metadata antigravityMetadata `json:"metadata"`
}

type antigravityMetadata struct {
	IDEType    string `json:"ide_type"`
	IDEVersion string `json:"ide_version"`
	IDEName    string `json:"ide_name"`
}

func (c *Client) RefreshAccessToken(ctx context.Context, refreshToken string) (*TokenData, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, fmt.Errorf("refresh token is required")
	}

	form := url.Values{}
	form.Set("client_id", antigravityClientID)
	form.Set("client_secret", antigravityClientSecret)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenRefreshURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := utils.ReadLimitedBody(resp.Body, 1<<20)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &api.APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var payload refreshTokenResponse
	if err := sonic.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return nil, fmt.Errorf("antigravity refresh returned empty access_token")
	}
	nextRefresh := refreshToken
	if strings.TrimSpace(payload.RefreshToken) != "" {
		nextRefresh = strings.TrimSpace(payload.RefreshToken)
	}
	expiresIn := payload.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	return &TokenData{
		AccessToken:  strings.TrimSpace(payload.AccessToken),
		RefreshToken: nextRefresh,
		Expiry:       time.Now().Add(time.Duration(expiresIn) * time.Second),
	}, nil
}

func (c *Client) FetchUserEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", antigravityUserAgent())

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := utils.ReadLimitedBody(resp.Body, 1<<20)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &api.APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var payload userInfoResponse
	if err := sonic.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	if strings.TrimSpace(payload.Email) == "" {
		return "", fmt.Errorf("antigravity userinfo returned empty email")
	}
	return strings.TrimSpace(payload.Email), nil
}

func (c *Client) ResolveProjectID(ctx context.Context, accessToken string) (string, error) {
	profile, err := c.ResolveProjectProfile(ctx, accessToken)
	if err != nil {
		return "", err
	}
	return profile.ProjectID, nil
}

func (c *Client) ResolveProjectProfile(ctx context.Context, accessToken string) (*ProjectProfile, error) {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return nil, fmt.Errorf("resolve antigravity project_id: access token is empty")
	}

	body, err := sonic.Marshal(loadCodeAssistRequest{Metadata: antigravityCodeAssistMetadata()})
	if err != nil {
		return nil, fmt.Errorf("marshal antigravity loadCodeAssist request: %w", err)
	}
	respBody, err := c.doCodeAssistControlRequest(ctx, accessToken, "loadCodeAssist", body)
	if err != nil {
		return nil, err
	}

	if projectID := extractCodeAssistProjectID(respBody, "cloudaicompanionProject"); projectID != "" {
		creditsAmount, creditTypes := extractAvailableCredits(respBody)
		return &ProjectProfile{
			ProjectID:     projectID,
			PlanType:      resolveAntigravityPlanType(respBody),
			CreditsAmount: creditsAmount,
			CreditTypes:   creditTypes,
		}, nil
	}
	tierID := defaultCodeAssistTier(respBody)
	projectID, err := c.onboardUser(ctx, accessToken, tierID)
	if err != nil {
		return nil, err
	}
	creditsAmount, creditTypes := extractAvailableCredits(respBody)
	return &ProjectProfile{
		ProjectID:     projectID,
		PlanType:      resolveAntigravityPlanType(respBody),
		CreditsAmount: creditsAmount,
		CreditTypes:   creditTypes,
	}, nil
}

func (c *Client) onboardUser(ctx context.Context, accessToken string, tierID string) (string, error) {
	tierID = strings.TrimSpace(tierID)
	if tierID == "" {
		tierID = "legacy-tier"
	}
	body, err := sonic.Marshal(onboardUserRequest{
		TierID:   tierID,
		Metadata: antigravityCodeAssistMetadata(),
	})
	if err != nil {
		return "", fmt.Errorf("marshal antigravity onboardUser request: %w", err)
	}

	var lastBody []byte
	for attempt := 0; attempt < 5; attempt++ {
		respBody, err := c.doCodeAssistControlRequest(ctx, accessToken, "onboardUser", body)
		if err != nil {
			return "", err
		}
		lastBody = respBody
		if !gjson.GetBytes(respBody, "done").Bool() {
			if attempt < 4 {
				time.Sleep(2 * time.Second)
			}
			continue
		}
		if projectID := extractCodeAssistProjectID(respBody, "response.cloudaicompanionProject"); projectID != "" {
			return projectID, nil
		}
		return "", fmt.Errorf("antigravity onboardUser response missing project_id")
	}
	return "", fmt.Errorf("antigravity onboardUser did not complete: %s", strings.TrimSpace(string(lastBody)))
}

func (c *Client) doCodeAssistControlRequest(ctx context.Context, accessToken string, action string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		fmt.Sprintf("%s/%s:%s", c.baseURL(), codeAssistVersion, action),
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("create antigravity %s request: %w", action, err)
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", antigravityLoadCodeAssistUserAgent())
	req.Header.Set("X-Goog-Api-Client", antigravityGoogAPIClientUA)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute antigravity %s request: %w", action, err)
	}
	defer resp.Body.Close()

	respBody, err := utils.ReadLimitedBody(resp.Body, readBodyLimit)
	if err != nil {
		return nil, fmt.Errorf("read antigravity %s response: %w", action, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &api.APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	return respBody, nil
}

func antigravityCodeAssistMetadata() antigravityMetadata {
	return antigravityMetadata{
		IDEType:    "ANTIGRAVITY",
		IDEVersion: antigravityVersion,
		IDEName:    "antigravity",
	}
}

func extractCodeAssistProjectID(body []byte, path string) string {
	value := gjson.GetBytes(body, path)
	if !value.Exists() {
		return ""
	}
	if value.Type == gjson.String {
		return strings.TrimSpace(value.String())
	}
	if value.IsObject() {
		return strings.TrimSpace(value.Get("id").String())
	}
	return ""
}

func defaultCodeAssistTier(body []byte) string {
	tierID := "legacy-tier"
	tiers := gjson.GetBytes(body, "allowedTiers")
	if !tiers.IsArray() {
		return tierID
	}
	for _, tier := range tiers.Array() {
		if tier.Get("isDefault").Bool() {
			if id := strings.TrimSpace(tier.Get("id").String()); id != "" {
				return id
			}
		}
	}
	return tierID
}

func resolveAntigravityPlanType(body []byte) string {
	for _, path := range []string{"paidTier", "currentTier"} {
		if planType := planTypeFromTier(gjson.GetBytes(body, path)); planType != "" {
			return planType
		}
	}
	tiers := gjson.GetBytes(body, "allowedTiers")
	if tiers.IsArray() {
		for _, tier := range tiers.Array() {
			if !tier.Get("isDefault").Bool() {
				continue
			}
			if planType := planTypeFromTier(tier); planType != "" {
				return planType
			}
		}
	}
	return "unknown"
}

func planTypeFromTier(tier gjson.Result) string {
	if !tier.Exists() || !tier.IsObject() {
		return ""
	}
	combined := strings.ToLower(strings.TrimSpace(strings.Join([]string{
		tier.Get("id").String(),
		tier.Get("name").String(),
		tier.Get("description").String(),
	}, " ")))
	switch {
	case strings.Contains(combined, "ultra"):
		return "ultra"
	case strings.Contains(combined, "pro"):
		return "pro"
	case strings.Contains(combined, "free") || strings.Contains(combined, "legacy") || strings.Contains(combined, "default"):
		return "free"
	default:
		return ""
	}
}

func extractAvailableCredits(body []byte) (float64, []string) {
	seen := map[string]struct{}{}
	var amount float64
	var types []string
	collectAvailableCredits := func(credits gjson.Result) {
		if !credits.IsArray() {
			return
		}
		for _, credit := range credits.Array() {
			amount += credit.Get("creditAmount").Float()
			creditType := strings.TrimSpace(credit.Get("creditType").String())
			if creditType == "" {
				continue
			}
			if _, ok := seen[creditType]; ok {
				continue
			}
			seen[creditType] = struct{}{}
			types = append(types, creditType)
		}
	}
	for _, path := range []string{"paidTier.availableCredits", "currentTier.availableCredits"} {
		collectAvailableCredits(gjson.GetBytes(body, path))
	}
	tiers := gjson.GetBytes(body, "allowedTiers")
	if tiers.IsArray() {
		for _, tier := range tiers.Array() {
			collectAvailableCredits(tier.Get("availableCredits"))
		}
	}
	slices.Sort(types)
	return amount, types
}
