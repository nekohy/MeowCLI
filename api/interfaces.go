package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/nekohy/MeowCLI/utils"
)

// BackendOpts 后端专有选项的统一接口，各后端定义自己的 Options 类型实现此接口
type BackendOpts interface {
	HandlerType() utils.HandlerType
}

// Request 携带转发请求的公共上下文，由 bridge 层构建后传递给 Backend.Chat
type Request struct {
	Ctx     context.Context
	CredID  string
	Body    []byte
	Headers http.Header
	APIType utils.APIType
	Opts    BackendOpts // 后端专有选项，如 *gemini.Options
}

// Backend 后端适配器统一接口
type Backend interface {
	HandlerType() utils.HandlerType
	APIType() []utils.APIType
	ReplaceModel(body []byte, model string) []byte
	Chat(req *Request) (*http.Response, error)
}

// APIError 表示上游返回的非 2xx 响应
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("api error (status %d): %s", e.StatusCode, e.Body)
}
