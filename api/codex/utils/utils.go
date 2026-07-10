package utils

import (
	"strings"
)

const (
	RefreshTokenURL = "https://auth.openai.com/oauth/token"
	ClientID        = "app_EMoamEEZ73f0CkXaXp7hrann"

	ChatURL                        = "https://chatgpt.com/backend-api/codex/responses"
	UsageURL                       = "https://chatgpt.com/backend-api/wham/usage"
	WhoamiURL                      = "https://auth.openai.com/api/accounts/v1/user-auth-credential/whoami"
	RateLimitResetCreditsURL       = "https://chatgpt.com/backend-api/wham/rate-limit-reset-credits"
	ConsumeRateLimitResetCreditURL = "https://chatgpt.com/backend-api/wham/rate-limit-reset-credits/consume"
)

// 当 UA 无法解析出 originator/version 时使用的固定回退值
const (
	fallbackOriginator = "codex_cli_rs"
	fallbackVersion    = "9.9.9"
)

// DefaultHeaders 是 Codex 客户端初始化时注入的固定请求头
var DefaultHeaders = map[string]string{
	"Accept-Language": "en-US,en;q=0.9",
}

// ParseInfoFromUA 从 Codex User-Agent 中提取 originator 与 version
// 无法解析（为空、不含斜杠、originator 为空、version 缺失或不符合格式）时，
// 返回固定回退值：originator=codex_cli_rs，version=9.9.9
func ParseInfoFromUA(ua string) (originator, version string) {
	ua = strings.TrimSpace(ua)
	if ua == "" {
		return fallbackOriginator, fallbackVersion
	}
	slash := strings.IndexByte(ua, '/')
	if slash <= 0 {
		return fallbackOriginator, fallbackVersion
	}
	originator = strings.TrimSpace(ua[:slash])
	rest := ua[slash+1:]
	space := strings.IndexAny(rest, " \t")
	if space < 0 {
		space = len(rest)
	}
	version = strings.TrimSpace(rest[:space])
	if originator == "" || !isValidCodexVersion(version) {
		return fallbackOriginator, fallbackVersion
	}
	return originator, version
}

// isValidCodexVersion 校验 version 形如 0.x.x
func isValidCodexVersion(v string) bool {
	parts := strings.Split(v, ".")
	if len(parts) != 3 || parts[0] != "0" {
		return false
	}
	return true
}

// CodexTokenData holds the result of a token refresh.
type CodexTokenData struct {
	IDToken      string
	AccessToken  string
	RefreshToken string
	Expire       string // RFC 3339
}
