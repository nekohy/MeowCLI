package gemini

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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

func (c *Client) GenerateContent(ctx context.Context, _ string, modelName string, action string, rawQuery string, body []byte, headers http.Header) (*http.Response, error) {
	modelName = strings.TrimSpace(modelName)
	action = strings.TrimSpace(action)
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
	projectID := strings.TrimSpace(headers.Get("X-Meow-Gemini-Project"))
	wrappedBody := wrapCodeAssistBody(body, modelName, projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(wrappedBody))
	if err != nil {
		return nil, err
	}
	copyHeaders(req.Header, headers)
	req.Header.Del("X-Meow-Gemini-Project")
	if strings.TrimSpace(req.Header.Get("Content-Type")) == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(req.Header.Get("Accept")) == "" {
		if action == "streamGenerateContent" {
			req.Header.Set("Accept", "text/event-stream")
		} else {
			req.Header.Set("Accept", "application/json")
		}
	}
	req.Header.Set("User-Agent", geminiCLIUserAgent(modelName))
	req.Header.Set("X-Goog-Api-Client", geminiCLIApiClientHeader)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) GetModels(ctx context.Context, _ string, modelName string, rawQuery string, headers http.Header) (*http.Response, error) {
	targetURL := generativeLanguage + "/v1beta/models"
	modelName = strings.TrimSpace(strings.TrimPrefix(modelName, "models/"))
	if modelName != "" {
		targetURL += "/" + url.PathEscape(modelName)
	}
	if strings.TrimSpace(rawQuery) != "" {
		targetURL += "?" + strings.TrimSpace(rawQuery)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	copyHeaders(req.Header, headers)
	if strings.TrimSpace(req.Header.Get("Accept")) == "" {
		req.Header.Set("Accept", "application/json")
	}
	req.Header.Set("User-Agent", geminiCLIUserAgent(""))
	req.Header.Set("X-Goog-Api-Client", geminiCLIApiClientHeader)

	resp, err := c.httpClient().Do(req)
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
