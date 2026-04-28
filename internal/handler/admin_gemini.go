package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	geminiapi "github.com/nekohy/MeowCLI/api/gemini"
	coregemini "github.com/nekohy/MeowCLI/core/gemini"
	"github.com/nekohy/MeowCLI/core/scheduling"
	db "github.com/nekohy/MeowCLI/internal/store"
	"github.com/nekohy/MeowCLI/utils"

	"github.com/gin-gonic/gin"
)

const defaultGeminiPageSize = 6

type batchCreateGeminiReq struct {
	Tokens []string `json:"tokens" binding:"required,min=1"`
}

type geminiCodeAssistPlanLoader interface {
	LoadCodeAssistPlan(ctx context.Context, accessToken string, projectID string) (string, error)
}

type geminiListItem struct {
	Handler        string                 `json:"handler"`
	ID             string                 `json:"id"`
	Status         string                 `json:"status"`
	Email          string                 `json:"email"`
	ProjectID      string                 `json:"project_id"`
	PlanType       string                 `json:"plan_type"`
	Expired        time.Time              `json:"expired"`
	Reason         string                 `json:"reason"`
	ThrottledUntil time.Time              `json:"throttled_until"`
	SyncedAt       time.Time              `json:"synced_at"`
	Pro            geminiSchedulingMetric `json:"pro"`
	Flash          geminiSchedulingMetric `json:"flash"`
	Flashlite      geminiSchedulingMetric `json:"flashlite"`
}

func (a *AdminHandler) ListGemini(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(defaultGeminiPageSize)))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = defaultGeminiPageSize
	}

	filters := geminiCredentialFiltersFromRequest(c)
	sortOptions := credentialSortOptionsFromRequest(c.Query, geminiCredentialSortKeys)
	total, rows, err := a.listGeminiCredentials(c.Request.Context(), page, pageSize, filters, sortOptions)
	if err != nil {
		writeInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"data":      rows,
	})
}

func geminiCredentialFiltersFromRequest(c *gin.Context) db.CredentialFilterParams {
	status := strings.TrimSpace(c.Query("status"))
	if status != "enabled" && status != "disabled" {
		status = ""
	}

	return db.CredentialFilterParams{
		Search:   strings.TrimSpace(c.Query("search")),
		Status:   status,
		PlanType: coregemini.NormalizePlanType(c.Query("plan_type")),
	}
}

func (a *AdminHandler) BatchCreateGemini(c *gin.Context) {
	if a == nil || a.geminiAPI == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gemini backend is unavailable"})
		return
	}

	var req batchCreateGeminiReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	job := a.importJobs.Start(context.Background(), utils.HandlerGemini, req.Tokens, func(ctx context.Context, token string) (string, error) {
		credential, err := a.upsertGeminiCredential(ctx, token)
		if err != nil {
			return "", err
		}
		return credential.ID, nil
	}, func(id string) {
		a.invalidateCredentials(utils.HandlerGemini, []string{id})
		a.syncCredentialQuotas(context.Background(), utils.HandlerGemini, []string{id})
	})

	c.JSON(http.StatusAccepted, job)
}

func (a *AdminHandler) BatchUpdateGeminiStatus(c *gin.Context) {
	var req batchUpdateStatusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	updated := make([]string, 0, len(req.IDs))
	errs := make([]batchError, 0)

	for _, id := range req.IDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}

		_, err := a.store.UpdateGeminiCLIStatus(ctx, id, req.Status, "")
		if err != nil {
			errs = append(errs, batchError{
				Input: id,
				Error: storeErrorMessage(err, "credential not found", ""),
			})
			continue
		}
		updated = append(updated, id)
	}

	a.refreshCredentials(ctx, utils.HandlerGemini, updated)
	if req.Status == "enabled" {
		a.syncCredentialQuotas(ctx, utils.HandlerGemini, updated)
	}
	c.JSON(http.StatusOK, gin.H{
		"updated": updated,
		"errors":  errs,
	})
}

func (a *AdminHandler) BatchDeleteGemini(c *gin.Context) {
	var req batchDeleteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	deleted := make([]string, 0, len(req.IDs))
	errs := make([]batchError, 0)

	for _, id := range req.IDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}

		if err := a.store.DeleteGeminiCLI(ctx, id); err != nil {
			errs = append(errs, batchError{
				Input: id,
				Error: storeErrorMessage(err, "credential not found", ""),
			})
			continue
		}
		deleted = append(deleted, id)
	}

	a.refreshCredentials(ctx, utils.HandlerGemini, deleted)
	c.JSON(http.StatusOK, gin.H{
		"deleted": deleted,
		"errors":  errs,
	})
}

