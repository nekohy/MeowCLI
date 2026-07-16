package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	geminiapi "github.com/nekohy/MeowCLI/api/gemini"
	corecodex "github.com/nekohy/MeowCLI/core/codex"
	"github.com/nekohy/MeowCLI/internal/settings"
	"github.com/nekohy/MeowCLI/utils"

	"github.com/gin-gonic/gin"
)

type settingsUpdateRequest struct {
	// Global runtime settings
	GlobalProxy                 *string `json:"global_proxy"`
	RefreshBeforeSeconds        *int    `json:"refresh_before_seconds"`
	QuotaSyncIntervalSeconds    *int    `json:"quota_sync_interval_seconds"`
	ScoreRefreshIntervalSeconds *int    `json:"score_refresh_interval_seconds"`
	ThrottleBaseSeconds         *int    `json:"throttle_base_seconds"`
	ThrottleMaxSeconds          *int    `json:"throttle_max_seconds"`
	RelayMaxRetries             *int    `json:"relay_max_retries"`
	WeightedBestCount           *int    `json:"weighted_best_count"`
	ContentAffinityMaxEntries   *int    `json:"content_affinity_max_entries"`

	// Admin/backend-only operation settings
	ImportConcurrency    *int `json:"import_concurrency"`
	LogsRetentionSeconds *int `json:"logs_retention_seconds"`
	MaxLogRows           *int `json:"max_log_rows"`

	// Codex handler settings
	CodexProxy               *string `json:"codex_proxy"`
	CodexPreferredPlanTypes  *string `json:"codex_preferred_plan_types"`
	CodexUserAgent           *string `json:"codex_user_agent"`
	CodexEnableStickySession *bool   `json:"codex_enable_sticky_session"`

	// Gemini handler settings
	GeminiProxy              *string `json:"gemini_proxy"`
	GeminiBaseURLs           *string `json:"gemini_base_urls"`
	GeminiPreferredPlanTypes *string `json:"gemini_preferred_plan_types"`

	// Antigravity handler settings
	AntigravityProxy              *string `json:"antigravity_proxy"`
	AntigravityPreferredPlanTypes *string `json:"antigravity_preferred_plan_types"`
	AntigravityAPIEndpoint        *string `json:"antigravity_api_endpoint"`
	AntigravityUseCredits         *bool   `json:"antigravity_use_credits"`
	AntigravityUserAgent          *string `json:"antigravity_user_agent"`

	// OpenCode Go handler settings
	OpenCodeGoProxy *string `json:"opencode_go_proxy"`
}

func (a *AdminHandler) GetSettings(c *gin.Context) {
	c.JSON(http.StatusOK, buildSettingsResponse(a.currentSettings()))
}

func (a *AdminHandler) UpdateSettings(c *gin.Context) {
	var req settingsUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if a.settingsSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "settings service is unavailable"})
		return
	}

	next, err := buildSettingsUpdate(a.currentSettings(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := a.store.SaveSettings(c.Request.Context(), next.SettingParams()); err != nil {
		writeInternalError(c, err)
		return
	}

	postCommitCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	reloaded, err := a.settingsSvc.Reload(postCommitCtx)
	if err != nil {
		writeInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"settings": buildSettingsResponse(reloaded),
	})
}

