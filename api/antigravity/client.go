package antigravity

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
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
	c.client = utils.NewProxyHTTPClient(utils.DefaultUpstreamTimeout, func(*http.Request) (*url.URL, error) {
		return c.proxyURL()
	})
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
	query := utils.TransformSSEQuery(action, opts.RawQuery)
	if query != "" {
		targetURL += "?" + query
	}

	useCredits := c.settings != nil && c.settings.Snapshot().AntigravityUseCredits
	wrappedBody := wrapAntigravityBody(req.Body, modelName, opts.ProjectID, opts.CreditTypes, useCredits)
	httpReq, err := http.NewRequestWithContext(req.Ctx, http.MethodPost, targetURL, bytes.NewReader(wrappedBody))
	if err != nil {
		return nil, err
	}
	utils.CopyHeadersExcept(httpReq.Header, req.Headers, "Accept", "Accept-Encoding", "Content-Length", "User-Agent")
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
	Project            string                 `json:"project"`
	Request            sonic.NoCopyRawMessage `json:"request"`
	Model              string                 `json:"model"`
	UserAgent          string                 `json:"userAgent"`
	RequestType        string                 `json:"requestType"`
	RequestID          string                 `json:"requestId"`
	EnabledCreditTypes []string               `json:"enabledCreditTypes,omitempty"`
}

func wrapAntigravityBody(body []byte, modelName, projectID string, creditTypes []string, useCredits bool) []byte {
	request := body
	if nested := gjson.GetBytes(body, "request"); nested.Exists() {
		request = []byte(nested.Raw)
	}
	request = normalizeGeminiRequestForAntigravity(request, modelName)
	if strings.Contains(strings.ToLower(modelName), "claude") {
		request, _ = sjson.SetBytes(request, "toolConfig.functionCallingConfig.mode", "VALIDATED")
	}
	var enabledCreditTypes []string
	if useCredits {
		if normalized := normalizeCreditTypes(creditTypes); len(normalized) > 0 {
			enabledCreditTypes = normalized
		}
	}

	isImageModel := strings.Contains(strings.ToLower(modelName), "image")
	requestType := "agent"
	requestID := "agent-" + uuid.NewString()
	if isImageModel {
		requestType = "image_gen"
		requestID = fmt.Sprintf("image_gen/%d/%s/12", time.Now().UnixMilli(), uuid.NewString())
	}

	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		projectID = defaultProjectID
	}
	wrapped, err := sonic.Marshal(wrappedRequest{
		Project:            projectID,
		Request:            sonic.NoCopyRawMessage(request),
		Model:              modelName,
		UserAgent:          "antigravity",
		RequestType:        requestType,
		RequestID:          requestID,
		EnabledCreditTypes: enabledCreditTypes,
	})
	if err != nil {
		return body
	}
	if !isImageModel {
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
	body, _ = sjson.DeleteBytes(body, "safetySettings")
	if !strings.Contains(strings.ToLower(modelName), "claude") {
		body, _ = sjson.DeleteBytes(body, "generationConfig.maxOutputTokens")
	}
	return ensureGeminiFunctionCallIDs(body)
}

type pendingFunctionCall struct {
	id   string
	name string
	used bool
}

func ensureGeminiFunctionCallIDs(body []byte) []byte {
	root, parseErr := ast.NewParser(string(body)).Parse()
	if parseErr != 0 {
		return body
	}

	contents := root.Get("contents")
	contentsLen, err := loadNodeLen(contents)
	if err != nil {
		return body
	}

	changed := false
	pending := make([]pendingFunctionCall, 0)

	for i := 0; i < contentsLen; i++ {
		content := contents.Index(i)
		contentChanged, err := ensureContentFunctionCallIDs(content, &pending)
		if err != nil {
			return body
		}
		if contentChanged {
			if _, err := contents.SetByIndex(i, *content); err != nil {
				return body
			}
			changed = true
		}
	}

	if !changed {
		return body
	}
	if _, err := root.Set("contents", *contents); err != nil {
		return body
	}
	out, err := root.MarshalJSON()
	if err != nil {
		return body
	}
	return out
}

func ensureContentFunctionCallIDs(content *ast.Node, pending *[]pendingFunctionCall) (bool, error) {
	parts := content.Get("parts")
	partsLen, err := loadNodeLen(parts)
	if err != nil {
		return false, nil
	}

	changed := false
	for i := 0; i < partsLen; i++ {
		part := parts.Index(i)
		partChanged, err := ensurePartFunctionCallID(part, pending)
		if err != nil {
			return false, err
		}
		if !partChanged {
			continue
		}
		if _, err := parts.SetByIndex(i, *part); err != nil {
			return false, err
		}
		changed = true
	}
	if !changed {
		return false, nil
	}
	if _, err := content.Set("parts", *parts); err != nil {
		return false, err
	}
	return true, nil
}

func ensurePartFunctionCallID(part *ast.Node, pending *[]pendingFunctionCall) (bool, error) {
	if functionCall := part.Get("functionCall"); functionCall.Exists() {
		return ensureFunctionCallID(part, functionCall, pending)
	}
	if functionResponse := part.Get("functionResponse"); functionResponse.Exists() {
		return ensureFunctionResponseID(part, functionResponse, *pending)
	}
	return false, nil
}

func ensureFunctionCallID(part, functionCall *ast.Node, pending *[]pendingFunctionCall) (bool, error) {
	id := strings.TrimSpace(astString(functionCall.Get("id")))
	changed := false
	if id == "" {
		id = "toolu_" + uuid.NewString()
		if _, err := functionCall.Set("id", ast.NewString(id)); err != nil {
			return false, err
		}
		changed = true
	}
	*pending = append(*pending, pendingFunctionCall{
		id:   id,
		name: astString(functionCall.Get("name")),
	})
	if !changed {
		return false, nil
	}
	if _, err := part.Set("functionCall", *functionCall); err != nil {
		return false, err
	}
	return true, nil
}

func ensureFunctionResponseID(part, functionResponse *ast.Node, pending []pendingFunctionCall) (bool, error) {
	if id := strings.TrimSpace(astString(functionResponse.Get("id"))); id != "" {
		return false, nil
	}
	id := consumePendingFunctionCallID(pending, astString(functionResponse.Get("name")))
	if id == "" {
		return false, nil
	}
	if _, err := functionResponse.Set("id", ast.NewString(id)); err != nil {
		return false, err
	}
	if _, err := part.Set("functionResponse", *functionResponse); err != nil {
		return false, err
	}
	return true, nil
}

func consumePendingFunctionCallID(pending []pendingFunctionCall, name string) string {
	name = strings.TrimSpace(name)
	for i := range pending {
		if name == "" || pending[i].used || pending[i].name != name {
			continue
		}
		pending[i].used = true
		return pending[i].id
	}
	return ""
}

func loadNodeLen(node *ast.Node) (int, error) {
	if node == nil {
		return 0, ast.ErrNotExist
	}
	if err := node.Load(); err != nil {
		return 0, err
	}
	return node.Len()
}

func astString(node *ast.Node) string {
	if node == nil {
		return ""
	}
	value, err := node.String()
	if err != nil {
		return ""
	}
	return value
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
