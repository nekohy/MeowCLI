package antigravity

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/nekohy/MeowCLI/api"
	"github.com/nekohy/MeowCLI/internal/settings"
	"github.com/nekohy/MeowCLI/utils"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const readBodyLimit = 4 << 20

var (
	antigravityEndpointRandomIntn = rand.Intn
	randSource                    = rand.New(rand.NewSource(time.Now().UnixNano()))
	randSourceMutex               sync.Mutex
)

type Client struct {
	client   *http.Client
	settings settings.Provider
}

func NewClient() *Client {
	c := &Client{}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = utils.DefaultUpstreamTimeout
	transport.IdleConnTimeout = utils.DefaultUpstreamTimeout
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 20
	transport.Proxy = func(*http.Request) (*url.URL, error) {
		return c.proxyURL()
	}
	c.client = &http.Client{Transport: transport}
	return c
}

func (c *Client) SetSettingsProvider(provider settings.Provider) {
	c.settings = provider
}

func (c *Client) HandlerType() utils.HandlerType {
	return utils.HandlerAntigravity
}

func (c *Client) APIType() []utils.APIType {
	return []utils.APIType{utils.APIGemini}
}

func (c *Client) ReplaceModel(body []byte, model string) []byte {
	out := body
	if response := gjson.GetBytes(out, "response"); response.Exists() && response.Type == gjson.JSON {
		out = []byte(response.Raw)
	}
	for _, path := range []string{"modelVersion", "response.modelVersion"} {
		if !gjson.GetBytes(out, path).Exists() || model == "" {
			continue
		}
		if updated, err := sjson.SetBytes(out, path, model); err == nil {
			out = updated
		}
	}
	return out
}

func (c *Client) Chat(req *api.Request) (*http.Response, error) {
	var opts Options
	if req.Opts != nil {
		opts = *req.Opts.(*Options)
	}
	modelName := strings.TrimSpace(opts.ModelName)
	action := strings.TrimSpace(opts.Action)
	if modelName == "" {
		return nil, fmt.Errorf("model is required")
	}
	if action != "generateContent" && action != "streamGenerateContent" {
		return nil, fmt.Errorf("unsupported antigravity Code Assist action %q", action)
	}

	targetURL := c.baseURL() + "/" + codeAssistVersion + ":" + action
	query := transformQuery(action, opts.RawQuery)
	if query != "" {
		targetURL += "?" + query
	}

	wrappedBody := wrapAntigravityBody(req.Body, modelName, opts.ProjectID, opts.CreditTypes, c.useCreditsWhenQuotaExhausted())
	httpReq, err := http.NewRequestWithContext(req.Ctx, http.MethodPost, targetURL, bytes.NewReader(wrappedBody))
	if err != nil {
		return nil, err
	}
	copyHeaders(httpReq.Header, req.Headers)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", antigravityUserAgent())
	if action == "streamGenerateContent" {
		httpReq.Header.Set("Accept", "text/event-stream")
	} else {
		httpReq.Header.Set("Accept", "application/json")
	}
	return c.httpClient().Do(httpReq)
}

type wrappedRequest struct {
	Project     string                 `json:"project"`
	Request     sonic.NoCopyRawMessage `json:"request"`
	Model       string                 `json:"model"`
	UserAgent   string                 `json:"userAgent"`
	RequestType string                 `json:"requestType"`
	RequestID   string                 `json:"requestId"`
}

func wrapAntigravityBody(body []byte, modelName, projectID string, creditTypes []string, useCredits bool) []byte {
	request := body
	if nested := gjson.GetBytes(body, "request"); nested.Exists() {
		request = []byte(nested.Raw)
	}
	request = normalizeGeminiRequestForAntigravity(request, modelName)
	if useCredits {
		if normalized := normalizeCreditTypes(creditTypes); len(normalized) > 0 {
			request, _ = sjson.SetBytes(request, "enabledCreditTypes", normalized)
		}
	}

	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		projectID = defaultProjectID
	}
	wrapped, err := sonic.Marshal(wrappedRequest{
		Project:     projectID,
		Request:     sonic.NoCopyRawMessage(request),
		Model:       modelName,
		UserAgent:   "antigravity",
		RequestType: requestTypeForModel(modelName),
		RequestID:   requestIDForModel(modelName),
	})
	if err != nil {
		return body
	}
	if !strings.Contains(strings.ToLower(modelName), "image") {
		wrapped, _ = sjson.SetBytes(wrapped, "request.sessionId", generateStableSessionID(request))
	}
	return wrapped
}

