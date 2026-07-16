package opencodego

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
	"github.com/nekohy/MeowCLI/api"
	"github.com/nekohy/MeowCLI/internal/settings"
	"github.com/nekohy/MeowCLI/utils"
)

const (
	BaseURL             = "https://opencode.ai/zen/go"
	ChatCompletionsPath = "/v1/chat/completions"

	dashboardOrigin = "https://opencode.ai"
	dashboardBase   = dashboardOrigin + "/workspace"
	serverBase      = dashboardOrigin + "/_server"

	workspaceServerID = "def39973159c7f0483d8793a822b8dbb10d067e12c65455fcb4608459ba0234f"
	listKeysServerID  = "c22cd964237ba79f2f9b95faa2a14b804f870d1bab49279463379cc6a0fd0c85"
	applyRewardID     = "f386778c1b78eade3e6acff87c9284e02fcd86826463c080526143c4fe8fff23"

	readBodyLimit = 4 << 20
	userAgent     = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Gecko/20100101 Firefox/148.0"
)

var (
	workspaceIDPattern    = regexp.MustCompile(`wrk_[A-Za-z0-9]+`)
	workspaceEntryPattern = regexp.MustCompile(`(?s)\bid\s*:\s*"(wrk_[^"]+)"[^{}]*?\bname\s*:\s*"([^"]*)"`)
	apiKeyObjectPattern   = regexp.MustCompile(`(?s)\{[^{}]*\bkey\s*:\s*"sk-[A-Za-z0-9]+"[^{}]*\}`)
	apiKeyPattern         = regexp.MustCompile(`\bkey\s*:\s*"(sk-[A-Za-z0-9]+)"`)
	apiKeyEmailPattern    = regexp.MustCompile(`\bemail\s*:\s*"([^"]+)"`)
	rewardPattern         = regexp.MustCompile(`(?s)id\s*:\s*"(ref_[^"]+)".*?source\s*:\s*"([^"]+)".*?status\s*:\s*"([^"]+)".*?email\s*:\s*(?:"([^"]*)"|null).*?amount\s*:\s*([0-9]+).*?timeCreated\s*:\s*(?:\$R\[[0-9]+\]\s*=\s*)?new Date\("([^"]+)"\)`)
)

type Client struct {
	client        *http.Client
	settings      settings.Provider
	apiBaseURL    string
	dashboardBase string
	serverBase    string
}

type Quota struct {
	Quota5h           float64          `json:"quota_5h"`
	Quota7d           float64          `json:"quota_7d"`
	Quota1mo          float64          `json:"quota_1mo"`
	Reset5h           time.Time        `json:"reset_5h"`
	Reset7d           time.Time        `json:"reset_7d"`
	Reset1mo          time.Time        `json:"reset_1mo"`
	AvailableRewards  []ReferralReward `json:"available_rewards"`
	RewardsTotalCents int              `json:"rewards_total_cents"`
}

type ReferralReward struct {
	ID          string    `json:"id"`
	Source      string    `json:"source"`
	Status      string    `json:"status"`
	Email       string    `json:"email,omitempty"`
	AmountCents int       `json:"amount_cents"`
	CreatedAt   time.Time `json:"created_at"`
}

type ReferralRewards struct {
	Rewards    []ReferralReward `json:"rewards"`
	TotalCents int              `json:"total_cents"`
}

type DiscoveredAPIKey struct {
	APIKey      string
	Email       string
	WorkspaceID string
}

func NewClient() *Client {
	c := &Client{
		apiBaseURL:    BaseURL,
		dashboardBase: dashboardBase,
		serverBase:    serverBase,
	}
	c.client = utils.NewProxyHTTPClient(utils.DefaultUpstreamTimeout, func(*http.Request) (*url.URL, error) {
		return c.proxyURL()
	})
	c.client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return c
}

func (c *Client) SetSettingsProvider(provider settings.Provider) {
	if c == nil {
		return
	}
	c.settings = provider
}

func (c *Client) HandlerType() utils.HandlerType {
	return utils.HandlerOpenCodeGo
}

func (c *Client) APIType() []utils.APIType {
	return []utils.APIType{utils.APICompletion}
}

