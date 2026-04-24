package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	corecodex "github.com/nekohy/MeowCLI/core/codex"
	"github.com/nekohy/MeowCLI/core/scheduling"
	db "github.com/nekohy/MeowCLI/internal/store"
	"github.com/nekohy/MeowCLI/utils"

	"github.com/gin-gonic/gin"
)

const defaultCodexPageSize = 25

type batchCreateResult struct {
	ID string `json:"id"`
}

type batchError struct {
	Input string `json:"input"`
	Error string `json:"error"`
}

type codexListItem struct {
	Handler        string    `json:"handler"`
	ID             string    `json:"id"`
	Status         string    `json:"status"`
	Expired        time.Time `json:"expired"`
	PlanType       string    `json:"plan_type"`
	Reason         string    `json:"reason"`
	Quota5h        float64   `json:"quota_5h"`
	Quota7d        float64   `json:"quota_7d"`
	QuotaSpark5h   float64   `json:"quota_spark_5h"`
	QuotaSpark7d   float64   `json:"quota_spark_7d"`
	Reset5h        time.Time `json:"reset_5h"`
	Reset7d        time.Time `json:"reset_7d"`
	ResetSpark5h   time.Time `json:"reset_spark_5h"`
	ResetSpark7d   time.Time `json:"reset_spark_7d"`
	ThrottledUntil time.Time `json:"throttled_until"`
	SyncedAt       time.Time `json:"synced_at"`
	Score          float64   `json:"score"`
	ScoreSpark     float64   `json:"score_spark"`
	SparkAvailable bool      `json:"spark_available"`
	ErrorRate      float64   `json:"error_rate"`
	Weight         float64   `json:"weight"`
	ErrorRateSpark float64   `json:"error_rate_spark"`
	WeightSpark    float64   `json:"weight_spark"`
	AdjustedScore  float64   `json:"adjusted_score"`
	AdjustedSpark  float64   `json:"adjusted_spark"`
}

func (a *AdminHandler) ListCodex(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(defaultCodexPageSize)))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = defaultCodexPageSize
	}

	filters := codexFiltersFromRequest(c)

	total, err := a.store.CountCodexFiltered(c.Request.Context(), filters)
	if err != nil {
		writeInternalError(c, err)
		return
	}

	offset := int32((page - 1) * pageSize)
	rows, err := a.store.ListCodexPaged(c.Request.Context(), db.ListCredentialPagedParams{
		Limit:                  int32(pageSize),
		Offset:                 offset,
		CredentialFilterParams: filters,
	})
	if err != nil {
		writeInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"data":      a.serializeCodexRows(c.Request.Context(), rows),
	})
}

func codexFiltersFromRequest(c *gin.Context) db.CredentialFilterParams {
	status := strings.TrimSpace(c.Query("status"))
	if status != "enabled" && status != "disabled" {
		status = ""
	}

	return db.CredentialFilterParams{
		Search:       strings.TrimSpace(c.Query("search")),
		Status:       status,
		PlanType:     corecodex.NormalizePlanType(c.Query("plan_type")),
		UnsyncedOnly: c.Query("unsynced") == "true",
	}
}

func (a *AdminHandler) BatchCreateCodex(c *gin.Context) {
	if a == nil || a.codexAPI == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "codex backend is unavailable"})
		return
	}

	var req batchCreateCodexReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	job := a.importJobs.StartMasked(context.Background(), utils.HandlerCodex, req.Tokens, func(ctx context.Context, token string) (string, error) {
		return a.processOneToken(ctx, token)
	}, func(id string) {
		a.invalidateCredentials(utils.HandlerCodex, []string{id})
		a.syncCredentialQuotas(context.Background(), utils.HandlerCodex, []string{id})
	}, maskToken)

	c.JSON(http.StatusAccepted, job)
}

type batchCreateCodexReq struct {
	Tokens []string `json:"tokens" binding:"required,min=1"`
}

