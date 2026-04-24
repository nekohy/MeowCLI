package gemini

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/bytedance/sonic"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nekohy/MeowCLI/api"
	"github.com/nekohy/MeowCLI/internal/settings"
	"github.com/nekohy/MeowCLI/utils"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	readBodyLimit = 4 << 20
)

type Client struct {
	client   *http.Client
	settings settings.Provider
}

func NewClient() *Client {
	c := &Client{}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{
		Timeout:   15 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ResponseHeaderTimeout = 30 * time.Second
	transport.ExpectContinueTimeout = time.Second
	transport.IdleConnTimeout = 90 * time.Second
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 20
	transport.Proxy = func(*http.Request) (*url.URL, error) {
		return c.proxyURL()
	}
	c.client = &http.Client{Transport: transport}
	return c
}

func (c *Client) SetSettingsProvider(provider settings.Provider) {
	if c == nil {
		return
	}
	c.settings = provider
}

func (c *Client) HandlerType() utils.HandlerType {
	return utils.HandlerGemini
}

func (c *Client) APIType() []utils.APIType {
	return []utils.APIType{
		utils.APIGemini,
	}
}

func (c *Client) ReplaceModel(body []byte, model string) []byte {
	out := body
	if response := gjson.GetBytes(out, "response"); response.Exists() && response.Type == gjson.JSON {
		out = []byte(response.Raw)
	}
	for _, path := range []string{"modelVersion", "response.modelVersion"} {
		if !gjson.GetBytes(out, path).Exists() {
			continue
		}
		if model == "" {
			continue
		}
		if updated, err := sjson.SetBytes(out, path, model); err == nil {
			out = updated
		}
	}
	return out
}

func (c *Client) Chat(req *api.Request) (*http.Response, error) {
	ctx := req.Ctx
	body := req.Body
	headers := req.Headers

	var opts Options
	if req.Opts != nil {
		opts = *req.Opts.(*Options)
	}
	modelName := strings.TrimSpace(opts.ModelName)
	action := strings.TrimSpace(opts.Action)
	rawQuery := opts.RawQuery
	projectID := strings.TrimSpace(opts.ProjectID)
	if modelName == "" {
		return nil, fmt.Errorf("model is required")
	}
	if action == "" {
		return nil, fmt.Errorf("gemini action is required")
	}

	targetURL := fmt.Sprintf("%s/%s:%s", codeAssistEndpoint, codeAssistVersion, action)
	query := transformCodeAssistQuery(action, rawQuery)
	if query != "" {
		targetURL += "?" + query
	}
	wrappedBody := wrapCodeAssistBody(body, modelName, projectID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(wrappedBody))
	if err != nil {
		return nil, err
	}
	copyHeaders(httpReq.Header, headers)
	if strings.TrimSpace(httpReq.Header.Get("Content-Type")) == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(httpReq.Header.Get("Accept")) == "" {
		if action == "streamGenerateContent" {
			httpReq.Header.Set("Accept", "text/event-stream")
		} else {
			httpReq.Header.Set("Accept", "application/json")
		}
	}
	httpReq.Header.Set("User-Agent", geminiCLIUserAgent(modelName))
	httpReq.Header.Set("X-Goog-Api-Client", geminiCLIApiClientHeader)

	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func wrapCodeAssistBody(body []byte, modelName, projectID string) []byte {
	requestBody := body
	if gjson.GetBytes(body, "request").Exists() {
		requestBody = []byte(gjson.GetBytes(body, "request").Raw)
	}

	wrapped := []byte(`{"model":"","project":"","request":{}}`)
	if updated, err := sjson.SetBytes(wrapped, "model", modelName); err == nil {
		wrapped = updated
	}
	if updated, err := sjson.SetBytes(wrapped, "project", projectID); err == nil {
		wrapped = updated
	}
	if updated, err := sjson.SetRawBytes(wrapped, "request", requestBody); err == nil {
		wrapped = updated
	}
	return wrapped
}

func transformCodeAssistQuery(action, rawQuery string) string {
	query := strings.TrimSpace(rawQuery)
	if action == "streamGenerateContent" && query == "" {
		return "alt=sse"
	}
	return query
}

func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		if len(values) == 0 {
			continue
		}
		dst[key] = append([]string(nil), values...)
	}
}

func readLimitedBody(r io.Reader, limit int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, limit))
}

func (c *Client) proxyURL() (*url.URL, error) {
	if c == nil || c.settings == nil {
		return nil, nil
	}
	proxy := strings.TrimSpace(c.settings.Snapshot().EffectiveGeminiProxy())
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

// FetchQuota fetches the real remaining quota from the retrieveUserQuota API.
// If projectID is empty, it is resolved automatically from the access token.
func (c *Client) FetchQuota(ctx context.Context, _ string, accessToken string, projectID string) (*Quota, error) {
	token := strings.TrimSpace(accessToken)
	if token == "" {
		return nil, fmt.Errorf("fetch gemini quota: access token is empty")
	}

	pid := strings.TrimSpace(projectID)
	if pid == "" {
		resolved, err := c.ResolveProjectID(ctx, token)
		if err != nil {
			return nil, fmt.Errorf("resolve project id for gemini quota: %w", err)
		}
		pid = resolved
	}
	if pid == "" {
		return nil, fmt.Errorf("fetch gemini quota: project id is empty")
	}

	reqBody, _ := sonic.Marshal(map[string]string{"project": pid})
	quotaURL := fmt.Sprintf("%s/%s:retrieveUserQuota", codeAssistEndpoint, codeAssistVersion)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, quotaURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create gemini quota request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("fetch gemini quota: %w", err)
	}
	defer resp.Body.Close()

	body, err := readLimitedBody(resp.Body, readBodyLimit)
	if err != nil {
		return nil, fmt.Errorf("read gemini quota response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	return ParseQuotaFromBuckets(body), nil
}

type APIError = api.APIError

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
