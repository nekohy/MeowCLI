package utils

import (
	"fmt"
	"strings"
	"time"
)

const (
	DefaultRefreshBefore   = 30 * time.Second
	DefaultPollInterval    = 200 * time.Millisecond
	DefaultUpstreamTimeout = 120 * time.Second
	DefaultMaxRetries      = 3
)

type HandlerType string

const (
	HandlerCodex       HandlerType = "codex"
	HandlerGemini      HandlerType = "gemini"
	HandlerAntigravity HandlerType = "antigravity"
	HandlerOpenCodeGo  HandlerType = "opencode-go"
)

func ParseHandlerType(s string) (HandlerType, bool) {
	switch HandlerType(s) {
	case HandlerCodex, HandlerGemini, HandlerAntigravity, HandlerOpenCodeGo:
		return HandlerType(s), true
	default:
		return "", false
	}
}

type APIType string

const (
	APIResponses          APIType = "responses"
	APIResponsesCompact   APIType = "responses_compact"
	APICompletion         APIType = "completion"
	APIMessages           APIType = "messages"
	APIGemini             APIType = "gemini"
	APIResponsesWebsocket APIType = "responses_websocket"
)

type AccountStatus string

const (
	StatusEnabled  AccountStatus = "enabled"
	StatusDisabled AccountStatus = "disabled"
)

func ParseAccountStatus(s string) (AccountStatus, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "enabled":
		return StatusEnabled, nil
	case "disabled":
		return StatusDisabled, nil
	default:
		return "", fmt.Errorf("unknown status: %q", s)
	}
}