func (a *AdminHandler) processOneToken(ctx context.Context, token string) (string, error) {
	switch {
	case strings.HasPrefix(token, "rt_"), strings.HasPrefix(token, "oaistb"):
		tokenData, _, err := a.codexAPI.RefreshAccessToken(ctx, token)
		if err != nil {
			return "", fmt.Errorf("failed to refresh refresh_token: %w", err)
		}
		return a.upsertCodexFromTokenData(ctx, tokenData.AccessToken, tokenData.RefreshToken, tokenData.IDToken)
	case strings.HasPrefix(token, "eyJ"):
		return a.upsertCodexFromTokenData(ctx, token, "", "")
	default:
		return "", fmt.Errorf("unsupported token format: expected refresh_token starting with rt_/oaistb or access_token starting with eyJ")
	}
}

type codexCredentialPayload struct {
	CredentialID string
	AccessToken  string
	RefreshToken string
	Expired      time.Time
	PlanType     string
	Email        string
}

func (a *AdminHandler) parseCodexTokenData(accessToken, refreshToken, idToken string) (*codexCredentialPayload, error) {
	accessClaims, err := utils.ParseJWT(accessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to parse access_token: %w", err)
	}
	if exp := accessClaims.GetExpiry(); !exp.IsZero() && exp.Before(time.Now()) {
		return nil, fmt.Errorf("access_token is expired")
	}

	accountUserID := accessClaims.GetAccountUserID()
	planType := accessClaims.GetPlanType()
	expired := accessClaims.GetExpiry()

	email := accessClaims.GetEmail()
	if idToken != "" {
		idClaims, idErr := utils.ParseJWT(idToken)
		if idErr == nil {
			if idEmail := idClaims.GetEmail(); idEmail != "" {
				email = idEmail
			}
		}
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, fmt.Errorf("could not extract email from token")
	}
	if accountUserID == "" {
		return nil, fmt.Errorf("could not extract chatgpt account user id from token")
	}
	accountID := utils.AccountIDFromCredentialID(accountUserID)
	if accountID == "" {
		return nil, fmt.Errorf("could not extract chatgpt account id from token")
	}
	credentialID := email + "__" + accountID

	planType = corecodex.NormalizePlanType(planType)

	return &codexCredentialPayload{
		CredentialID: credentialID,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Expired:      expired,
		PlanType:     planType,
		Email:        email,
	}, nil
}

func (a *AdminHandler) upsertCodexFromTokenData(ctx context.Context, accessToken, refreshToken, idToken string) (string, error) {
	payload, err := a.parseCodexTokenData(accessToken, refreshToken, idToken)
	if err != nil {
		return "", err
	}

	_, err = a.store.CreateCodex(ctx, db.CreateCodexParams{
		ID:           payload.CredentialID,
		Status:       "enabled",
		AccessToken:  payload.AccessToken,
		Expired:      payload.Expired,
		RefreshToken: payload.RefreshToken,
		PlanType:     payload.PlanType,
	})
	if err == nil {
		return payload.CredentialID, nil
	}
	if !errors.Is(err, db.ErrConflict) {
		return "", err
	}

	_, err = a.store.UpdateCodexTokens(ctx, db.UpdateCodexTokensParams{
		ID:           payload.CredentialID,
		Status:       "enabled",
		AccessToken:  payload.AccessToken,
		Expired:      payload.Expired,
		RefreshToken: payload.RefreshToken,
		PlanType:     payload.PlanType,
	})
	if err != nil {
		return "", err
	}
	return payload.CredentialID, nil
}

func (a *AdminHandler) BatchUpdateStatus(c *gin.Context) {
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

		_, err := a.store.UpdateCodexStatus(ctx, id, req.Status, "")
		if err != nil {
			errs = append(errs, batchError{
				Input: id,
				Error: storeErrorMessage(err, "credential not found", ""),
			})
			continue
		}
		updated = append(updated, id)
	}

	a.refreshCredentials(ctx, utils.HandlerCodex, updated)
	if req.Status == "enabled" {
		a.syncCredentialQuotas(ctx, utils.HandlerCodex, updated)
	}
	c.JSON(http.StatusOK, gin.H{
		"updated": updated,
		"errors":  errs,
	})
}

func (a *AdminHandler) BatchDeleteCodex(c *gin.Context) {
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

		if err := a.store.DeleteCodex(ctx, id); err != nil {
			errs = append(errs, batchError{
				Input: id,
				Error: storeErrorMessage(err, "credential not found", ""),
			})
			continue
		}
		deleted = append(deleted, id)
	}

	a.refreshCredentials(ctx, utils.HandlerCodex, deleted)
	c.JSON(http.StatusOK, gin.H{
		"deleted": deleted,
		"errors":  errs,
	})
}

