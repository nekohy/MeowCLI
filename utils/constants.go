package utils

import (
	"fmt"
	"strings"
	"time"
)

const (
	DefaultRefreshBefore     = 30 * time.Second
	DefaultPollInterval      = 200 * time.Millisecond
	DefaultUpstreamTimeout   = 120 * time.Second
	DefaultMaxRetries        = 3
	HeaderPlanTypePreference = "X-Meow-Plan-Type"
)

type HandlerType string

const (
	HandlerCodex  HandlerType = "codex"
	HandlerGemini HandlerType = "gemini"
)

func ParseHandlerType(s string) (HandlerType, bool) {
	switch HandlerType(s) {
	case HandlerCodex, HandlerGemini:
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
	StatusEnabled   AccountStatus = "enabled"
	StatusDisabled  AccountStatus = "disabled"
	StatusThrottled AccountStatus = "throttled"
)

func ParseAccountStatus(s string) (AccountStatus, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "enabled":
		return StatusEnabled, nil
	case "disabled":
		return StatusDisabled, nil
	case "throttled":
		return StatusThrottled, nil
	default:
		return "", fmt.Errorf("unknown status: %q", s)
	}
}