func (c *Client) ReplaceModel(body []byte, model string) []byte {
	var root ast.Node
	if err := root.UnmarshalJSON(body); err != nil {
		return body
	}
	if !root.Get("model").Exists() {
		return body
	}
	if _, err := root.Set("model", ast.NewString(model)); err != nil {
		return body
	}
	updated, err := root.MarshalJSON()
	if err != nil {
		return body
	}
	return updated
}

func (c *Client) PrepareRequest(root *ast.Node, apiType utils.APIType, _ api.BackendOpts) (api.PreparedRequest, error) {
	if apiType != utils.APICompletion {
		return api.PreparedRequest{}, fmt.Errorf("unsupported opencode go api type %q", apiType)
	}
	if root == nil {
		return api.PreparedRequest{}, fmt.Errorf("opencode go request JSON is nil")
	}
	return api.PreparedRequest{Root: root, PayloadAPIType: apiType}, nil
}

func (c *Client) Chat(req *api.Request) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("opencode go request is nil")
	}
	if req.APIType != utils.APICompletion {
		return nil, fmt.Errorf("unsupported opencode go api type %q", req.APIType)
	}

	httpReq, err := http.NewRequestWithContext(req.Ctx, http.MethodPost, strings.TrimRight(c.apiURL(), "/")+ChatCompletionsPath, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}
	utils.CopyHeadersExcept(httpReq.Header, req.Headers, "Accept-Encoding", "Content-Length", "Cookie", "Host", "Proxy-Authorization")
	if strings.TrimSpace(httpReq.Header.Get("Content-Type")) == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(httpReq.Header.Get("Accept")) == "" {
		httpReq.Header.Set("Accept", "application/json")
	}
	return c.httpClient().Do(httpReq)
}

