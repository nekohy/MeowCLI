package codex

import (
	"bytes"
	"context"
	"github.com/nekohy/MeowCLI/api"
	codexutils "github.com/nekohy/MeowCLI/api/codex/utils"
	"github.com/nekohy/MeowCLI/internal/settings"
	"github.com/nekohy/MeowCLI/utils"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/go-resty/resty/v2"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type Client struct {
	client   *resty.Client
	settings settings.Provider
	// OnQuota 响应头配额回调，Responses 每次收到响应后自动解析并调用
	OnQuota func(ctx context.Context, credentialID string, q *codexutils.Quota)
}

func NewClient() *Client {
	c := &Client{}
	rc := resty.New()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{
		Timeout:   defaultDialTimeout,
		KeepAlive: 30 * time.Second,
	}).DialContext
	transport.TLSHandshakeTimeout = defaultTLSHandshakeTimeout
	transport.ResponseHeaderTimeout = defaultResponseHeaderTimeout
	transport.ExpectContinueTimeout = defaultExpectContinueTimeout
	transport.IdleConnTimeout = defaultIdleConnTimeout
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 20
	transport.Proxy = func(*http.Request) (*url.URL, error) {
		return c.proxyURL()
	}
	rc.SetTransport(transport)
	rc.JSONMarshal = sonic.Marshal
	rc.JSONUnmarshal = sonic.Unmarshal
	rc.OnBeforeRequest(func(_ *resty.Client, req *resty.Request) error {
		for k, v := range codexutils.DefaultHeaders {
			req.SetHeader(k, v)
		}
		return nil
	})
	rc.OnAfterResponse(func(_ *resty.Client, resp *resty.Response) error {
		// Responses 使用 DoNotParseResponse，body 未读取，跳过
		if resp.Body() == nil {
			return nil
		}
		// 2xx 为成功响应，不作为错误处理
		if resp.StatusCode() >= 200 && resp.StatusCode() < 300 {
			return nil
		}
		return &api.APIError{
			StatusCode: resp.StatusCode(),
			Body:       string(resp.Body()),
		}
	})
	c.client = rc
	return c
}

func (c *Client) SetSettingsProvider(provider settings.Provider) {
	if c == nil {
		return
	}
	c.settings = provider
}

func (c *Client) HandlerType() utils.HandlerType {
	return utils.HandlerCodex
}

func (c *Client) APIType() []utils.APIType {
	return []utils.APIType{
		utils.APIResponses,
		utils.APIResponsesCompact,
	}
}

func (c *Client) ReplaceModel(body []byte, model string) []byte {
	out := body
	if gjson.GetBytes(out, "model").Exists() {
		if updated, err := sjson.SetBytes(out, "model", model); err == nil {
			out = updated
		}
	}
	if gjson.GetBytes(out, "response.model").Exists() {
		updated, err := sjson.SetBytes(out, "response.model", model)
		if err != nil {
			return out
		}
		return updated
	}
	return out
}

// Chat 向上游 Codex API 发送请求，返回原始 *http.Response（调用方负责关闭 Body）
// headers 应已包含认证头（Authorization, Chatgpt-Account-Id 等），由调用方负责组合
// OnBeforeRequest 最终强制 User-Agent 等默认头
func (c *Client) Chat(req *api.Request) (*http.Response, error) {
	ctx := req.Ctx
	body := req.Body
	credentialID := req.CredID
	headers := req.Headers
	// 预处理 body：读取 stream 标志，补充缺失的 instructions 字段
	isStream := gjson.GetBytes(body, "stream").Bool()

	if !gjson.GetBytes(body, "instructions").Exists() {
		if modified, err := sjson.SetBytes(body, "instructions", ""); err == nil {
			body = modified
		}
	}

	r := c.client.R().
		SetContext(ctx).
		SetDoNotParseResponse(true).
		SetBody(body)

	for k, vs := range headers {
		if len(vs) > 0 {
			r.SetHeader(k, vs[0])
		}
	}

	resp, err := r.Post(codexutils.ChatURL)
	if err != nil {
		return nil, err
	}

	raw := resp.RawResponse

	// stream 请求时确保响应带有 SSE Content-Type，便于下游正确识别流式响应
	if isStream && raw.Header.Get("Content-Type") == "" {
		raw.Header.Set("Content-Type", "text/event-stream")
	}

	if !isStream && raw != nil && raw.StatusCode >= 200 && raw.StatusCode < 300 {
		translated, translateErr := translateCodexNonStreamResponse(raw)
		if translateErr != nil {
			_ = raw.Body.Close()
			return nil, translateErr
		}
		raw = translated
	}

	if c.OnQuota != nil {
		rl := codexutils.ParseRateLimit(raw.Header)
		q := rl.ToQuota()
		c.OnQuota(ctx, credentialID, &q)
	}

	return raw, nil
}

func translateCodexNonStreamResponse(resp *http.Response) (*http.Response, error) {
	if resp == nil || resp.Body == nil {
		return resp, nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	_ = resp.Body.Close()

	translated := body
	root := gjson.ParseBytes(body)
	if root.Get("type").String() == "response.completed" {
		if response := root.Get("response"); response.Exists() && response.Type == gjson.JSON {
			translated = []byte(response.Raw)
		}
	}

	cloned := resp.Header.Clone()
	cloned.Set("Content-Type", "application/json")
	cloned.Del("Content-Length")
	resp.Header = cloned
	resp.Body = io.NopCloser(bytes.NewReader(translated))
	resp.ContentLength = int64(len(translated))
	return resp, nil
}

func (c *Client) proxyURL() (*url.URL, error) {
	if c == nil || c.settings == nil {
		return nil, nil
	}
	proxy := strings.TrimSpace(c.settings.Snapshot().EffectiveCodexProxy())
	if proxy == "" {
		return nil, nil
	}
	return url.Parse(proxy)
}
