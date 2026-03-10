package api

import (
	"context"
	"fmt"
	"github.com/nekohy/MeowCLI/utils"
	"net/http"
)

// Backend 后端适配器通用接口
type Backend interface {
	HandlerType() utils.HandlerType
	APIType() []utils.APIType
	Chat(ctx context.Context, credID string, body []byte, header http.Header, apiType utils.APIType) (*http.Response, error)
	// ReplaceModel 替换请求/响应 body 中的 model 字段，各后端自行实现
	ReplaceModel(body []byte, model string) []byte
}

// APIError 表示上游返回的非 2xx 响应
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("api error (status %d): %s", e.StatusCode, e.Body)
}
