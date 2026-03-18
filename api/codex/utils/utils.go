package utils

const (
	RefreshTokenURL = "https://auth.openai.com/oauth/token"
	ClientID        = "app_EMoamEEZ73f0CkXaXp7hrann"

	ChatURL  = "https://chatgpt.com/backend-api/codex/responses"
	UsageURL = "https://chatgpt.com/backend-api/wham/usage"
)

// DefaultHeaders 是 Codex 客户端初始化时注入的固定请求头
var DefaultHeaders = map[string]string{
	"Accept-Language": "en-US,en;q=0.9",
	"User-Agent":      "codex_cli_rs/0.76.0 (Debian 13.0.0; x86_64) WindowsTerminal",
}

// CodexTokenData holds the result of a token refresh.
type CodexTokenData struct {
	IDToken      string
	AccessToken  string
	RefreshToken string
	Expire       string // RFC 3339
}
