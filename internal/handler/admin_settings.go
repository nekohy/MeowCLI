package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	corecodex "github.com/nekohy/MeowCLI/core/codex"
	"github.com/nekohy/MeowCLI/internal/settings"
	db "github.com/nekohy/MeowCLI/internal/store"

	"github.com/gin-gonic/gin"
)

type settingsUpdateRequest struct {
	AllowUserPlanTypeHeader      *bool   `json:"allow_user_plan_type_header"`
	GlobalProxy                  *string `json:"global_proxy"`
	CodexProxy                   *string `json:"codex_proxy"`
	CodexAllowUserPlanTypeHeader *bool   `json:"codex_allow_user_plan_type_header"`
	CodexPreferredPlanTypes      *string `json:"codex_preferred_plan_types"`
	RefreshBeforeSeconds         *int    `json:"refresh_before_seconds"`
	PollIntervalMilliseconds     *int    `json:"poll_interval_milliseconds"`
	QuotaSyncIntervalSeconds     *int    `json:"quota_sync_interval_seconds"`
	ThrottleBaseSeconds          *int    `json:"throttle_base_seconds"`
	ThrottleMaxSeconds           *int    `json:"throttle_max_seconds"`
	RelayMaxRetries              *int    `json:"relay_max_retries"`
	LogsRetentionSeconds         *int    `json:"logs_retention_seconds"`
}

func (a *AdminHandler) GetSettings(c *gin.Context) {
	c.JSON(http.StatusOK, a.currentSettings())
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

	settingsParams := snapshotToSettingParams(next)
	if err := a.store.SaveSettings(c.Request.Context(), settingsParams); err != nil {
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

	c.JSON(http.StatusOK, gin.H{"settings": reloaded})
}

func buildSettingsUpdate(base settings.Snapshot, req settingsUpdateRequest) (settings.Snapshot, error) {
	next := base

	if req.AllowUserPlanTypeHeader != nil {
		next.AllowUserPlanTypeHeader = *req.AllowUserPlanTypeHeader
	}
	if req.GlobalProxy != nil {
		next.GlobalProxy = strings.TrimSpace(*req.GlobalProxy)
	}
	if req.CodexProxy != nil {
		next.CodexProxy = strings.TrimSpace(*req.CodexProxy)
	}
	if req.CodexAllowUserPlanTypeHeader != nil {
		next.CodexAllowUserPlanTypeHeader = *req.CodexAllowUserPlanTypeHeader
	}
	if req.CodexPreferredPlanTypes != nil {
		next.CodexPreferredPlanTypes = corecodex.NormalizePlanTypeList(*req.CodexPreferredPlanTypes)
	}
	if err := applyPositiveSetting("refresh_before_seconds", req.RefreshBeforeSeconds, &next.RefreshBeforeSeconds); err != nil {
		return settings.Snapshot{}, err
	}
	if err := applyPositiveSetting("poll_interval_milliseconds", req.PollIntervalMilliseconds, &next.PollIntervalMilliseconds); err != nil {
		return settings.Snapshot{}, err
	}
	if err := applyPositiveSetting("quota_sync_interval_seconds", req.QuotaSyncIntervalSeconds, &next.QuotaSyncIntervalSeconds); err != nil {
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
	if err := applyPositiveSetting("logs_retention_seconds", req.LogsRetentionSeconds, &next.LogsRetentionSeconds); err != nil {
		return settings.Snapshot{}, err
	}

	if err := validateProxyURL(next.GlobalProxy, "global_proxy"); err != nil {
		return settings.Snapshot{}, err
	}
	if err := validateProxyURL(next.CodexProxy, "codex_proxy"); err != nil {
		return settings.Snapshot{}, err
	}
	if next.ThrottleMaxSeconds < next.ThrottleBaseSeconds {
		return settings.Snapshot{}, fmt.Errorf("throttle_max_seconds must be greater than or equal to throttle_base_seconds")
	}
	next.CodexPreferredPlanTypes = corecodex.NormalizePlanTypeList(next.CodexPreferredPlanTypes)

	return next.Normalize(), nil
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

func snapshotToSettingParams(snapshot settings.Snapshot) []db.UpsertSettingParams {
	return []db.UpsertSettingParams{
		{Key: settings.KeyAllowUserPlanTypeHeader, Value: fmt.Sprintf("%t", snapshot.AllowUserPlanTypeHeader)},
		{Key: settings.KeyGlobalProxy, Value: snapshot.GlobalProxy},
		{Key: settings.KeyCodexProxy, Value: snapshot.CodexProxy},
		{Key: settings.KeyCodexAllowUserPlanTypeHeader, Value: fmt.Sprintf("%t", snapshot.CodexAllowUserPlanTypeHeader)},
		{Key: settings.KeyCodexPreferredPlanTypes, Value: snapshot.CodexPreferredPlanTypes},
		{Key: settings.KeyRefreshBeforeSeconds, Value: fmt.Sprintf("%d", snapshot.RefreshBeforeSeconds)},
		{Key: settings.KeyPollIntervalMilliseconds, Value: fmt.Sprintf("%d", snapshot.PollIntervalMilliseconds)},
		{Key: settings.KeyQuotaSyncIntervalSeconds, Value: fmt.Sprintf("%d", snapshot.QuotaSyncIntervalSeconds)},
		{Key: settings.KeyThrottleBaseSeconds, Value: fmt.Sprintf("%d", snapshot.ThrottleBaseSeconds)},
		{Key: settings.KeyThrottleMaxSeconds, Value: fmt.Sprintf("%d", snapshot.ThrottleMaxSeconds)},
		{Key: settings.KeyRelayMaxRetries, Value: fmt.Sprintf("%d", snapshot.RelayMaxRetries)},
		{Key: settings.KeyLogsRetentionSeconds, Value: fmt.Sprintf("%d", snapshot.LogsRetentionSeconds)},
	}
}
