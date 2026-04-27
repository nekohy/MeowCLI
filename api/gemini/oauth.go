package gemini

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/nekohy/MeowCLI/api"
)

type TokenData struct {
	AccessToken  string
	RefreshToken string
	Expiry       time.Time
}

type refreshTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

type userInfoResponse struct {
	Email string `json:"email"`
}

type listProjectsResponse struct {
	Projects []googleProject `json:"projects"`
}

type googleProject struct {
	ProjectID      string `json:"projectId"`
	ProjectNumber  string `json:"projectNumber"`
	LifecycleState string `json:"lifecycleState"`
}

func (c *Client) RefreshAccessToken(ctx context.Context, refreshToken string) (*TokenData, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, fmt.Errorf("refresh token is required")
	}

	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
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

	body, err := readLimitedBody(resp.Body, 1<<20)
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
		return nil, fmt.Errorf("gemini refresh returned empty access_token")
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
	req.Header.Set("User-Agent", geminiCLIUserAgent(""))

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := readLimitedBody(resp.Body, 1<<20)
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
		return "", fmt.Errorf("gemini userinfo returned empty email")
	}
	return strings.TrimSpace(payload.Email), nil
}

func (c *Client) ResolveProjectID(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cloudResourceMgr+"/v1/projects", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", geminiCLIUserAgent(""))

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := readLimitedBody(resp.Body, 2<<20)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &api.APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var payload listProjectsResponse
	if err := sonic.Unmarshal(body, &payload); err != nil {
		return "", err
	}

	activeProjectIDs := make([]string, 0, len(payload.Projects))
	for _, project := range payload.Projects {
		projectID := strings.TrimSpace(project.ProjectID)
		if projectID == "" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(project.LifecycleState), "ACTIVE") {
			activeProjectIDs = append(activeProjectIDs, projectID)
		}
	}
	if len(activeProjectIDs) > 0 {
		return activeProjectIDs[rand.Intn(len(activeProjectIDs))], nil
	}
	return "", fmt.Errorf("gemini project resolver returned no active project_id")
}