func normalizeCreditTypes(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func (c *Client) useCreditsWhenQuotaExhausted() bool {
	if c.settings == nil {
		return false
	}
	return c.settings.Snapshot().AntigravityUseCredits
}

func (c *Client) baseURL() string {
	rawEndpoint := settings.AntigravityAPIEndpointProd
	if c.settings == nil {
		return antigravityBaseURLProd
	}
	rawEndpoint = c.settings.Snapshot().AntigravityAPIEndpoint
	endpoints := normalizeAntigravityBaseURLs(rawEndpoint)
	if len(endpoints) == 0 {
		return antigravityBaseURLProd
	}
	if len(endpoints) == 1 {
		return endpoints[0]
	}
	return endpoints[antigravityEndpointRandomIntn(len(endpoints))]
}

func normalizeAntigravityBaseURLs(raw string) []string {
	keys := settings.NormalizeAntigravityAPIEndpointKeys(raw)
	urls := make([]string, 0, len(keys))
	for _, key := range keys {
		switch key {
		case settings.AntigravityAPIEndpointDaily:
			urls = append(urls, antigravityBaseURLDaily)
		case settings.AntigravityAPIEndpointSandboxDaily:
			urls = append(urls, antigravitySandboxBaseURLDaily)
		case settings.AntigravityAPIEndpointProd:
			urls = append(urls, antigravityBaseURLProd)
		}
	}
	if len(urls) == 0 {
		return []string{antigravityBaseURLProd}
	}
	return urls
}

func normalizeGeminiRequestForAntigravity(body []byte, modelName string) []byte {
	if systemInstruction := gjson.GetBytes(body, "system_instruction"); systemInstruction.Exists() && !gjson.GetBytes(body, "systemInstruction").Exists() {
		body, _ = sjson.SetRawBytes(body, "systemInstruction", []byte(systemInstruction.Raw))
		body, _ = sjson.DeleteBytes(body, "system_instruction")
	}
	body, _ = sjson.DeleteBytes(body, "safetySettings")
	if !strings.Contains(strings.ToLower(modelName), "claude") {
		body, _ = sjson.DeleteBytes(body, "generationConfig.maxOutputTokens")
	}
	return body
}

func requestTypeForModel(modelName string) string {
	if strings.Contains(strings.ToLower(modelName), "image") {
		return "image_gen"
	}
	return "agent"
}

func requestIDForModel(modelName string) string {
	if strings.Contains(strings.ToLower(modelName), "image") {
		return fmt.Sprintf("image_gen/%d/%s/12", time.Now().UnixMilli(), uuid.NewString())
	}
	return "agent-" + uuid.NewString()
}

func transformQuery(action, rawQuery string) string {
	query := strings.TrimSpace(rawQuery)
	if action == "streamGenerateContent" && query == "" {
		return "alt=sse"
	}
	return query
}

func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		switch http.CanonicalHeaderKey(key) {
		case "Accept", "Accept-Encoding", "Content-Length", "User-Agent":
			continue
		}
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
	if c.settings == nil {
		return nil, nil
	}
	proxy := strings.TrimSpace(c.settings.Snapshot().EffectiveAntigravityProxy())
	if proxy == "" {
		return nil, nil
	}
	return url.Parse(proxy)
}

func (c *Client) httpClient() *http.Client {
	if c.client == nil {
		return http.DefaultClient
	}
	return c.client
}

func generateStableSessionID(payload []byte) string {
	contents := gjson.GetBytes(payload, "contents")
	if contents.IsArray() {
		for _, content := range contents.Array() {
			if content.Get("role").String() != "user" {
				continue
			}
			text := content.Get("parts.0.text").String()
			if text == "" {
				continue
			}
			h := sha256.Sum256([]byte(text))
			n := int64(binary.BigEndian.Uint64(h[:8])) & 0x7FFFFFFFFFFFFFFF
			return "-" + strconv.FormatInt(n, 10)
		}
	}
	return generateSessionID()
}

func generateSessionID() string {
	randSourceMutex.Lock()
	n := randSource.Int63n(9_000_000_000_000_000_000)
	randSourceMutex.Unlock()
	return "-" + strconv.FormatInt(n, 10)
}
