package handler

import (
	"context"
	"encoding/json"
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

const defaultGeminiPageSize = 25

type batchCreateGeminiReq struct {
	Tokens []string `json:"tokens" binding:"required,min=1"`
}

func (r *batchCreateGeminiReq) UnmarshalJSON(data []byte) error {
	var raw struct {
		Tokens        []string `json:"tokens"`
		RefreshTokens []string `json:"refresh_tokens"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	r.Tokens = raw.Tokens
	if len(r.Tokens) == 0 {
		r.Tokens = raw.RefreshTokens
	}
	return nil
}

type geminiListItem struct {
	Handler            string    `json:"handler"`
	ID                 string    `json:"id"`
	Status             string    `json:"status"`
	Email              string    `json:"email"`
	ProjectID          string    `json:"project_id"`
	PlanType           string    `json:"plan_type"`
	Expired            time.Time `json:"expired"`
	Reason             string    `json:"reason"`
	QuotaPro           float64   `json:"quota_pro"`
	ResetPro           time.Time `json:"reset_pro"`
	QuotaFlash         float64   `json:"quota_flash"`
	ResetFlash         time.Time `json:"reset_flash"`
	QuotaFlashlite     float64   `json:"quota_flashlite"`
	ResetFlashlite     time.Time `json:"reset_flashlite"`
	ThrottledUntil     time.Time `json:"throttled_until"`
	SyncedAt           time.Time `json:"synced_at"`
	ScorePro           float64   `json:"score_pro"`
	ScoreFlash         float64   `json:"score_flash"`
	ScoreFlashlite     float64   `json:"score_flashlite"`
	ErrorRatePro       float64   `json:"error_rate_pro"`
	WeightPro          float64   `json:"weight_pro"`
	ErrorRateFlash     float64   `json:"error_rate_flash"`
	WeightFlash        float64   `json:"weight_flash"`
	ErrorRateFlashlite float64   `json:"error_rate_flashlite"`
	WeightFlashlite    float64   `json:"weight_flashlite"`
	AdjustedScorePro   float64   `json:"adjusted_score_pro"`
	AdjustedScoreFlash float64   `json:"adjusted_score_flash"`
	AdjustedScoreLite  float64   `json:"adjusted_score_flashlite"`
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
	total, rows, err := a.listGeminiCredentials(c.Request.Context(), page, pageSize, filters)
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

	var created []batchCreateResult
	var errs []batchError

	for _, token := range req.Tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		id, err := a.upsertGeminiCredential(c.Request.Context(), token)
		if err != nil {
			errs = append(errs, batchError{
				Input: truncateToken(token),
				Error: err.Error(),
			})
			continue
		}
		created = append(created, batchCreateResult{ID: id.ID})
	}

	createdIDs := resultIDs(created)
	a.refreshCredentials(c.Request.Context(), utils.HandlerGemini, createdIDs)
	a.syncCredentialQuotas(c.Request.Context(), utils.HandlerGemini, createdIDs)
	c.JSON(http.StatusOK, gin.H{
		"created": created,
		"errors":  errs,
	})
}

func (a *AdminHandler) BatchUpdateGeminiStatus(c *gin.Context) {
	var req batchUpdateStatusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	var updated []string
	var errs []batchError

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
	var deleted []string
	var errs []batchError

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

func (a *AdminHandler) listGeminiCredentials(ctx context.Context, page, pageSize int, filters db.CredentialFilterParams) (int64, []geminiListItem, error) {
	total, err := a.store.CountGeminiCLIFiltered(ctx, filters)
	if err != nil {
		return 0, nil, err
	}

	rows, err := a.store.ListGeminiCLIPaged(ctx, db.ListCredentialPagedParams{
		Limit:                  int32(pageSize),
		Offset:                 int32((page - 1) * pageSize),
		CredentialFilterParams: filters,
	})
	if err != nil {
		return 0, nil, err
	}

	snap := a.currentSettings()
	ws := snap.QuotaWindowGeminiSeconds()

	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}

	var ratesPro, ratesFlash, ratesFlashlite map[string]float64
	if a.logStore != nil && len(ids) > 0 {
		window := snap.ErrorRateWindow()
		ratesPro, _ = a.logStore.ErrorRatesForCredentials(ctx, string(utils.HandlerGemini), coregemini.ModelTierPro, ids, window)
		ratesFlash, _ = a.logStore.ErrorRatesForCredentials(ctx, string(utils.HandlerGemini), coregemini.ModelTierFlash, ids, window)
		ratesFlashlite, _ = a.logStore.ErrorRatesForCredentials(ctx, string(utils.HandlerGemini), coregemini.ModelTierFlashLite, ids, window)
	}

	items := make([]geminiListItem, len(rows))
	for i, row := range rows {
		scorePro := coregemini.CalcScoreForTier(row.QuotaPro, row.QuotaFlash, row.QuotaFlashlite, row.ResetPro, row.ResetFlash, row.ResetFlashlite, coregemini.ModelTierPro, ws)
		scoreFlash := coregemini.CalcScoreForTier(row.QuotaPro, row.QuotaFlash, row.QuotaFlashlite, row.ResetPro, row.ResetFlash, row.ResetFlashlite, coregemini.ModelTierFlash, ws)
		scoreFlashlite := coregemini.CalcScoreForTier(row.QuotaPro, row.QuotaFlash, row.QuotaFlashlite, row.ResetPro, row.ResetFlash, row.ResetFlashlite, coregemini.ModelTierFlashLite, ws)

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
			Handler:            string(utils.HandlerGemini),
			ID:                 row.ID,
			Status:             row.Status,
			Email:              row.Email,
			ProjectID:          row.ProjectID,
			PlanType:           row.PlanType,
			Expired:            row.Expired,
			Reason:             row.Reason,
			QuotaPro:           row.QuotaPro,
			ResetPro:           row.ResetPro,
			QuotaFlash:         row.QuotaFlash,
			ResetFlash:         row.ResetFlash,
			QuotaFlashlite:     row.QuotaFlashlite,
			ResetFlashlite:     row.ResetFlashlite,
			ThrottledUntil:     row.ThrottledUntil,
			SyncedAt:           row.SyncedAt,
			ScorePro:           scorePro,
			ScoreFlash:         scoreFlash,
			ScoreFlashlite:     scoreFlashlite,
			ErrorRatePro:       erPro,
			WeightPro:          wPro,
			ErrorRateFlash:     erFlash,
			WeightFlash:        wFlash,
			ErrorRateFlashlite: erFlashlite,
			WeightFlashlite:    wFlashlite,
			AdjustedScorePro:   scheduling.AdjustedScore(scorePro, wPro),
			AdjustedScoreFlash: scheduling.AdjustedScore(scoreFlash, wFlash),
			AdjustedScoreLite:  scheduling.AdjustedScore(scoreFlashlite, wFlashlite),
		}
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
	} else if getErr != nil && !isStoreNotFound(getErr) {
		return db.GeminiCredential{}, getErr
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

func truncateToken(token string) string {
	if len(token) <= 8 {
		return "***"
	}
	return token[:4] + "..." + token[len(token)-4:]
}
