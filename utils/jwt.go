package utils

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/bytedance/sonic"
)

// JWTClaims 表示 JSON Web Token (JWT) 的声明部分
type JWTClaims struct {
	Email         string        `json:"email"`
	Exp           int64         `json:"exp"`
	CodexAuthInfo CodexAuthInfo `json:"https://api.openai.com/auth"`
	Profile       *JWTProfile   `json:"https://api.openai.com/profile"`
}

// JWTProfile 表示 access_token 中的 profile 声明（email 在此而非顶层）
type JWTProfile struct {
	Email string `json:"email"`
}

// Organizations 定义 JWT 声明中的组织详情结构
type Organizations struct {
	ID        string `json:"id"`
	IsDefault bool   `json:"is_default"`
	Role      string `json:"role"`
	Title     string `json:"title"`
}

// CodexAuthInfo 包含 Codex 特有的认证相关信息
type CodexAuthInfo struct {
	ChatgptAccountUserID string          `json:"chatgpt_account_user_id"` // 是acc_id+user_id拼接的，原生
	ChatgptPlanType      string          `json:"chatgpt_plan_type"`
	Groups               []any           `json:"groups"`
	Organizations        []Organizations `json:"organizations"`
}

// ParseJWT 解析 JWT 字符串并提取声明，不做签名验证
func ParseJWT(token string) (*JWTClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT token format: expected 3 parts, got %d", len(parts))
	}

	claimsData, err := base64URLDecode(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWT claims: %w", err)
	}

	var claims JWTClaims
	if err = sonic.Unmarshal(claimsData, &claims); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JWT claims: %w", err)
	}

	return &claims, nil
}

// base64URLDecode 解码 Base64 URL 编码的字符串，必要时补充填充字符
func base64URLDecode(data string) ([]byte, error) {
	switch len(data) % 4 {
	case 2:
		data += "=="
	case 3:
		data += "="
	}
	return base64.URLEncoding.DecodeString(data)
}

// GetAccountUserID 从 JWT 声明中提取 account-scoped user id
func (c *JWTClaims) GetAccountUserID() string {
	return strings.TrimSpace(c.CodexAuthInfo.ChatgptAccountUserID)
}

// GetCredentialID 返回默认持久化/调度使用的凭据 ID: email__account_id.
func (c *JWTClaims) GetCredentialID() string {
	email := strings.ToLower(strings.TrimSpace(c.GetEmail()))
	accountID := AccountIDFromCredentialID(c.GetAccountUserID())
	if email == "" || accountID == "" {
		return ""
	}
	return email + "__" + accountID
}

// AccountIDFromCredentialID 从默认 credential id 中提取 account id
func AccountIDFromCredentialID(credentialID string) string {
	credentialID = strings.TrimSpace(credentialID)
	if credentialID == "" {
		return ""
	}
	idx := strings.LastIndex(credentialID, "__")
	if idx < 0 || idx+2 >= len(credentialID) {
		return credentialID
	}
	return strings.TrimSpace(credentialID[idx+2:])
}

// GetPlanType 从 JWT 声明中提取 chatgpt_plan_type
func (c *JWTClaims) GetPlanType() string {
	return c.CodexAuthInfo.ChatgptPlanType
}

// GetExpiry 将 exp 声明转换为 time.Time
func (c *JWTClaims) GetExpiry() time.Time {
	if c.Exp == 0 {
		return time.Time{}
	}
	return time.Unix(c.Exp, 0)
}

// GetEmail 从顶层 email 或 profile.email 获取邮箱
func (c *JWTClaims) GetEmail() string {
	if c.Email != "" {
		return c.Email
	}
	if c.Profile != nil {
		return c.Profile.Email
	}
	return ""
}
