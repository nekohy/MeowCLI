package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/bytedance/sonic/ast"
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
	Stream  bool
	Opts    BackendOpts // 后端专有选项，如 *gemini.Options
}

// PreparedRequest 是完成协议转换和刚需请求体补全后的请求
// Root 是最终内容请求的唯一表示，内容亲和直接读取它；统一 relay 在指纹完成后
// 才序列化一次。PayloadAPIType 描述 Root 当前采用的请求载荷协议
type PreparedRequest struct {
	Root           *ast.Node
	PayloadAPIType utils.APIType
}

// Backend 后端适配器统一接口
type Backend interface {
	HandlerType() utils.HandlerType
	APIType() []utils.APIType
	ReplaceModel(body []byte, model string) []byte
	PrepareRequest(root *ast.Node, apiType utils.APIType, opts BackendOpts) (PreparedRequest, error)
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