func (a *AdminHandler) listGeminiCredentials(ctx context.Context, page, pageSize int, filters db.CredentialFilterParams, sortOptions credentialSortOptions) (int64, []geminiListItem, error) {
	total, err := a.store.CountGeminiCLIFiltered(ctx, filters)
	if err != nil {
		return 0, nil, err
	}

	offset := int32((page - 1) * pageSize)
	limit := int32(pageSize)
	if sortOptions.enabled() {
		offset = 0
		limit = credentialFetchLimit(total)
	}
	rows, err := a.store.ListGeminiCLIPaged(ctx, db.ListCredentialPagedParams{
		Limit:                  limit,
		Offset:                 offset,
		CredentialFilterParams: filters,
	})
	if err != nil {
		return 0, nil, err
	}

	snap := a.currentSettings()
	ws := snap.QuotaWindowGeminiSeconds()

	var ratesPro, ratesFlash, ratesFlashlite map[string]float64
	if a.logStore != nil && len(rows) > 0 {
		proSince := make([]db.ErrorRateSince, 0, len(rows))
		flashSince := make([]db.ErrorRateSince, 0, len(rows))
		flashliteSince := make([]db.ErrorRateSince, 0, len(rows))
		for _, row := range rows {
			if since := coregemini.ErrorRateSince(row.ResetPro, ws); !since.IsZero() {
				proSince = append(proSince, db.ErrorRateSince{CredentialID: row.ID, Since: since})
			}
			if since := coregemini.ErrorRateSince(row.ResetFlash, ws); !since.IsZero() {
				flashSince = append(flashSince, db.ErrorRateSince{CredentialID: row.ID, Since: since})
			}
			if since := coregemini.ErrorRateSince(row.ResetFlashlite, ws); !since.IsZero() {
				flashliteSince = append(flashliteSince, db.ErrorRateSince{CredentialID: row.ID, Since: since})
			}
		}
		ratesPro, _ = a.logStore.ErrorRatesForCredentials(ctx, string(utils.HandlerGemini), coregemini.ModelTierPro, proSince, scheduling.MinErrorRateSamples)
		ratesFlash, _ = a.logStore.ErrorRatesForCredentials(ctx, string(utils.HandlerGemini), coregemini.ModelTierFlash, flashSince, scheduling.MinErrorRateSamples)
		ratesFlashlite, _ = a.logStore.ErrorRatesForCredentials(ctx, string(utils.HandlerGemini), coregemini.ModelTierFlashLite, flashliteSince, scheduling.MinErrorRateSamples)
	}

	items := make([]geminiListItem, len(rows))
	for i, row := range rows {
		scorePro := coregemini.CalcScore(row.QuotaPro, row.QuotaFlash, row.QuotaFlashlite, row.ResetPro, row.ResetFlash, row.ResetFlashlite, coregemini.ModelTierPro, ws)
		scoreFlash := coregemini.CalcScore(row.QuotaPro, row.QuotaFlash, row.QuotaFlashlite, row.ResetPro, row.ResetFlash, row.ResetFlashlite, coregemini.ModelTierFlash, ws)
		scoreFlashlite := coregemini.CalcScore(row.QuotaPro, row.QuotaFlash, row.QuotaFlashlite, row.ResetPro, row.ResetFlash, row.ResetFlashlite, coregemini.ModelTierFlashLite, ws)

		var erPro, erFlash, erFlashlite float64
		if ratesPro != nil {
			erPro = ratesPro[row.ID]
		}
		if ratesFlash != nil {
			erFlash = ratesFlash[row.ID]
		}
		if ratesFlashlite != nil {
			erFlashlite = ratesFlashlite[row.ID]
		}
		wPro := scheduling.CalcWeight(erPro)
		wFlash := scheduling.CalcWeight(erFlash)
		wFlashlite := scheduling.CalcWeight(erFlashlite)

		items[i] = geminiListItem{
			Handler:        string(utils.HandlerGemini),
			ID:             row.ID,
			Status:         row.Status,
			Email:          row.Email,
			ProjectID:      row.ProjectID,
			PlanType:       row.PlanType,
			Expired:        row.Expired,
			Reason:         row.Reason,
			ThrottledUntil: row.ThrottledUntil,
			SyncedAt:       row.SyncedAt,
			Pro: geminiSchedulingMetric{
				Available: scorePro >= 0,
				Quota:     row.QuotaPro,
				Reset:     row.ResetPro,
				Score:     scorePro,
				Weight:    wPro,
			},
			Flash: geminiSchedulingMetric{
				Available: scoreFlash >= 0,
				Quota:     row.QuotaFlash,
				Reset:     row.ResetFlash,
				Score:     scoreFlash,
				Weight:    wFlash,
			},
			Flashlite: geminiSchedulingMetric{
				Available: scoreFlashlite >= 0,
				Quota:     row.QuotaFlashlite,
				Reset:     row.ResetFlashlite,
				Score:     scoreFlashlite,
				Weight:    wFlashlite,
			},
		}
	}
	if sortOptions.enabled() {
		sortGeminiListItems(items, sortOptions)
		items = paginateGeminiListItems(items, page, pageSize)
	}
	return total, items, nil
}

