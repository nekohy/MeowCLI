package utils

import (
	"fmt"
	"strings"
	"time"
)

const (
	DefaultRefreshBefore     = 30 * time.Second
	DefaultPollInterval      = 200 * time.Millisecond
	DefaultMaxRetries        = 3
	HeaderPlanTypePreference = "X-Meow-Plan-Type"
)

type HandlerType string

const (
	HandlerCodex     HandlerType = "codex"
	HandlerGeminiCLI HandlerType = "gemini-cli"
)

func ParseHandlerType(s string) (HandlerType, bool) {
	switch HandlerType(s) {
	case HandlerCodex, HandlerGeminiCLI:
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
