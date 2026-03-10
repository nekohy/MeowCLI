package handler

import (
	"context"
	"errors"
	"fmt"
	"github.com/nekohy/MeowCLI/internal/settings"
	db "github.com/nekohy/MeowCLI/internal/store"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

type settingsUpdateRequest struct {
	GlobalProxy              *string `json:"global_proxy"`
	CodexProxy               *string `json:"codex_proxy"`
	CodexDeleteFreeAccounts  *bool   `json:"codex_delete_free_accounts"`
	RefreshBeforeSeconds     *int    `json:"refresh_before_seconds"`
	PollIntervalMilliseconds *int    `json:"poll_interval_milliseconds"`
	QuotaSyncIntervalSeconds *int    `json:"quota_sync_interval_seconds"`
	ThrottleBaseSeconds      *int    `json:"throttle_base_seconds"`
	ThrottleMaxSeconds       *int    `json:"throttle_max_seconds"`
	RelayMaxRetries          *int    `json:"relay_max_retries"`
	LogsRetentionSeconds     *int    `json:"logs_retention_seconds"`
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

	saved, err := a.settingsSvc.Save(c.Request.Context(), next)
	if err != nil {
		writeInternalError(c, err)
		return
	}

	deleted, err := a.deleteFreeCodexAccounts(c.Request.Context(), saved)
	if err != nil {
		writeInternalError(c, err)
		return
	}
	if len(deleted) > 0 {
		a.refreshCredentials(c.Request.Context())
	}

	c.JSON(http.StatusOK, gin.H{
		"settings":              saved,
		"deleted_free_accounts": deleted,
	})
}

func buildSettingsUpdate(base settings.Snapshot, req settingsUpdateRequest) (settings.Snapshot, error) {
	next := base

	if req.GlobalProxy != nil {
		next.GlobalProxy = strings.TrimSpace(*req.GlobalProxy)
	}
	if req.CodexProxy != nil {
		next.CodexProxy = strings.TrimSpace(*req.CodexProxy)
	}
	if req.CodexDeleteFreeAccounts != nil {
		next.CodexDeleteFreeAccounts = *req.CodexDeleteFreeAccounts
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

func (a *AdminHandler) deleteFreeCodexAccounts(ctx context.Context, snapshot settings.Snapshot) ([]string, error) {
	if !snapshot.CodexDeleteFreeAccounts {
		return nil, nil
	}

	rows, err := a.store.ListCodex(ctx)
	if err != nil {
		return nil, err
	}

	deleted := make([]string, 0)
	for _, row := range rows {
		if !strings.EqualFold(strings.TrimSpace(row.PlanType), "free") {
			continue
		}
		if err := a.store.DeleteCodex(ctx, row.ID); err != nil {
			if errors.Is(err, db.ErrNotFound) {
				continue
			}
			return deleted, err
		}
		deleted = append(deleted, row.ID)
	}
	return deleted, nil
}