func (a *AdminHandler) serializeCodexRows(ctx context.Context, rows []db.ListCodexRow) []codexListItem {
	snap := a.currentSettings()
	w5h := snap.QuotaWindow5hSeconds()
	w7d := snap.QuotaWindow7dSeconds()

	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}

	var ratesDefault, ratesSpark map[string]float64
	if a.logStore != nil && len(ids) > 0 {
		window := snap.ErrorRateWindow()
		ratesDefault, _ = a.logStore.ErrorRatesForCredentials(ctx, string(utils.HandlerCodex), corecodex.ModelTierDefault, ids, window)
		ratesSpark, _ = a.logStore.ErrorRatesForCredentials(ctx, string(utils.HandlerCodex), corecodex.ModelTierSpark, ids, window)
	}

	items := make([]codexListItem, 0, len(rows))
	for _, row := range rows {
		score := corecodex.CalcScore(row.Quota5h, row.Quota7d, row.Reset5h, row.Reset7d, w5h, w7d)
		scoreSpark := corecodex.CalcScoreSpark(row.QuotaSpark5h, row.QuotaSpark7d, row.ResetSpark5h, row.ResetSpark7d, w5h, w7d)

		var er, erSpark float64
		if ratesDefault != nil {
			er = ratesDefault[row.ID]
		}
		if ratesSpark != nil {
			erSpark = ratesSpark[row.ID]
		}
		w := scheduling.CalcWeight(er)
		wSpark := scheduling.CalcWeight(erSpark)

		items = append(items, codexListItem{
			Handler:        string(utils.HandlerCodex),
			ID:             row.ID,
			Status:         row.Status,
			Expired:        row.Expired,
			PlanType:       corecodex.NormalizePlanType(row.PlanType),
			Reason:         row.Reason,
			Quota5h:        row.Quota5h,
			Quota7d:        row.Quota7d,
			QuotaSpark5h:   row.QuotaSpark5h,
			QuotaSpark7d:   row.QuotaSpark7d,
			Reset5h:        row.Reset5h,
			Reset7d:        row.Reset7d,
			ResetSpark5h:   row.ResetSpark5h,
			ResetSpark7d:   row.ResetSpark7d,
			ThrottledUntil: row.ThrottledUntil,
			SyncedAt:       row.SyncedAt,
			Score:          score,
			ScoreSpark:     scoreSpark,
			SparkAvailable: isSparkAvailable(row, scoreSpark),
			ErrorRate:      er,
			Weight:         w,
			ErrorRateSpark: erSpark,
			WeightSpark:    wSpark,
			AdjustedScore:  scheduling.AdjustedScore(score, w),
			AdjustedSpark:  scheduling.AdjustedScore(scoreSpark, wSpark),
		})
	}
	return items
}

func isSparkAvailable(row db.ListCodexRow, scoreSpark float64) bool {
	if scoreSpark < 0 {
		return false
	}
	return true
}

func maskToken(token string) string {
	if len(token) <= 12 {
		return "***"
	}
	return token[:6] + "..." + token[len(token)-6:]
}

type batchUpdateStatusReq struct {
	IDs    []string `json:"ids" binding:"required,min=1"`
	Status string   `json:"status" binding:"required,oneof=enabled disabled"`
}

func (a *AdminHandler) refreshCredentials(ctx context.Context, handler utils.HandlerType, ids []string) {
	if a == nil || a.credRefresh == nil {
		return
	}
	a.credRefresh.InvalidateCredentials(handler, ids)
	_ = a.credRefresh.RefreshAvailable(ctx, handler)
}

func (a *AdminHandler) invalidateCredentials(handler utils.HandlerType, ids []string) {
	if a == nil || a.credRefresh == nil {
		return
	}
	a.credRefresh.InvalidateCredentials(handler, ids)
}

func (a *AdminHandler) syncCredentialQuotas(ctx context.Context, handler utils.HandlerType, ids []string) {
	if a == nil || a.credRefresh == nil {
		return
	}
	if len(ids) > 0 {
		a.credRefresh.SyncQuotas(ctx, handler, ids)
	}
}
