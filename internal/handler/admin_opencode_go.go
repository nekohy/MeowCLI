package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	coreopencodego "github.com/nekohy/MeowCLI/core/opencodego"
	"github.com/nekohy/MeowCLI/core/scheduling"
	db "github.com/nekohy/MeowCLI/internal/store"
	"github.com/nekohy/MeowCLI/utils"

	"github.com/gin-gonic/gin"
)

const defaultOpenCodeGoPageSize = 6

var openCodeGoThrottleStatusTiers = stringSet(coreopencodego.ModelTierDefault)

type openCodeGoListItem struct {
	Handler      string                `json:"handler"`
	ID           string                `json:"id"`
	Email        string                `json:"email"`
	Status       []string              `json:"status"`
	Reason       string                `json:"reason"`
	WorkspaceID  string                `json:"workspace_id"`
	SyncedAt     time.Time             `json:"synced_at"`
	Quota        codexSchedulingMetric `json:"quota"`
	RewardsCount int                   `json:"rewards_count"`
}

func (a *AdminHandler) ListOpenCodeGo(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(defaultOpenCodeGoPageSize)))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = defaultOpenCodeGoPageSize
	}

	filters := db.CredentialFilterParams{
		Search:       strings.TrimSpace(c.Query("search")),
		Statuses:     credentialStatusesFromRequest(c, openCodeGoThrottleStatusTiers),
		UnsyncedOnly: c.Query("unsynced") == "true",
	}
	sortOptions := credentialSortOptionsFromRequest(c.Query, openCodeGoCredentialSortCapabilities)
	total, err := a.store.CountOpenCodeGoFiltered(c.Request.Context(), filters)
	if err != nil {
		writeInternalError(c, err)
		return
	}
	offset := int32((page - 1) * pageSize)
	limit := int32(pageSize)
	if sortOptions.enabled() {
		offset = 0
		limit = credentialFetchLimit(total)
	}
	rows, err := a.store.ListOpenCodeGoPaged(c.Request.Context(), db.ListCredentialPagedParams{
		Limit:                  limit,
		Offset:                 offset,
		CredentialFilterParams: filters,
	})
	if err != nil {
		writeInternalError(c, err)
		return
	}
	items := a.serializeOpenCodeGoRows(c.Request.Context(), rows)
	if sortOptions.enabled() {
		sortOpenCodeGoListItems(items, sortOptions)
		items = paginateOpenCodeGoListItems(items, page, pageSize)
	}
	c.JSON(http.StatusOK, gin.H{
		"total": total, "page": page, "page_size": pageSize,
		"data": items,
	})
}

func (a *AdminHandler) BatchCreateOpenCodeGo(c *gin.Context) {
	var req batchCreateCodexReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	inputs := make([]string, 0, len(req.Tokens))
	inputValues := make(map[string]string, len(req.Tokens))
	for index, input := range req.Tokens {
		if strings.TrimSpace(input) == "" {
			continue
		}
		label := fmt.Sprintf("第 %d 行", index+1)
		inputs = append(inputs, label)
		inputValues[label] = input
	}
	job := a.importJobs.Start(context.Background(), utils.HandlerOpenCodeGo, inputs, func(ctx context.Context, label string) (string, error) {
		ids, err := a.processOpenCodeGoCredentials(ctx, inputValues[label])
		if err != nil {
			return "", err
		}
		a.invalidateCredentials(utils.HandlerOpenCodeGo, ids)
		a.syncCredentialQuotas(context.Background(), utils.HandlerOpenCodeGo, ids)
		return "", nil
	}, nil)
	c.JSON(http.StatusAccepted, importJobStartResponse(job))
}

func (a *AdminHandler) processOpenCodeGoCredentials(ctx context.Context, input string) ([]string, error) {
	authCookie, err := parseOpenCodeGoAuth(input)
	if err != nil {
		return nil, err
	}
	if a.openCodeGoAPI == nil {
		return nil, errors.New("opencode go API key discovery is unavailable")
	}
	discovered, err := a.openCodeGoAPI.DiscoverAPIKeys(ctx, authCookie)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(discovered))
	for _, item := range discovered {
		id, err := a.upsertOpenCodeGoCredential(ctx, item.APIKey, authCookie, item.Email, item.WorkspaceID)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (a *AdminHandler) upsertOpenCodeGoCredential(ctx context.Context, apiKey, authCookie, email, workspaceID string) (string, error) {
	id := utils.DefaultOpenCodeGoCredentialID(email, workspaceID)
	if id == "" {
		return "", errors.New("opencode go email or workspace id is invalid")
	}

	_, err := a.store.UpsertOpenCodeGo(ctx, db.UpsertOpenCodeGoParams{
		ID: id, Status: string(utils.StatusEnabled), APIKey: apiKey,
		AuthCookie: authCookie,
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

func parseOpenCodeGoAuth(input string) (string, error) {
	authCookie := strings.TrimSpace(input)
	if !strings.HasPrefix(authCookie, "Fe26") {
		return "", errors.New("opencode go auth value must begin with Fe26")
	}
	return authCookie, nil
}

func (a *AdminHandler) BatchUpdateOpenCodeGoStatus(c *gin.Context) {
	a.batchUpdateCredentialStatus(c, utils.HandlerOpenCodeGo, func(ctx context.Context, id, status, reason string) error {
		_, err := a.store.UpdateOpenCodeGoStatus(ctx, id, status, reason)
		return err
	})
}

func (a *AdminHandler) BatchDeleteOpenCodeGo(c *gin.Context) {
	a.batchDeleteCredentials(c, utils.HandlerOpenCodeGo, a.store.DeleteOpenCodeGo)
}

func (a *AdminHandler) ListOpenCodeGoReferralRewards(c *gin.Context) {
	credentialID := strings.TrimSpace(c.Query("credential_id"))
	if credentialID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "credential_id is required"})
		return
	}
	if a.openCodeGoAPI == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "opencode go admin API is unavailable"})
		return
	}
	row, workspaceID, err := a.openCodeGoAdminCredential(c.Request.Context(), credentialID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	rewards, err := a.openCodeGoAPI.ListReferralRewards(c.Request.Context(), workspaceID, row.AuthCookie)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rewards)
}

