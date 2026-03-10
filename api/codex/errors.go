package codex

import (
	"errors"
	"github.com/nekohy/MeowCLI/api"
)

// ParseAPIError 解包 resty 的 APIError，便于上层按状态码分类处理
func ParseAPIError(err error) (statusCode int, body string, ok bool) {
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		return 0, "", false
	}
	return apiErr.StatusCode, apiErr.Body, true
}
