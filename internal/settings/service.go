package settings

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	db "github.com/nekohy/MeowCLI/internal/store"
)

const (
	KeyAllowUserPlanTypeHeader       = "allow_user_plan_type_header"
	KeyGlobalProxy                   = "global_proxy"
	KeyCodexProxy                    = "codex_proxy"
	KeyGeminiProxy                   = "gemini_proxy"
	KeyCodexAllowUserPlanTypeHeader  = "codex_allow_user_plan_type_header"
	KeyCodexPreferredPlanTypes       = "codex_preferred_plan_types"
	KeyGeminiAllowUserPlanTypeHeader = "gemini_allow_user_plan_type_header"
	KeyGeminiPreferredPlanTypes      = "gemini_preferred_plan_types"
	KeyRefreshBeforeSeconds          = "refresh_before_seconds"
	KeyPollIntervalMilliseconds      = "poll_interval_milliseconds"
	KeyQuotaSyncIntervalSeconds      = "quota_sync_interval_seconds"
	KeyThrottleBaseSeconds           = "throttle_base_seconds"
	KeyThrottleMaxSeconds            = "throttle_max_seconds"
	KeyRelayMaxRetries               = "relay_max_retries"
	KeyLogsRetentionSeconds          = "logs_retention_seconds"
)

const (
	defaultCodexUnauthorizedCheckTimeoutSeconds       = 30
	defaultCodexImportedCheckTimeoutSeconds           = 30
	defaultCodexQuotaWindow5hSeconds            int64 = 5 * 60 * 60
	defaultCodexQuotaWindow7dSeconds            int64 = 7 * 24 * 60 * 60
	defaultLogsRetentionSeconds                       = 24 * 60 * 60
)

type Snapshot struct {
	AllowUserPlanTypeHeader       bool   `json:"allow_user_plan_type_header"`
	GlobalProxy                   string `json:"global_proxy"`
	CodexProxy                    string `json:"codex_proxy"`
	GeminiProxy                   string `json:"gemini_proxy"`
	CodexAllowUserPlanTypeHeader  bool   `json:"codex_allow_user_plan_type_header"`
	CodexPreferredPlanTypes       string `json:"codex_preferred_plan_types"`
	GeminiAllowUserPlanTypeHeader bool   `json:"gemini_allow_user_plan_type_header"`
	GeminiPreferredPlanTypes      string `json:"gemini_preferred_plan_types"`
	RefreshBeforeSeconds          int    `json:"refresh_before_seconds"`
	PollIntervalMilliseconds      int    `json:"poll_interval_milliseconds"`
	QuotaSyncIntervalSeconds      int    `json:"quota_sync_interval_seconds"`
	ThrottleBaseSeconds           int    `json:"throttle_base_seconds"`
	ThrottleMaxSeconds            int    `json:"throttle_max_seconds"`
	RelayMaxRetries               int    `json:"relay_max_retries"`
	LogsRetentionSeconds          int    `json:"logs_retention_seconds"`
}

type Provider interface {
	Snapshot() Snapshot
}

type Store interface {
	ListSettings(ctx context.Context) ([]db.Setting, error)
	UpsertSetting(ctx context.Context, arg db.UpsertSettingParams) (db.Setting, error)
}

type Service struct {
	store Store

	mu      sync.RWMutex
	current Snapshot
}

func DefaultSnapshot() Snapshot {
	return Snapshot{
		AllowUserPlanTypeHeader:       false,
		GlobalProxy:                   "",
		CodexProxy:                    "",
		GeminiProxy:                   "",
		CodexAllowUserPlanTypeHeader:  false,
		CodexPreferredPlanTypes:       "",
		GeminiAllowUserPlanTypeHeader: false,
		GeminiPreferredPlanTypes:      "",
		RefreshBeforeSeconds:          30,
		PollIntervalMilliseconds:      200,
		QuotaSyncIntervalSeconds:      15 * 60,
		ThrottleBaseSeconds:           60,
		ThrottleMaxSeconds:            30 * 60,
		RelayMaxRetries:               3,
		LogsRetentionSeconds:          defaultLogsRetentionSeconds,
	}
}

func NewService(ctx context.Context, store Store) (*Service, error) {
	svc := &Service{
		store:   store,
		current: DefaultSnapshot(),
	}
	if store == nil {
		return svc, nil
	}
	if _, err := svc.Reload(ctx); err != nil {
		return nil, err
	}
	return svc, nil
}