func (a *AdminHandler) ApplyOpenCodeGoReferralReward(c *gin.Context) {
	var req struct {
		CredentialID string `json:"credential_id" binding:"required"`
		ReferralID   string `json:"referral_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "credential_id and referral_id are required"})
		return
	}
	if a.openCodeGoAPI == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "opencode go admin API is unavailable"})
		return
	}
	if a.credRefresh == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "opencode go quota refresher is unavailable"})
		return
	}
	credentialID := strings.TrimSpace(req.CredentialID)
	referralID := strings.TrimSpace(req.ReferralID)
	row, workspaceID, err := a.openCodeGoAdminCredential(c.Request.Context(), credentialID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	rewards, err := a.openCodeGoAPI.ListReferralRewards(c.Request.Context(), workspaceID, row.AuthCookie)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	available := false
	for _, reward := range rewards.Rewards {
		if reward.ID == referralID && strings.EqualFold(reward.Status, "available") {
			available = true
			break
		}
	}
	if !available {
		c.JSON(http.StatusBadRequest, gin.H{"error": "opencode go referral reward is not available"})
		return
	}
	if err := a.openCodeGoAPI.ApplyReferralReward(c.Request.Context(), workspaceID, referralID, row.AuthCookie); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if err := a.credRefresh.RefreshQuota(c.Request.Context(), utils.HandlerOpenCodeGo, credentialID); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   fmt.Sprintf("referral reward was applied, but quota refresh failed: %v", err),
			"applied": true,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "quota_refreshed": true})
}

func (a *AdminHandler) openCodeGoAdminCredential(ctx context.Context, credentialID string) (db.OpenCodeGoCredential, string, error) {
	workspaceID := utils.OpenCodeGoWorkspaceIDFromCredentialID(credentialID)
	if workspaceID == "" {
		return db.OpenCodeGoCredential{}, "", errors.New("opencode go credential id is invalid")
	}
	row, err := a.store.GetOpenCodeGo(ctx, credentialID)
	if err != nil {
		return db.OpenCodeGoCredential{}, "", err
	}
	if strings.TrimSpace(row.AuthCookie) == "" {
		return db.OpenCodeGoCredential{}, "", errors.New("this credential has no auth_cookie; referral rewards are unavailable")
	}
	return row, workspaceID, nil
}

func (a *AdminHandler) serializeOpenCodeGoRows(ctx context.Context, rows []db.ListOpenCodeGoRow) []openCodeGoListItem {
	var rates map[string]float64
	if a.logStore != nil && len(rows) > 0 {
		since := make([]db.ErrorRateSince, 0, len(rows))
		for _, row := range rows {
			if start := coreopencodego.ErrorRateSince(row.Reset5h, row.Reset7d, row.Reset1mo); !start.IsZero() {
				since = append(since, db.ErrorRateSince{CredentialID: row.ID, Since: start})
			}
		}
		rates, _ = a.logStore.ErrorRatesForCredentials(ctx, string(utils.HandlerOpenCodeGo), coreopencodego.ModelTierDefault, since, scheduling.MinErrorRateSamples)
	}
	items := make([]openCodeGoListItem, 0, len(rows))
	for _, row := range rows {
		score := coreopencodego.CalcScore(row.Quota5h, row.Quota7d, row.Quota1mo, row.Reset5h, row.Reset7d, row.Reset1mo)
		if row.Reset5h.IsZero() && row.Reset7d.IsZero() && row.Reset1mo.IsZero() {
			score = 1
		}
		weight := 1.0
		if rates != nil {
			weight = scheduling.CalcWeight(rates[row.ID])
		}
		item := openCodeGoListItem{
			Handler: string(utils.HandlerOpenCodeGo), ID: row.ID,
			Email:       utils.OpenCodeGoEmailFromCredentialID(row.ID),
			Status:      credentialStatusList(row.Status, throttleStatusDeadline{Tier: coreopencodego.ModelTierDefault, Deadline: row.ThrottledUntil}),
			Reason:      row.Reason,
			WorkspaceID: utils.OpenCodeGoWorkspaceIDFromCredentialID(row.ID),
			SyncedAt:    row.SyncedAt,
			Quota: codexSchedulingMetric{
				Available: score >= 0, Quota5h: row.Quota5h, Quota7d: row.Quota7d, Quota1mo: row.Quota1mo,
				Reset5h: row.Reset5h, Reset7d: row.Reset7d, Reset1mo: row.Reset1mo,
				ThrottledUntil: activeThrottleDeadline(row.ThrottledUntil), Score: score, Weight: weight,
			},
			RewardsCount: row.RewardsCount,
		}
		items = append(items, item)
	}
	return items
}