func (a *AdminHandler) upsertGeminiCredential(ctx context.Context, refreshToken string) (db.GeminiCredential, error) {
	if a == nil || a.geminiAPI == nil {
		return db.GeminiCredential{}, fmt.Errorf("gemini backend is unavailable")
	}

	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return db.GeminiCredential{}, fmt.Errorf("refresh_token is required")
	}

	tokenData, err := a.geminiAPI.RefreshAccessToken(ctx, refreshToken)
	if err != nil {
		return db.GeminiCredential{}, err
	}
	return a.upsertGeminiCredentialFromTokenData(ctx, tokenData)
}

func (a *AdminHandler) upsertGeminiCredentialFromTokenData(ctx context.Context, tokenData *geminiapi.TokenData) (db.GeminiCredential, error) {
	email, err := a.geminiAPI.FetchUserEmail(ctx, tokenData.AccessToken)
	if err != nil {
		return db.GeminiCredential{}, err
	}
	projectID, _ := a.geminiAPI.ResolveProjectID(ctx, tokenData.AccessToken)
	credentialID := coregemini.DefaultCredentialID(email, projectID)
	if credentialID == "" {
		return db.GeminiCredential{}, fmt.Errorf("gemini credential id is required")
	}
	planType := coregemini.PlanTypeFree
	if current, getErr := a.store.GetGeminiCLI(ctx, credentialID); getErr == nil {
		if projectID == "" {
			projectID = coregemini.CredentialProjectID(current.ID)
		}
		if normalized := coregemini.NormalizePlanType(current.PlanType); normalized != "" {
			planType = normalized
		}
	} else if !isStoreNotFound(getErr) {
		return db.GeminiCredential{}, getErr
	}
	if loadedPlanType, ok := loadGeminiImportPlanType(ctx, a.geminiAPI, tokenData.AccessToken, projectID); ok {
		planType = loadedPlanType
	}

	return a.store.UpsertGeminiCLI(ctx, db.UpsertGeminiCLIParams{
		ID:           credentialID,
		Status:       "enabled",
		AccessToken:  tokenData.AccessToken,
		RefreshToken: tokenData.RefreshToken,
		Expired:      tokenData.Expiry,
		Email:        strings.TrimSpace(email),
		ProjectID:    strings.TrimSpace(projectID),
		PlanType:     planType,
		Reason:       "",
	})
}

func loadGeminiImportPlanType(ctx context.Context, loader geminiCodeAssistPlanLoader, accessToken string, projectID string) (string, bool) {
	if loader == nil || strings.TrimSpace(accessToken) == "" || strings.TrimSpace(projectID) == "" {
		return "", false
	}
	loaded, err := loader.LoadCodeAssistPlan(ctx, accessToken, projectID)
	if err != nil {
		return "", false
	}
	planType := coregemini.NormalizePlanType(loaded)
	if planType == "" {
		return "", false
	}
	return planType, true
}

func isStoreNotFound(err error) bool {
	return errors.Is(err, db.ErrNotFound)
}

func (a *AdminHandler) geminiCounts(ctx context.Context) (int64, int64, bool, error) {
	total, err := a.store.CountGeminiCLI(ctx)
	if err != nil {
		return 0, 0, true, err
	}
	enabled, err := a.store.CountEnabledGeminiCLI(ctx)
	if err != nil {
		return 0, 0, true, err
	}
	return total, enabled, true, nil
}