func (s *Service) Snapshot() Snapshot {
	if s == nil {
		return DefaultSnapshot()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

func (s *Service) Reload(ctx context.Context) (Snapshot, error) {
	if s == nil || s.store == nil {
		return DefaultSnapshot(), nil
	}

	rows, err := s.store.ListSettings(ctx)
	if err != nil {
		return Snapshot{}, err
	}

	values := make(map[string]string, len(rows))
	for _, row := range rows {
		values[row.Key] = row.Value
	}

	next := DefaultSnapshot()
	applyValues(&next, values)
	next = next.Normalize()

	s.mu.Lock()
	s.current = next
	s.mu.Unlock()
	return next, nil
}

func (s *Service) Save(ctx context.Context, next Snapshot) (Snapshot, error) {
	if s == nil {
		return Snapshot{}, nil
	}

	next = next.Normalize()
	if s.store == nil {
		s.mu.Lock()
		s.current = next
		s.mu.Unlock()
		return next, nil
	}

	for key, value := range next.asMap() {
		if _, err := s.store.UpsertSetting(ctx, db.UpsertSettingParams{
			Key:   key,
			Value: value,
		}); err != nil {
			return Snapshot{}, err
		}
	}

	s.mu.Lock()
	s.current = next
	s.mu.Unlock()
	return next, nil
}

func (s Snapshot) Normalize() Snapshot {
	defaults := DefaultSnapshot()

	s.GlobalProxy = strings.TrimSpace(s.GlobalProxy)
	s.CodexProxy = strings.TrimSpace(s.CodexProxy)
	s.GeminiProxy = strings.TrimSpace(s.GeminiProxy)
	s.CodexPreferredPlanTypes = strings.TrimSpace(s.CodexPreferredPlanTypes)
	s.GeminiPreferredPlanTypes = strings.TrimSpace(s.GeminiPreferredPlanTypes)

	if s.RefreshBeforeSeconds <= 0 {
		s.RefreshBeforeSeconds = defaults.RefreshBeforeSeconds
	}
	if s.PollIntervalMilliseconds <= 0 {
		s.PollIntervalMilliseconds = defaults.PollIntervalMilliseconds
	}
	if s.QuotaSyncIntervalSeconds <= 0 {
		s.QuotaSyncIntervalSeconds = defaults.QuotaSyncIntervalSeconds
	}
	if s.ThrottleBaseSeconds <= 0 {
		s.ThrottleBaseSeconds = defaults.ThrottleBaseSeconds
	}
	if s.ThrottleMaxSeconds < s.ThrottleBaseSeconds {
		s.ThrottleMaxSeconds = defaults.ThrottleMaxSeconds
	}
	if s.RelayMaxRetries <= 0 {
		s.RelayMaxRetries = defaults.RelayMaxRetries
	}
	if s.LogsRetentionSeconds <= 0 {
		s.LogsRetentionSeconds = defaults.LogsRetentionSeconds
	}

	return s
}

func (s Snapshot) EffectiveCodexProxy() string {
	if s.CodexProxy != "" {
		return s.CodexProxy
	}
	return s.GlobalProxy
}

func (s Snapshot) EffectiveGeminiProxy() string {
	if s.GeminiProxy != "" {
		return s.GeminiProxy
	}
	return s.GlobalProxy
}

func (s Snapshot) RefreshBefore() time.Duration {
	return time.Duration(s.RefreshBeforeSeconds) * time.Second
}

func (s Snapshot) PollInterval() time.Duration {
	return time.Duration(s.PollIntervalMilliseconds) * time.Millisecond
}

func (s Snapshot) QuotaSyncInterval() time.Duration {
	return time.Duration(s.QuotaSyncIntervalSeconds) * time.Second
}

func (s Snapshot) UnauthorizedCheckTimeout() time.Duration {
	return time.Duration(defaultCodexUnauthorizedCheckTimeoutSeconds) * time.Second
}

func (s Snapshot) ImportedCheckTimeout() time.Duration {
	return time.Duration(defaultCodexImportedCheckTimeoutSeconds) * time.Second
}

func (s Snapshot) QuotaWindow5hSeconds() int64 {
	return defaultCodexQuotaWindow5hSeconds
}

func (s Snapshot) QuotaWindow7dSeconds() int64 {
	return defaultCodexQuotaWindow7dSeconds
}

func (s Snapshot) ThrottleBase() time.Duration {
	return time.Duration(s.ThrottleBaseSeconds) * time.Second
}

func (s Snapshot) ThrottleMax() time.Duration {
	return time.Duration(s.ThrottleMaxSeconds) * time.Second
}

func (s Snapshot) LogsRetention() time.Duration {
	return time.Duration(s.LogsRetentionSeconds) * time.Second
}

func (s Snapshot) asMap() map[string]string {
	return map[string]string{
		KeyAllowUserPlanTypeHeader:       strconv.FormatBool(s.AllowUserPlanTypeHeader),
		KeyGlobalProxy:                   s.GlobalProxy,
		KeyCodexProxy:                    s.CodexProxy,
		KeyGeminiProxy:                   s.GeminiProxy,
		KeyCodexAllowUserPlanTypeHeader:  strconv.FormatBool(s.CodexAllowUserPlanTypeHeader),
		KeyCodexPreferredPlanTypes:       s.CodexPreferredPlanTypes,
		KeyGeminiAllowUserPlanTypeHeader: strconv.FormatBool(s.GeminiAllowUserPlanTypeHeader),
		KeyGeminiPreferredPlanTypes:      s.GeminiPreferredPlanTypes,
		KeyRefreshBeforeSeconds:          strconv.Itoa(s.RefreshBeforeSeconds),
		KeyPollIntervalMilliseconds:      strconv.Itoa(s.PollIntervalMilliseconds),
		KeyQuotaSyncIntervalSeconds:      strconv.Itoa(s.QuotaSyncIntervalSeconds),
		KeyThrottleBaseSeconds:           strconv.Itoa(s.ThrottleBaseSeconds),
		KeyThrottleMaxSeconds:            strconv.Itoa(s.ThrottleMaxSeconds),
		KeyRelayMaxRetries:               strconv.Itoa(s.RelayMaxRetries),
		KeyLogsRetentionSeconds:          strconv.Itoa(s.LogsRetentionSeconds),
	}
}

func applyValues(target *Snapshot, values map[string]string) {
	if value, ok := valueForKeys(values, KeyAllowUserPlanTypeHeader); ok {
		if parsed, err := strconv.ParseBool(value); err == nil {
			target.AllowUserPlanTypeHeader = parsed
		}
	}
	if value, ok := valueForKeys(values, KeyGlobalProxy); ok {
		target.GlobalProxy = strings.TrimSpace(value)
	}
	if value, ok := valueForKeys(values, KeyCodexProxy); ok {
		target.CodexProxy = strings.TrimSpace(value)
	}
	if value, ok := valueForKeys(values, KeyGeminiProxy); ok {
		target.GeminiProxy = strings.TrimSpace(value)
	}
	if value, ok := valueForKeys(values, KeyCodexAllowUserPlanTypeHeader); ok {
		if parsed, err := strconv.ParseBool(value); err == nil {
			target.CodexAllowUserPlanTypeHeader = parsed
		}
	}
	if value, ok := valueForKeys(values, KeyCodexPreferredPlanTypes); ok {
		target.CodexPreferredPlanTypes = value
	}
	if value, ok := valueForKeys(values, KeyGeminiAllowUserPlanTypeHeader); ok {
		if parsed, err := strconv.ParseBool(value); err == nil {
			target.GeminiAllowUserPlanTypeHeader = parsed
		}
	}
	if value, ok := valueForKeys(values, KeyGeminiPreferredPlanTypes); ok {
		target.GeminiPreferredPlanTypes = value
	}
	if parsed, ok := intValueForKeys(values, KeyRefreshBeforeSeconds); ok {
		target.RefreshBeforeSeconds = parsed
	}
	if parsed, ok := intValueForKeys(values, KeyPollIntervalMilliseconds); ok {
		target.PollIntervalMilliseconds = parsed
	}
	if parsed, ok := intValueForKeys(values, KeyQuotaSyncIntervalSeconds); ok {
		target.QuotaSyncIntervalSeconds = parsed
	}
	if parsed, ok := intValueForKeys(values, KeyThrottleBaseSeconds); ok {
		target.ThrottleBaseSeconds = parsed
	}
	if parsed, ok := intValueForKeys(values, KeyThrottleMaxSeconds); ok {
		target.ThrottleMaxSeconds = parsed
	}
	if parsed, ok := intValueForKeys(values, KeyRelayMaxRetries); ok {
		target.RelayMaxRetries = parsed
	}
	if parsed, ok := intValueForKeys(values, KeyLogsRetentionSeconds); ok {
		target.LogsRetentionSeconds = parsed
	}
}

func valueForKeys(values map[string]string, keys ...string) (string, bool) {
	for _, key := range keys {
		value, ok := values[key]
		if ok {
			return value, true
		}
	}
	return "", false
}

func intValueForKeys(values map[string]string, keys ...string) (int, bool) {
	value, ok := valueForKeys(values, keys...)
	if !ok {
		return 0, false
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	return parsed, true
}
