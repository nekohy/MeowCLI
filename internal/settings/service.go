package settings

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	db "github.com/nekohy/MeowCLI/internal/store"
	"github.com/nekohy/MeowCLI/internal/useragent"
	"github.com/nekohy/MeowCLI/utils"
)

const (
	KeyGlobalProxy                 = "global_proxy"
	KeyRefreshBeforeSeconds        = "refresh_before_seconds"
	KeyQuotaSyncIntervalSeconds    = "quota_sync_interval_seconds"
	KeyScoreRefreshIntervalSeconds = "score_refresh_interval_seconds"
	KeyThrottleBaseSeconds         = "throttle_base_seconds"
	KeyThrottleMaxSeconds          = "throttle_max_seconds"
	KeyRelayMaxRetries             = "relay_max_retries"
	KeyWeightedBestCount           = "weighted_best_count"

	KeyImportConcurrency    = "import_concurrency"
	KeyLogsRetentionSeconds = "logs_retention_seconds"
	KeyMaxLogRows           = "max_log_rows"

	KeyCodexProxy              = "codex_proxy"
	KeyCodexPreferredPlanTypes = "codex_preferred_plan_types"
	KeyCodexUserAgent          = "codex_user_agent"

	KeyGeminiProxy              = "gemini_proxy"
	KeyGeminiBaseURLs           = "gemini_base_urls"
	KeyGeminiPreferredPlanTypes = "gemini_preferred_plan_types"

	KeyAntigravityProxy              = "antigravity_proxy"
	KeyAntigravityPreferredPlanTypes = "antigravity_preferred_plan_types"
	KeyAntigravityAPIEndpoint        = "antigravity_api_endpoint"
	KeyAntigravityUseCredits         = "antigravity_use_credits"
	KeyAntigravityUserAgent          = "antigravity_user_agent"
)

const (
	defaultCodexUnauthorizedCheckTimeoutSeconds       = 30
	defaultCodexImportedCheckTimeoutSeconds           = 30
	defaultCodexQuotaWindow5hSeconds            int64 = 5 * 60 * 60
	defaultCodexQuotaWindow7dSeconds            int64 = 7 * 24 * 60 * 60
	defaultPollIntervalMilliseconds                   = 200
	defaultGeminiQuotaWindowSeconds             int64 = 24 * 60 * 60
	defaultLogsRetentionSeconds                       = 24 * 60 * 60
)

const (
	defaultWeightedBestCount = 10
	defaultImportConcurrency = 4
	defaultMaxLogRows        = 100000
)

const (
	AntigravityAPIEndpointProd         = "prod"
	AntigravityAPIEndpointDaily        = "daily"
	AntigravityAPIEndpointSandboxDaily = "daily_sandbox"
	DefaultAntigravityAPIEndpoint      = AntigravityAPIEndpointSandboxDaily + "," + AntigravityAPIEndpointDaily
)