func buildSettingsUpdate(base settings.Snapshot, req settingsUpdateRequest) (settings.Snapshot, error) {
	next := base

	applyTrimmedStringSetting(req.GlobalProxy, &next.GlobalProxy)
	if err := applyPositiveSetting("refresh_before_seconds", req.RefreshBeforeSeconds, &next.RefreshBeforeSeconds); err != nil {
		return settings.Snapshot{}, err
	}
	if err := applyPositiveSetting("quota_sync_interval_seconds", req.QuotaSyncIntervalSeconds, &next.QuotaSyncIntervalSeconds); err != nil {
		return settings.Snapshot{}, err
	}
	if err := applyPositiveSetting("score_refresh_interval_seconds", req.ScoreRefreshIntervalSeconds, &next.ScoreRefreshIntervalSeconds); err != nil {
		return settings.Snapshot{}, err
	}
	if err := applyPositiveSetting("throttle_base_seconds", req.ThrottleBaseSeconds, &next.ThrottleBaseSeconds); err != nil {
		return settings.Snapshot{}, err
	}
	if err := applyPositiveSetting("throttle_max_seconds", req.ThrottleMaxSeconds, &next.ThrottleMaxSeconds); err != nil {
		return settings.Snapshot{}, err
	}
	if err := applyPositiveSetting("relay_max_retries", req.RelayMaxRetries, &next.RelayMaxRetries); err != nil {
		return settings.Snapshot{}, err
	}
	if err := applyPositiveSetting("weighted_best_count", req.WeightedBestCount, &next.WeightedBestCount); err != nil {
		return settings.Snapshot{}, err
	}
	if err := applyPositiveSetting("content_affinity_max_entries", req.ContentAffinityMaxEntries, &next.ContentAffinityMaxEntries); err != nil {
		return settings.Snapshot{}, err
	}

	if err := applyPositiveSetting("import_concurrency", req.ImportConcurrency, &next.ImportConcurrency); err != nil {
		return settings.Snapshot{}, err
	}
	if err := applyPositiveSetting("logs_retention_seconds", req.LogsRetentionSeconds, &next.LogsRetentionSeconds); err != nil {
		return settings.Snapshot{}, err
	}
	if err := applyPositiveSetting("max_log_rows", req.MaxLogRows, &next.MaxLogRows); err != nil {
		return settings.Snapshot{}, err
	}

	applyTrimmedStringSetting(req.CodexProxy, &next.CodexProxy)
	if req.CodexPreferredPlanTypes != nil {
		next.CodexPreferredPlanTypes = corecodex.NormalizePlanTypeList(*req.CodexPreferredPlanTypes)
	}
	applyTrimmedStringSetting(req.CodexUserAgent, &next.CodexUserAgent)
	if req.CodexEnableStickySession != nil {
		next.CodexEnableStickySession = *req.CodexEnableStickySession
	}

	applyTrimmedStringSetting(req.GeminiProxy, &next.GeminiProxy)
	if req.GeminiBaseURLs != nil {
		next.GeminiBaseURLsRaw = *req.GeminiBaseURLs
	}
	if req.GeminiPreferredPlanTypes != nil {
		next.GeminiPreferredPlanTypes = utils.NormalizeCodeAssistPlanTypeList(*req.GeminiPreferredPlanTypes)
	}

	applyTrimmedStringSetting(req.AntigravityProxy, &next.AntigravityProxy)
	if req.AntigravityPreferredPlanTypes != nil {
		next.AntigravityPreferredPlanTypes = utils.NormalizeCodeAssistPlanTypeList(*req.AntigravityPreferredPlanTypes)
	}
	if req.AntigravityAPIEndpoint != nil {
		next.AntigravityAPIEndpoint = settings.NormalizeAntigravityAPIEndpoint(*req.AntigravityAPIEndpoint)
	}
	if req.AntigravityUseCredits != nil {
		next.AntigravityUseCredits = *req.AntigravityUseCredits
	}
	applyTrimmedStringSetting(req.AntigravityUserAgent, &next.AntigravityUserAgent)

	applyTrimmedStringSetting(req.OpenCodeGoProxy, &next.OpenCodeGoProxy)

	if err := validateProxyURL(next.GlobalProxy, "global_proxy"); err != nil {
		return settings.Snapshot{}, err
	}
	if err := validateProxyURL(next.CodexProxy, "codex_proxy"); err != nil {
		return settings.Snapshot{}, err
	}
	if err := validateProxyURL(next.GeminiProxy, "gemini_proxy"); err != nil {
		return settings.Snapshot{}, err
	}
	if err := validateProxyURL(next.AntigravityProxy, "antigravity_proxy"); err != nil {
		return settings.Snapshot{}, err
	}
	if err := validateProxyURL(next.OpenCodeGoProxy, "opencode_go_proxy"); err != nil {
		return settings.Snapshot{}, err
	}
	if next.ThrottleMaxSeconds < next.ThrottleBaseSeconds {
		return settings.Snapshot{}, fmt.Errorf("throttle_max_seconds must be greater than or equal to throttle_base_seconds")
	}
	next.CodexPreferredPlanTypes = corecodex.NormalizePlanTypeList(next.CodexPreferredPlanTypes)
	next.GeminiPreferredPlanTypes = utils.NormalizeCodeAssistPlanTypeList(next.GeminiPreferredPlanTypes)
	next.AntigravityPreferredPlanTypes = utils.NormalizeCodeAssistPlanTypeList(next.AntigravityPreferredPlanTypes)

	return next.Normalize(), nil
}

func buildSettingsResponse(snapshot settings.Snapshot) gin.H {
	return gin.H{
		"global_proxy":                   snapshot.GlobalProxy,
		"refresh_before_seconds":         snapshot.RefreshBeforeSeconds,
		"quota_sync_interval_seconds":    snapshot.QuotaSyncIntervalSeconds,
		"score_refresh_interval_seconds": snapshot.ScoreRefreshIntervalSeconds,
		"throttle_base_seconds":          snapshot.ThrottleBaseSeconds,
		"throttle_max_seconds":           snapshot.ThrottleMaxSeconds,
		"relay_max_retries":              snapshot.RelayMaxRetries,
		"weighted_best_count":            snapshot.WeightedBestCount,
		"content_affinity_max_entries":   snapshot.ContentAffinityMaxEntries,

		"import_concurrency":     snapshot.ImportConcurrency,
		"logs_retention_seconds": snapshot.LogsRetentionSeconds,
		"max_log_rows":           snapshot.MaxLogRows,

		"codex_proxy":                 snapshot.CodexProxy,
		"codex_preferred_plan_types":  snapshot.CodexPreferredPlanTypes,
		"codex_user_agent":            snapshot.CodexUserAgent,
		"codex_enable_sticky_session": snapshot.CodexEnableStickySession,

		"gemini_proxy":                strings.TrimSpace(snapshot.GeminiProxy),
		"gemini_base_urls":            strings.Join(geminiapi.NormalizeCodeAssistEndpointKeys(snapshot.GeminiBaseURLsRaw), ","),
		"gemini_preferred_plan_types": snapshot.GeminiPreferredPlanTypes,

		"antigravity_proxy":                strings.TrimSpace(snapshot.AntigravityProxy),
		"antigravity_preferred_plan_types": snapshot.AntigravityPreferredPlanTypes,
		"antigravity_api_endpoint":         snapshot.AntigravityAPIEndpoint,
		"antigravity_use_credits":          snapshot.AntigravityUseCredits,
		"antigravity_user_agent":           snapshot.AntigravityUserAgent,

		"opencode_go_proxy": strings.TrimSpace(snapshot.OpenCodeGoProxy),
	}
}

func applyTrimmedStringSetting(value *string, target *string) {
	if value != nil {
		*target = strings.TrimSpace(*value)
	}
}

func applyPositiveSetting(name string, value *int, target *int) error {
	if value == nil {
		return nil
	}
	if *value <= 0 {
		return fmt.Errorf("%s must be greater than 0", name)
	}
	*target = *value
	return nil
}

func validateProxyURL(raw, field string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s must be a valid proxy URL: %w", field, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%s must include scheme and host", field)
	}
	return nil
}
