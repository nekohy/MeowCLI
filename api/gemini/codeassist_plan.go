package gemini

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/nekohy/MeowCLI/api"
)

const (
	codeAssistPlanUltra = "ultra"
	codeAssistPlanPro   = "pro"
	codeAssistPlanFree  = "free"
)

type codeAssistLoadResponse struct {
	CurrentTier *codeAssistTier `json:"currentTier"`
	PaidTier    *codeAssistTier `json:"paidTier"`
}

type codeAssistTier struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type codeAssistLoadRequest struct {
	CloudaicompanionProject string                 `json:"cloudaicompanionProject"`
	Metadata                codeAssistLoadMetadata `json:"metadata"`
}

type codeAssistLoadMetadata struct {
	IDEType     string `json:"ideType"`
	Platform    string `json:"platform"`
	PluginType  string `json:"pluginType"`
	DuetProject string `json:"duetProject"`
}

// ResolveCodeAssistPlanFromLoadResponse maps loadCodeAssist subscription
// fields into MeowCLI's Gemini plan_type values.
func ResolveCodeAssistPlanFromLoadResponse(body []byte) string {
	var payload codeAssistLoadResponse
	if err := sonic.Unmarshal(body, &payload); err != nil {
		return codeAssistPlanFree
	}
	if plan := resolveCodeAssistPlanFromTier(payload.PaidTier); plan != "" {
		return plan
	}
	//if plan := resolveCodeAssistPlanFromTier(payload.CurrentTier); plan != "" {
	//	return plan
	//}
	return codeAssistPlanFree
}

func resolveCodeAssistPlanFromTier(tier *codeAssistTier) string {
	if tier == nil {
		return ""
	}
	id := strings.ToLower(strings.TrimSpace(tier.ID))
	switch {
	case strings.Contains(id, "ultra"):
		return codeAssistPlanUltra
	case strings.Contains(id, "pro"):
		return codeAssistPlanPro
	case strings.Contains(id, "free"):
		return codeAssistPlanFree
	default:
		return ""
	}
}

// LoadCodeAssistPlan fetches the Gemini Code Assist subscription tier for a
// project. If the response does not contain a paid tier, it resolves to free.
func (c *Client) LoadCodeAssistPlan(ctx context.Context, accessToken string, projectID string) (string, error) {
	token := strings.TrimSpace(accessToken)
	if token == "" {
		return "", fmt.Errorf("load code assist plan: access token is empty")
	}
	pid := strings.TrimSpace(projectID)
	if pid == "" {
		return "", fmt.Errorf("load code assist plan: project id is empty")
	}

	reqBody, _ := sonic.Marshal(codeAssistLoadRequest{
		CloudaicompanionProject: pid,
		Metadata: codeAssistLoadMetadata{
			IDEType:     "IDE_UNSPECIFIED",
			Platform:    "PLATFORM_UNSPECIFIED",
			PluginType:  "GEMINI",
			DuetProject: pid,
		},
	})
	loadURL := fmt.Sprintf("%s/%s:loadCodeAssist", c.codeAssistEndpoint(), codeAssistVersion)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, loadURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("create load code assist request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", geminiCLIUserAgent(""))

	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("load code assist plan: %w", err)
	}
	defer resp.Body.Close()

	body, err := readLimitedBody(resp.Body, readBodyLimit)
	if err != nil {
		return "", fmt.Errorf("read load code assist response: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", &api.APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}
	if resp.StatusCode != http.StatusOK {
		return "", &api.APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}
	return ResolveCodeAssistPlanFromLoadResponse(body), nil
}
