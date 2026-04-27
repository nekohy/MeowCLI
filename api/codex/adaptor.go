package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/nekohy/MeowCLI/api"
	completionconvert "github.com/nekohy/MeowCLI/api/codex/convert/completion"
	codexutils "github.com/nekohy/MeowCLI/api/codex/utils"
	"github.com/nekohy/MeowCLI/internal/settings"
	"github.com/nekohy/MeowCLI/utils"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
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
	transport.ResponseHeaderTimeout = utils.DefaultUpstreamTimeout
	transport.IdleConnTimeout = utils.DefaultUpstreamTimeout
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
		utils.APICompletion,
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
	if req.APIType == utils.APICompletion {
		model := gjson.GetBytes(body, "model").String()
		converted, err := completionconvert.Convert(model, body, gjson.GetBytes(body, "stream").Bool())
		if err != nil {
			return nil, err
		}
		body = converted
	}
	// Codex upstream only supports stream processing reliably; preserve the
	// client's requested mode locally and force the upstream request to stream.
	body, clientStream := prepareCodexResponsesRequestBody(body)

	r := c.client.R().
		SetContext(ctx).
		SetDoNotParseResponse(true).
		SetBody(body)

	for k, vs := range headers {
		if http.CanonicalHeaderKey(k) == "Accept" {
			continue
		}
		if len(vs) > 0 {
			r.SetHeader(k, vs[0])
		}
	}
	r.SetHeader("Accept", "text/event-stream")

	resp, err := r.Post(codexutils.ChatURL)
	if err != nil {
		return nil, err
	}

	raw := resp.RawResponse

	// Upstream is always requested as SSE. Ensure downstream stream handling can
	// detect it even if the upstream omits Content-Type.
	if clientStream && raw != nil && raw.Header.Get("Content-Type") == "" {
		raw.Header.Set("Content-Type", "text/event-stream")
	}

	if raw != nil && raw.StatusCode >= 200 && raw.StatusCode < 300 {
		switch {
		case req.APIType == utils.APICompletion:
			if clientStream {
				raw.Body = completionconvert.NewStreamReadCloser(ctx, raw.Body, gjson.GetBytes(body, "model").String())
			} else {
				translated, translateErr := completionconvert.TranslateNonStream(raw)
				if translateErr != nil {
					_ = raw.Body.Close()
					return nil, translateErr
				}
				raw = translated
			}
		case req.APIType == utils.APIResponses:
			if !clientStream {
				translated, translateErr := translateCodexStreamResponseToNonStream(raw)
				if translateErr != nil {
					_ = raw.Body.Close()
					return nil, translateErr
				}
				raw = translated
			}
		}
	}

	if c.OnQuota != nil {
		rl := codexutils.ParseRateLimit(raw.Header)
		if rl.HasQuotaWindows() {
			q := rl.ToQuotaForTier(resolveQuotaTierFromBody(body))
			c.OnQuota(ctx, credentialID, &q)
		}
	}

	return raw, nil
}

func prepareCodexResponsesRequestBody(body []byte) ([]byte, bool) {
	var root ast.Node
	if err := root.UnmarshalJSON(body); err != nil {
		return []byte{}, false
	}

	clientStream, err := root.Get("stream").Bool()
	if err != nil {
		return []byte{}, false
	}

	if _, err := root.Set("stream", ast.NewBool(true)); err != nil {
		return []byte{}, false
	}
	if _, err := root.Set("store", ast.NewBool(false)); err != nil {
		return []byte{}, false
	}
	if !root.Get("instructions").Exists() {
		if _, err := root.Set("instructions", ast.NewString("")); err != nil {
			return []byte{}, false
		}
	}
	if _, err := root.Unset("temperature"); err != nil {
		return []byte{}, false
	}

	out, err := root.MarshalJSON()
	if err != nil {
		return []byte{}, false
	}

	return out, clientStream
}
func resolveQuotaTierFromBody(body []byte) string {
	model := strings.ToLower(gjson.GetBytes(body, "model").String())
	if strings.Contains(model, "spark") {
		return "spark"
	}
	return "default"
}

func translateCodexStreamResponseToNonStream(resp *http.Response) (*http.Response, error) {
	if resp == nil || resp.Body == nil {
		return resp, nil
	}
	body, err := readCodexCompletedResponse(resp.Body)
	if err != nil {
		return nil, err
	}
	_ = resp.Body.Close()

	cloned := resp.Header.Clone()
	cloned.Set("Content-Type", "application/json")
	cloned.Del("Content-Length")
	resp.Header = cloned
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	return resp, nil
}

func readCodexCompletedResponse(r io.Reader) ([]byte, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 20_971_520)

	var outputItems []string
	for scanner.Scan() {
		payload := bytes.TrimSpace(scanner.Bytes())
		switch gjson.GetBytes(payload, "type").String() {
		case "response.output_item.done":
			if item := gjson.GetBytes(payload, "item"); item.Exists() && item.Type == gjson.JSON {
				outputItems = append(outputItems, item.Raw)
			}
		case "response.completed":
			response := gjson.GetBytes(payload, "response").Raw
			// 回填完整响应
			items := make([]json.RawMessage, 0, len(outputItems))
			for _, item := range outputItems {
				items = append(items, json.RawMessage(item))
			}

			if rawOutput, err := sonic.Marshal(items); err == nil {
				patched, err := sjson.SetRawBytes([]byte(response), "output", rawOutput)
				if err == nil {
					response = string(patched)
				}
			}
			return []byte(response), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return nil, io.ErrUnexpectedEOF
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