func (c *Client) FetchQuota(ctx context.Context, workspaceID, authCookie string) (*Quota, error) {
	cookieHeader := BuildCookieHeader(authCookie)
	if cookieHeader == "" {
		return nil, errors.New("opencode go auth value must begin with Fe26")
	}
	workspaceID = ExtractWorkspaceID(workspaceID)
	if workspaceID == "" {
		return nil, errors.New("opencode go workspace id is invalid")
	}

	target := strings.TrimRight(c.dashboardURL(), "/") + "/" + url.PathEscape(workspaceID) + "/go"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Cookie", cookieHeader)
	httpReq.Header.Set("User-Agent", userAgent)
	httpReq.Header.Set("Accept", "text/html, application/xhtml+xml")

	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("fetch opencode go dashboard: %w", err)
	}
	defer resp.Body.Close()
	body, err := utils.ReadLimitedBody(resp.Body, readBodyLimit)
	if err != nil {
		return nil, fmt.Errorf("read opencode go dashboard: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, &api.APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return nil, fmt.Errorf("opencode go dashboard redirected (HTTP %d), check workspace_id and auth cookie", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &api.APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	quota, err := ParseDashboardHTML(string(body), time.Now().UTC())
	if err != nil {
		return nil, err
	}
	return quota, nil
}

func (c *Client) ListReferralRewards(ctx context.Context, workspaceID, authCookie string) (*ReferralRewards, error) {
	quota, err := c.FetchQuota(ctx, workspaceID, authCookie)
	if err != nil {
		return nil, err
	}
	return &ReferralRewards{
		Rewards:    quota.AvailableRewards,
		TotalCents: quota.RewardsTotalCents,
	}, nil
}

func (c *Client) DiscoverAPIKeys(ctx context.Context, authCookie string) ([]DiscoveredAPIKey, error) {
	refs, err := c.fetchWorkspaceRefs(ctx, authCookie)
	if err != nil {
		return nil, err
	}
	discovered := make([]DiscoveredAPIKey, 0)
	for _, ref := range refs {
		key, err := c.FirstWorkspaceAPIKey(ctx, ref.ID, authCookie)
		if err != nil {
			return nil, fmt.Errorf("read first opencode go API key for workspace %s: %w", ref.ID, err)
		}
		discovered = append(discovered, key)
	}
	if len(discovered) == 0 {
		return nil, errors.New("no complete opencode go API keys are visible to the current auth session")
	}
	return discovered, nil
}

func (c *Client) FirstWorkspaceAPIKey(ctx context.Context, workspaceID, authCookie string) (DiscoveredAPIKey, error) {
	workspaceID = ExtractWorkspaceID(workspaceID)
	if workspaceID == "" {
		return DiscoveredAPIKey{}, errors.New("opencode go workspace id is required")
	}
	cookieHeader := BuildCookieHeader(authCookie)
	if cookieHeader == "" {
		return DiscoveredAPIKey{}, errors.New("opencode go auth value must begin with Fe26")
	}
	args, err := marshalServerQueryStringArg(workspaceID)
	if err != nil {
		return DiscoveredAPIKey{}, err
	}
	target := c.serverURL() + "?id=" + url.QueryEscape(listKeysServerID) + "&args=" + url.QueryEscape(string(args))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return DiscoveredAPIKey{}, err
	}
	referer := strings.TrimRight(c.dashboardURL(), "/") + "/" + url.PathEscape(workspaceID) + "/keys"
	setServerHeaders(httpReq.Header, listKeysServerID, cookieHeader, referer)

	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
		return DiscoveredAPIKey{}, fmt.Errorf("query opencode go API keys: %w", err)
	}
	defer resp.Body.Close()
	body, err := utils.ReadLimitedBody(resp.Body, readBodyLimit)
	if err != nil {
		return DiscoveredAPIKey{}, err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return DiscoveredAPIKey{}, fmt.Errorf("opencode go authentication failed (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return DiscoveredAPIKey{}, fmt.Errorf("opencode go API key query returned HTTP %d", resp.StatusCode)
	}
	if resp.Header.Get("X-Error") != "" {
		return DiscoveredAPIKey{}, errors.New("opencode go API key query failed")
	}

	object := apiKeyObjectPattern.Find(body)
	if len(object) == 0 {
		return DiscoveredAPIKey{}, errors.New("opencode go workspace has no complete API key")
	}
	keyMatch := apiKeyPattern.FindSubmatch(object)
	emailMatch := apiKeyEmailPattern.FindSubmatch(object)
	if len(keyMatch) != 2 {
		return DiscoveredAPIKey{}, errors.New("could not parse first opencode go API key")
	}
	if len(emailMatch) != 2 {
		return DiscoveredAPIKey{}, errors.New("first opencode go API key is missing email")
	}
	email := utils.NormalizeOpenCodeGoEmail(string(emailMatch[1]))
	if email == "" {
		return DiscoveredAPIKey{}, errors.New("first opencode go API key has invalid email")
	}
	return DiscoveredAPIKey{APIKey: string(keyMatch[1]), Email: email, WorkspaceID: workspaceID}, nil
}

func (c *Client) ApplyReferralReward(ctx context.Context, workspaceID, referralID, authCookie string) error {
	workspaceID = ExtractWorkspaceID(workspaceID)
	referralID = strings.TrimSpace(referralID)
	if workspaceID == "" {
		return errors.New("opencode go workspace id is required")
	}
	if !strings.HasPrefix(referralID, "ref_") {
		return errors.New("opencode go referral id is invalid")
	}
	body, err := sonic.Marshal([]string{workspaceID, referralID})
	if err != nil {
		return err
	}
	return c.doServerAction(ctx, applyRewardID, authCookie, "application/json", body, workspaceID)
}

type workspaceRef struct {
	ID   string
	Name string
}

type serverQueryArgs struct {
	Value serverQueryTuple `json:"t"`
	Flags int              `json:"f"`
	Meta  []any            `json:"m"`
}

type serverQueryTuple struct {
	Type   int                 `json:"t"`
	Index  int                 `json:"i"`
	Length int                 `json:"l"`
	Args   []serverQueryString `json:"a"`
	Offset int                 `json:"o"`
}

type serverQueryString struct {
	Type  int    `json:"t"`
	Value string `json:"s"`
}

func marshalServerQueryStringArg(value string) ([]byte, error) {
	return sonic.Marshal(serverQueryArgs{
		Value: serverQueryTuple{
			Type: 9, Index: 0, Length: 1, Offset: 0,
			Args: []serverQueryString{{Type: 1, Value: value}},
		},
		Flags: 31,
		Meta:  make([]any, 0),
	})
}

