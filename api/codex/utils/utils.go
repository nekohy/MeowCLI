package utils

import "github.com/nekohy/MeowCLI/internal/useragent"

const (
	RefreshTokenURL = "https://auth.openai.com/oauth/token"
	ClientID        = "app_EMoamEEZ73f0CkXaXp7hrann"

	ChatURL  = "https://chatgpt.com/backend-api/codex/responses"
	UsageURL = "https://chatgpt.com/backend-api/wham/usage"
)

// DefaultHeaders 是 Codex 客户端初始化时注入的固定请求头
var DefaultHeaders = map[string]string{
	"Accept-Language": "en-US,en;q=0.9",
	"User-Agent":      useragent.CodexCLI,
}

// CodexTokenData holds the result of a token refresh.
type CodexTokenData struct {
	IDToken      string
	AccessToken  string
	RefreshToken string
	Expire       string // RFC 3339
}
