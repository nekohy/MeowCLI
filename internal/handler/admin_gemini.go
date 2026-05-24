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

const defaultCredentialPageSize = 6

type batchCreateCredentialReq struct {
	Tokens []string `json:"tokens" binding:"required,min=1"`
}

type geminiCodeAssistPlanLoader interface {
	LoadCodeAssistPlan(ctx context.Context, accessToken string, projectID string) (string, error)
}

type geminiListItem struct {
	Handler   string                `json:"handler"`
	ID        string                `json:"id"`
	Status    []string              `json:"status"`
	Email     string                `json:"email"`
	ProjectID string                `json:"project_id"`
	PlanType  string                `json:"plan_type"`
	Expired   time.Time             `json:"expired"`
	Reason    string                `json:"reason"`
	SyncedAt  time.Time             `json:"synced_at"`
	Pro       quotaSchedulingMetric `json:"pro"`
	Flash     quotaSchedulingMetric `json:"flash"`
	Flashlite quotaSchedulingMetric `json:"flashlite"`
}

func (a *AdminHandler) ListGemini(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(defaultCredentialPageSize)))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = defaultCredentialPageSize
	}

	filters := geminiCredentialFiltersFromRequest(c)
	sortOptions := credentialSortOptionsFromRequest(c.Query, geminiCredentialSortKeys)
	planTypes, err := a.store.ListGeminiCLIPlanTypes(c.Request.Context(), credentialPlanTypeFilter(filters))
	if err != nil {
		writeInternalError(c, err)
		return
	}
	total, rows, err := a.listGeminiCredentials(c.Request.Context(), page, pageSize, filters, sortOptions)
	if err != nil {
		writeInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total":      total,
		"page":       page,
		"page_size":  pageSize,
		"plan_types": planTypes,
		"data":       rows,
	})
}

func geminiCredentialFiltersFromRequest(c *gin.Context) db.CredentialFilterParams {
	return db.CredentialFilterParams{
		Search:   strings.TrimSpace(c.Query("search")),
		Statuses: credentialStatusesFromRequest(c, geminiThrottleStatusTiers),
		PlanType: utils.NormalizeCodeAssistPlanType(c.Query("plan_type")),
	}
}

var geminiThrottleStatusTiers = stringSet(
	coregemini.ModelTierPro,
	coregemini.ModelTierFlash,
	coregemini.ModelTierFlashLite,
)

func (a *AdminHandler) BatchCreateGemini(c *gin.Context) {
	if a == nil || a.geminiAPI == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gemini backend is unavailable"})
		return
	}

	var req batchCreateCredentialReq
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
	a.batchUpdateCredentialStatus(c, utils.HandlerGemini, func(ctx context.Context, id, status, reason string) error {
		_, err := a.store.UpdateGeminiCLIStatus(ctx, id, status, reason)
		return err
	})
}

func (a *AdminHandler) BatchDeleteGemini(c *gin.Context) {
	a.batchDeleteCredentials(c, utils.HandlerGemini, a.store.DeleteGeminiCLI)
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
			Handler: string(utils.HandlerGemini),
			ID:      row.ID,
			Status: credentialStatusList(row.Status,
				throttleStatusDeadline{Tier: coregemini.ModelTierPro, Deadline: row.ThrottledUntilPro},
				throttleStatusDeadline{Tier: coregemini.ModelTierFlash, Deadline: row.ThrottledUntilFlash},
				throttleStatusDeadline{Tier: coregemini.ModelTierFlashLite, Deadline: row.ThrottledUntilFlashlite},
			),
			Email:     row.Email,
			ProjectID: row.ProjectID,
			PlanType:  row.PlanType,
			Expired:   row.Expired,
			Reason:    row.Reason,
			SyncedAt:  row.SyncedAt,
			Pro: quotaSchedulingMetric{
				Available:      scorePro >= 0,
				Quota:          row.QuotaPro,
				Reset:          row.ResetPro,
				ThrottledUntil: activeThrottleDeadline(row.ThrottledUntilPro),
				Score:          scorePro,
				Weight:         wPro,
			},
			Flash: quotaSchedulingMetric{
				Available:      scoreFlash >= 0,
				Quota:          row.QuotaFlash,
				Reset:          row.ResetFlash,
				ThrottledUntil: activeThrottleDeadline(row.ThrottledUntilFlash),
				Score:          scoreFlash,
				Weight:         wFlash,
			},
			Flashlite: quotaSchedulingMetric{
				Available:      scoreFlashlite >= 0,
				Quota:          row.QuotaFlashlite,
				Reset:          row.ResetFlashlite,
				ThrottledUntil: activeThrottleDeadline(row.ThrottledUntilFlashlite),
				Score:          scoreFlashlite,
				Weight:         wFlashlite,
			},
		}
	}
	overlayGeminiQuotaCache(items, a.credRefresh)
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
	projectID, err := a.geminiAPI.ResolveProjectID(ctx, tokenData.AccessToken)
	if err != nil {
		return db.GeminiCredential{}, fmt.Errorf("resolve gemini project_id: %w", err)
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return db.GeminiCredential{}, fmt.Errorf("resolve gemini project_id: empty project_id")
	}
	credentialID := utils.DefaultProjectCredentialID(email, projectID)
	if credentialID == "" {
		return db.GeminiCredential{}, fmt.Errorf("gemini credential id is required")
	}
	planType := utils.CodeAssistPlanTypeFree
	if current, getErr := a.store.GetGeminiCLI(ctx, credentialID); getErr == nil {
		if projectID == "" {
			projectID = utils.ProjectIDFromCredentialID(current.ID)
		}
		if normalized := utils.NormalizeCodeAssistPlanType(current.PlanType); normalized != "" {
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
	planType := utils.NormalizeCodeAssistPlanType(loaded)
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