func (c *Client) fetchWorkspaceRefs(ctx context.Context, authCookie string) ([]workspaceRef, error) {
	cookieHeader := BuildCookieHeader(authCookie)
	if cookieHeader == "" {
		return nil, errors.New("opencode go auth value must begin with Fe26")
	}
	target := c.serverURL() + "?id=" + url.QueryEscape(workspaceServerID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	setServerHeaders(httpReq.Header, workspaceServerID, cookieHeader, dashboardOrigin)

	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("query opencode go workspaces: %w", err)
	}
	defer resp.Body.Close()
	body, err := utils.ReadLimitedBody(resp.Body, readBodyLimit)
	if err != nil {
		return nil, fmt.Errorf("read opencode go workspace response: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("opencode go authentication failed (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("opencode go workspace query returned HTTP %d", resp.StatusCode)
	}
	if err := serverFunctionResponseError(resp); err != nil {
		return nil, err
	}

	refs := parseWorkspaceRefs(body)
	if len(refs) == 0 {
		return nil, errors.New("opencode go workspace response contained no workspace id")
	}
	return refs, nil
}

func parseWorkspaceRefs(body []byte) []workspaceRef {
	matches := workspaceEntryPattern.FindAllSubmatch(body, -1)
	refs := make([]workspaceRef, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if len(match) != 3 {
			continue
		}
		id := string(match[1])
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		refs = append(refs, workspaceRef{ID: id, Name: strings.TrimSpace(string(match[2]))})
	}
	return refs
}

func serverFunctionResponseError(resp *http.Response) error {
	if resp == nil || strings.TrimSpace(resp.Header.Get("X-Error")) == "" {
		return nil
	}
	message := strings.ToLower(strings.TrimSpace(resp.Header.Get("X-Error")))
	if strings.Contains(message, "actor") && strings.Contains(message, "public") {
		return errors.New("opencode go auth session was rejected by upstream; copy a fresh auth cookie from the signed-in opencode.ai browser session")
	}
	return errors.New("opencode go workspace query failed upstream")
}

func (c *Client) doServerAction(ctx context.Context, actionID, authCookie, contentType string, body []byte, workspaceID string) error {
	cookieHeader := BuildCookieHeader(authCookie)
	if cookieHeader == "" {
		return errors.New("opencode go auth value must begin with Fe26")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.serverURL(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	referer := strings.TrimRight(c.dashboardURL(), "/") + "/" + url.PathEscape(workspaceID) + "/go"
	setServerHeaders(httpReq.Header, actionID, cookieHeader, referer)
	httpReq.Header.Set("Content-Type", contentType)

	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
		return fmt.Errorf("call opencode go action: %w", err)
	}
	defer resp.Body.Close()
	responseBody, err := utils.ReadLimitedBody(resp.Body, readBodyLimit)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("opencode go authentication failed (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("opencode go action returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	if resp.Header.Get("X-Error") != "" {
		return fmt.Errorf("opencode go action failed: %s", strings.TrimSpace(string(responseBody)))
	}
	return nil
}

func ParseDashboardHTML(document string, now time.Time) (*Quota, error) {
	rolling, rollingOK := parseUsageWindow(document, "rollingUsage", now)
	weekly, weeklyOK := parseUsageWindow(document, "weeklyUsage", now)
	monthly, monthlyOK := parseUsageWindow(document, "monthlyUsage", now)
	if !rollingOK && !weeklyOK && !monthlyOK {
		return nil, errors.New("could not parse opencode go quota data from dashboard html")
	}

	quota := &Quota{Quota5h: 1, Quota7d: 1, Quota1mo: 1}
	if rollingOK {
		quota.Quota5h, quota.Reset5h = rolling.remaining, rolling.resetAt
	}
	if weeklyOK {
		quota.Quota7d, quota.Reset7d = weekly.remaining, weekly.resetAt
	}
	if monthlyOK {
		quota.Quota1mo, quota.Reset1mo = monthly.remaining, monthly.resetAt
	}
	for _, match := range rewardPattern.FindAllStringSubmatch(document, -1) {
		if len(match) < 7 || !strings.EqualFold(match[3], "available") {
			continue
		}
		amount, err := strconv.Atoi(match[5])
		if err != nil || amount <= 0 {
			continue
		}
		createdAt, _ := time.Parse(time.RFC3339Nano, match[6])
		quota.AvailableRewards = append(quota.AvailableRewards, ReferralReward{
			ID: match[1], Source: match[2], Status: match[3], Email: html.UnescapeString(match[4]),
			AmountCents: amount, CreatedAt: createdAt,
		})
		quota.RewardsTotalCents += amount
	}
	return quota, nil
}

type usageWindow struct {
	remaining float64
	resetAt   time.Time
}

func parseUsageWindow(document, field string, now time.Time) (usageWindow, bool) {
	quotedField := regexp.QuoteMeta(field)
	patterns := []*regexp.Regexp{
		regexp.MustCompile(quotedField + `\s*:\s*(?:\$R\[[0-9]+\]\s*=\s*)?\{[^}]*usagePercent\s*:\s*(-?[0-9]+(?:\.[0-9]+)?)[^}]*resetInSec\s*:\s*(-?[0-9]+(?:\.[0-9]+)?)[^}]*\}`),
		regexp.MustCompile(quotedField + `\s*:\s*(?:\$R\[[0-9]+\]\s*=\s*)?\{[^}]*resetInSec\s*:\s*(-?[0-9]+(?:\.[0-9]+)?)[^}]*usagePercent\s*:\s*(-?[0-9]+(?:\.[0-9]+)?)[^}]*\}`),
	}
	for i, pattern := range patterns {
		match := pattern.FindStringSubmatch(document)
		if len(match) != 3 {
			continue
		}
		first, err1 := strconv.ParseFloat(match[1], 64)
		second, err2 := strconv.ParseFloat(match[2], 64)
		if err1 != nil || err2 != nil {
			continue
		}
		usedPercent, resetSeconds := first, second
		if i == 1 {
			usedPercent, resetSeconds = second, first
		}
		usedPercent = max(0, min(100, usedPercent))
		return usageWindow{
			remaining: (100 - usedPercent) / 100,
			resetAt:   now.Add(time.Duration(resetSeconds * float64(time.Second))),
		}, true
	}
	return usageWindow{}, false
}

func BuildCookieHeader(raw string) string {
	auth := strings.TrimSpace(raw)
	if !strings.HasPrefix(auth, "Fe26") {
		return ""
	}
	return "auth=" + auth
}

func ExtractWorkspaceID(raw string) string {
	match := workspaceIDPattern.FindString(strings.TrimSpace(raw))
	return match
}

func setServerHeaders(headers http.Header, serverID, cookieHeader, referer string) {
	headers.Set("Cookie", cookieHeader)
	headers.Set("X-Server-Id", serverID)
	headers.Set("X-Server-Instance", "server-fn:"+strconv.FormatInt(time.Now().UTC().UnixNano(), 10))
	headers.Set("User-Agent", userAgent)
	headers.Set("Origin", dashboardOrigin)
	headers.Set("Referer", referer)
	headers.Set("Accept", "text/javascript, application/json;q=0.9, */*;q=0.8")
}

func (c *Client) proxyURL() (*url.URL, error) {
	if c == nil || c.settings == nil {
		return nil, nil
	}
	proxy := strings.TrimSpace(c.settings.Snapshot().EffectiveOpenCodeGoProxy())
	if proxy == "" {
		return nil, nil
	}
	return url.Parse(proxy)
}

func (c *Client) httpClient() *http.Client {
	if c == nil || c.client == nil {
		return http.DefaultClient
	}
	return c.client
}

func (c *Client) apiURL() string {
	if c == nil || strings.TrimSpace(c.apiBaseURL) == "" {
		return BaseURL
	}
	return c.apiBaseURL
}

func (c *Client) dashboardURL() string {
	if c == nil || strings.TrimSpace(c.dashboardBase) == "" {
		return dashboardBase
	}
	return c.dashboardBase
}

func (c *Client) serverURL() string {
	if c == nil || strings.TrimSpace(c.serverBase) == "" {
		return serverBase
	}
	return c.serverBase
}
