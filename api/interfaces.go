package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/nekohy/MeowCLI/utils"
)

// Backend 后端适配器通用接口
type Backend interface {
	HandlerType() utils.HandlerType
	APIType() []utils.APIType
	// ReplaceModel 替换请求/响应 body 中的 model 字段，各后端自行实现
	ReplaceModel(body []byte, model string) []byte
}

// ChatBackend 扩展 Backend，支持 Chat API 的后端（如 Codex）
type ChatBackend interface {
	Backend
	Chat(ctx context.Context, credID string, body []byte, header http.Header, apiType utils.APIType) (*http.Response, error)
}

// GeminiNativeBackend 扩展 Backend，支持 Gemini 原生 API 的后端
type GeminiNativeBackend interface {
	Backend
	GenerateContent(ctx context.Context, credentialID string, modelName string, action string, rawQuery string, body []byte, headers http.Header) (*http.Response, error)
	GetModels(ctx context.Context, credentialID string, modelName string, rawQuery string, headers http.Header) (*http.Response, error)
}

// APIError 表示上游返回的非 2xx 响应
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("api error (status %d): %s", e.StatusCode, e.Body)
}