type Snapshot struct {
	GlobalProxy                 string `json:"global_proxy"`
	RefreshBeforeSeconds        int    `json:"refresh_before_seconds"`
	QuotaSyncIntervalSeconds    int    `json:"quota_sync_interval_seconds"`
	ScoreRefreshIntervalSeconds int    `json:"score_refresh_interval_seconds"`
	ThrottleBaseSeconds         int    `json:"throttle_base_seconds"`
	ThrottleMaxSeconds          int    `json:"throttle_max_seconds"`
	RelayMaxRetries             int    `json:"relay_max_retries"`
	WeightedBestCount           int    `json:"weighted_best_count"`

	ImportConcurrency    int `json:"import_concurrency"`
	LogsRetentionSeconds int `json:"logs_retention_seconds"`
	MaxLogRows           int `json:"max_log_rows"`

	CodexProxy              string `json:"codex_proxy"`
	CodexPreferredPlanTypes string `json:"codex_preferred_plan_types"`
	CodexUserAgent          string `json:"codex_user_agent"`

	GeminiProxy              string `json:"gemini_proxy"`
	GeminiBaseURLsRaw        string `json:"gemini_base_urls"`
	GeminiPreferredPlanTypes string `json:"gemini_preferred_plan_types"`

	AntigravityProxy              string `json:"antigravity_proxy"`
	AntigravityPreferredPlanTypes string `json:"antigravity_preferred_plan_types"`
	AntigravityAPIEndpoint        string `json:"antigravity_api_endpoint"`
	AntigravityUseCredits         bool   `json:"antigravity_use_credits"`
	AntigravityUserAgent          string `json:"antigravity_user_agent"`
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
		GlobalProxy:                 "",
		RefreshBeforeSeconds:        5,
		QuotaSyncIntervalSeconds:    6 * 60 * 60,
		ScoreRefreshIntervalSeconds: 60,
		ThrottleBaseSeconds:         60,
		ThrottleMaxSeconds:          30 * 60,
		RelayMaxRetries:             3,
		WeightedBestCount:           defaultWeightedBestCount,

		ImportConcurrency:    defaultImportConcurrency,
		LogsRetentionSeconds: defaultLogsRetentionSeconds,
		MaxLogRows:           defaultMaxLogRows,

		CodexProxy:              "",
		CodexPreferredPlanTypes: "",
		CodexUserAgent:          "",

		GeminiProxy:              "",
		GeminiBaseURLsRaw:        "",
		GeminiPreferredPlanTypes: "",

		AntigravityProxy:              "",
		AntigravityPreferredPlanTypes: "",
		AntigravityAPIEndpoint:        DefaultAntigravityAPIEndpoint,
		AntigravityUseCredits:         false,
		AntigravityUserAgent:          "",
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

	for _, param := range next.SettingParams() {
		if _, err := s.store.UpsertSetting(ctx, param); err != nil {
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
	if s.RefreshBeforeSeconds <= 0 {
		s.RefreshBeforeSeconds = defaults.RefreshBeforeSeconds
	}
	if s.QuotaSyncIntervalSeconds <= 0 {
		s.QuotaSyncIntervalSeconds = defaults.QuotaSyncIntervalSeconds
	}
	if s.ScoreRefreshIntervalSeconds <= 0 {
		s.ScoreRefreshIntervalSeconds = defaults.ScoreRefreshIntervalSeconds
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
	if s.WeightedBestCount <= 0 {
		s.WeightedBestCount = defaults.WeightedBestCount
	}

	if s.ImportConcurrency <= 0 {
		s.ImportConcurrency = defaults.ImportConcurrency
	}
	if s.LogsRetentionSeconds <= 0 {
		s.LogsRetentionSeconds = defaults.LogsRetentionSeconds
	}
	if s.MaxLogRows <= 0 {
		s.MaxLogRows = defaults.MaxLogRows
	}

	s.CodexProxy = strings.TrimSpace(s.CodexProxy)
	s.CodexPreferredPlanTypes = strings.TrimSpace(s.CodexPreferredPlanTypes)
	s.CodexUserAgent = strings.TrimSpace(s.CodexUserAgent)

	s.GeminiProxy = strings.TrimSpace(s.GeminiProxy)
	s.GeminiBaseURLsRaw = strings.TrimSpace(s.GeminiBaseURLsRaw)
	s.GeminiPreferredPlanTypes = strings.TrimSpace(s.GeminiPreferredPlanTypes)

	s.AntigravityProxy = strings.TrimSpace(s.AntigravityProxy)
	s.AntigravityPreferredPlanTypes = strings.TrimSpace(s.AntigravityPreferredPlanTypes)
	s.AntigravityAPIEndpoint = NormalizeAntigravityAPIEndpoint(s.AntigravityAPIEndpoint)
	s.AntigravityUserAgent = strings.TrimSpace(s.AntigravityUserAgent)

	return s
}

func NormalizeAntigravityAPIEndpoint(value string) string {
	return strings.Join(NormalizeAntigravityAPIEndpointKeys(value), ",")
}

func NormalizeAntigravityAPIEndpointKeys(value string) []string {
	return utils.NormalizeAllowedKeys(value, []string{
		AntigravityAPIEndpointProd,
		AntigravityAPIEndpointDaily,
		AntigravityAPIEndpointSandboxDaily,
	}, AntigravityAPIEndpointProd)
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

func (s Snapshot) EffectiveAntigravityProxy() string {
	if s.AntigravityProxy != "" {
		return s.AntigravityProxy
	}
	return s.GlobalProxy
}

func (s Snapshot) RefreshBefore() time.Duration {
	return time.Duration(s.RefreshBeforeSeconds) * time.Second
}

func (s Snapshot) PollInterval() time.Duration {
	return time.Duration(defaultPollIntervalMilliseconds) * time.Millisecond
}

func (s Snapshot) QuotaSyncInterval() time.Duration {
	return time.Duration(s.QuotaSyncIntervalSeconds) * time.Second
}

func (s Snapshot) ScoreRefreshInterval() time.Duration {
	return time.Duration(s.ScoreRefreshIntervalSeconds) * time.Second
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

func (s Snapshot) QuotaWindowGeminiSeconds() int64 {
	return defaultGeminiQuotaWindowSeconds
}

func (s Snapshot) QuotaWindowCodeAssistSeconds() int64 {
	return defaultGeminiQuotaWindowSeconds
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

func (s Snapshot) EffectiveCodexUserAgent() string {
	if value := strings.TrimSpace(s.CodexUserAgent); value != "" {
		return value
	}
	return useragent.CodexCLI
}

func (s Snapshot) EffectiveAntigravityUserAgent() string {
	if value := strings.TrimSpace(s.AntigravityUserAgent); value != "" {
		return value
	}
	return useragent.AntigravityIDE()
}

func (s Snapshot) SettingParams() []db.UpsertSettingParams {
	return []db.UpsertSettingParams{
		{Key: KeyGlobalProxy, Value: s.GlobalProxy},
		{Key: KeyRefreshBeforeSeconds, Value: strconv.Itoa(s.RefreshBeforeSeconds)},
		{Key: KeyQuotaSyncIntervalSeconds, Value: strconv.Itoa(s.QuotaSyncIntervalSeconds)},
		{Key: KeyScoreRefreshIntervalSeconds, Value: strconv.Itoa(s.ScoreRefreshIntervalSeconds)},
		{Key: KeyThrottleBaseSeconds, Value: strconv.Itoa(s.ThrottleBaseSeconds)},
		{Key: KeyThrottleMaxSeconds, Value: strconv.Itoa(s.ThrottleMaxSeconds)},
		{Key: KeyRelayMaxRetries, Value: strconv.Itoa(s.RelayMaxRetries)},
		{Key: KeyWeightedBestCount, Value: strconv.Itoa(s.WeightedBestCount)},

		{Key: KeyImportConcurrency, Value: strconv.Itoa(s.ImportConcurrency)},
		{Key: KeyLogsRetentionSeconds, Value: strconv.Itoa(s.LogsRetentionSeconds)},
		{Key: KeyMaxLogRows, Value: strconv.Itoa(s.MaxLogRows)},

		{Key: KeyCodexProxy, Value: s.CodexProxy},
		{Key: KeyCodexPreferredPlanTypes, Value: s.CodexPreferredPlanTypes},
		{Key: KeyCodexUserAgent, Value: s.CodexUserAgent},

		{Key: KeyGeminiProxy, Value: s.GeminiProxy},
		{Key: KeyGeminiBaseURLs, Value: s.GeminiBaseURLsRaw},
		{Key: KeyGeminiPreferredPlanTypes, Value: s.GeminiPreferredPlanTypes},

		{Key: KeyAntigravityProxy, Value: s.AntigravityProxy},
		{Key: KeyAntigravityPreferredPlanTypes, Value: s.AntigravityPreferredPlanTypes},
		{Key: KeyAntigravityAPIEndpoint, Value: s.AntigravityAPIEndpoint},
		{Key: KeyAntigravityUseCredits, Value: strconv.FormatBool(s.AntigravityUseCredits)},
		{Key: KeyAntigravityUserAgent, Value: s.AntigravityUserAgent},
	}
}

func applyValues(target *Snapshot, values map[string]string) {
	if value, ok := valueForKeys(values, KeyGlobalProxy); ok {
		target.GlobalProxy = strings.TrimSpace(value)
	}
	if parsed, ok := intValueForKeys(values, KeyRefreshBeforeSeconds); ok {
		target.RefreshBeforeSeconds = parsed
	}
	if parsed, ok := intValueForKeys(values, KeyQuotaSyncIntervalSeconds); ok {
		target.QuotaSyncIntervalSeconds = parsed
	}
	if parsed, ok := intValueForKeys(values, KeyScoreRefreshIntervalSeconds); ok {
		target.ScoreRefreshIntervalSeconds = parsed
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
	if parsed, ok := intValueForKeys(values, KeyWeightedBestCount); ok {
		target.WeightedBestCount = parsed
	}

	if parsed, ok := intValueForKeys(values, KeyImportConcurrency); ok {
		target.ImportConcurrency = parsed
	}
	if parsed, ok := intValueForKeys(values, KeyLogsRetentionSeconds); ok {
		target.LogsRetentionSeconds = parsed
	}
	if parsed, ok := intValueForKeys(values, KeyMaxLogRows); ok {
		target.MaxLogRows = parsed
	}

	if value, ok := valueForKeys(values, KeyCodexProxy); ok {
		target.CodexProxy = strings.TrimSpace(value)
	}
	if value, ok := valueForKeys(values, KeyCodexPreferredPlanTypes); ok {
		target.CodexPreferredPlanTypes = value
	}
	if value, ok := valueForKeys(values, KeyCodexUserAgent); ok {
		target.CodexUserAgent = strings.TrimSpace(value)
	}

	if value, ok := valueForKeys(values, KeyGeminiProxy); ok {
		target.GeminiProxy = strings.TrimSpace(value)
	}
	if value, ok := valueForKeys(values, KeyGeminiBaseURLs); ok {
		target.GeminiBaseURLsRaw = value
	}
	if value, ok := valueForKeys(values, KeyGeminiPreferredPlanTypes); ok {
		target.GeminiPreferredPlanTypes = value
	}

	if value, ok := valueForKeys(values, KeyAntigravityProxy); ok {
		target.AntigravityProxy = strings.TrimSpace(value)
	}
	if value, ok := valueForKeys(values, KeyAntigravityPreferredPlanTypes); ok {
		target.AntigravityPreferredPlanTypes = value
	}
	if value, ok := valueForKeys(values, KeyAntigravityAPIEndpoint); ok {
		target.AntigravityAPIEndpoint = NormalizeAntigravityAPIEndpoint(value)
	}
	if value, ok := valueForKeys(values, KeyAntigravityUseCredits); ok {
		if parsed, err := strconv.ParseBool(value); err == nil {
			target.AntigravityUseCredits = parsed
		}
	}
	if value, ok := valueForKeys(values, KeyAntigravityUserAgent); ok {
		target.AntigravityUserAgent = strings.TrimSpace(value)
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
