package handler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	corecodex "github.com/nekohy/MeowCLI/core/codex"
	db "github.com/nekohy/MeowCLI/internal/store"
	"github.com/nekohy/MeowCLI/utils"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

const defaultCodexPageSize = 25

type batchCreateCodexReq struct {
	Tokens []string `json:"tokens" binding:"required,min=1"`
}

type batchUpdateStatusReq struct {
	IDs    []string `json:"ids" binding:"required,min=1"`
	Status string   `json:"status" binding:"required,oneof=enabled disabled"`
}

type batchDeleteCodexReq struct {
	IDs []string `json:"ids" binding:"required,min=1"`
}

type batchCreateResult struct {
	ID string `json:"id"`
}

type batchError struct {
	Input string `json:"input"`
	Error string `json:"error"`
}

type codexListItem struct {
	ID             string    `json:"id"`
	Status         string    `json:"status"`
	Expired        time.Time `json:"expired"`
	PlanType       string    `json:"plan_type"`
	PlanExpired    time.Time `json:"plan_expired"`
	Quota5h        float64   `json:"quota_5h"`
	Quota7d        float64   `json:"quota_7d"`
	Reset5h        time.Time `json:"reset_5h"`
	Reset7d        time.Time `json:"reset_7d"`
	ThrottledUntil time.Time `json:"throttled_until"`
	SyncedAt       time.Time `json:"synced_at"`
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

	total, err := a.store.CountCodex(c.Request.Context())
	if err != nil {
		writeInternalError(c, err)
		return
	}

	offset := int32((page - 1) * pageSize)
	rows, err := a.store.ListCodexPaged(c.Request.Context(), db.ListCodexPagedParams{
		Limit:  int32(pageSize),
		Offset: offset,
	})
	if err != nil {
		writeInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"data":      serializeCodexRows(rows),
	})
}

func (a *AdminHandler) BatchCreateCodex(c *gin.Context) {
	var req batchCreateCodexReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	var created []batchCreateResult
	var errs []batchError
	var newIDs []string

	for _, raw := range req.Tokens {
		token := strings.TrimSpace(raw)
		if token == "" {
			continue
		}

		id, createErr := a.processOneToken(ctx, token)
		if createErr != nil {
			errs = append(errs, batchError{
				Input: maskToken(token),
				Error: storeErrorMessage(createErr, "", "credential already exists"),
			})
			continue
		}
		created = append(created, batchCreateResult{ID: id})
		newIDs = append(newIDs, id)
	}

	// Refresh + sync quotas for newly created credentials
	a.refreshCredentials(ctx)
	if len(newIDs) > 0 && a.credRefresh != nil {
		a.credRefresh.SyncQuotas(ctx, newIDs)
	}

	c.JSON(http.StatusOK, gin.H{
		"created": created,
		"errors":  errs,
	})
}

func (a *AdminHandler) processOneToken(ctx context.Context, token string) (string, error) {
	switch {
	case strings.HasPrefix(token, "rt_"):
		if a.codexAPI == nil {
			return "", fmt.Errorf("codex api is unavailable")
		}
		tokenData, _, err := a.codexAPI.RefreshAccessToken(ctx, token)
		if err != nil {
			return "", fmt.Errorf("failed to refresh refresh_token: %w", err)
		}
		return a.createFromTokenData(ctx, tokenData.AccessToken, tokenData.RefreshToken, tokenData.IDToken)
	case strings.HasPrefix(token, "eyJ"):
		return a.createFromTokenData(ctx, token, "", "")
	default:
		return "", fmt.Errorf("unsupported token format: expected refresh_token starting with rt_ or access_token starting with eyJ")
	}
}

func (a *AdminHandler) createFromTokenData(ctx context.Context, accessToken, refreshToken, idToken string) (string, error) {
	accessClaims, err := utils.ParseJWT(accessToken)
	if err != nil {
		return "", fmt.Errorf("failed to parse access_token: %w", err)
	}
	if exp := accessClaims.GetExpiry(); !exp.IsZero() && exp.Before(time.Now()) {
		return "", fmt.Errorf("access_token is expired")
	}

	credentialID := accessClaims.GetCredentialID()

	planType := "unknown"
	if pt := accessClaims.GetPlanType(); pt != "" {
		planType = pt
	}
	planExpired := accessClaims.GetSubscriptionActiveUntil()
	expired := accessClaims.GetExpiry()
	email := accessClaims.GetEmail()

	// 优先从 id_token 提取 plan 信息（与 manager.refreshAndWriteBack 保持一致）
	if idToken != "" {
		if idClaims, idErr := utils.ParseJWT(idToken); idErr == nil {
			if pt := idClaims.GetPlanType(); pt != "" {
				planType = pt
			}
			if until := idClaims.GetSubscriptionActiveUntil(); !until.IsZero() {
				planExpired = until
			}
			if credentialID == "" {
				credentialID = idClaims.GetCredentialID()
			}
			if email == "" {
				email = idClaims.GetEmail()
			}
		}
	}
	if credentialID == "" {
		return "", fmt.Errorf("could not extract chatgpt account user id from token")
	}
	accountID := utils.AccountIDFromCredentialID(credentialID)
	if accountID == "" {
		return "", fmt.Errorf("could not extract chatgpt account id from token")
	}

	if a.currentSettings().CodexDeleteFreeAccounts && corecodex.IsFreePlanType(planType) {
		return "", fmt.Errorf("free plan credential is blocked by current settings")
	}
	planType = corecodex.NormalizePlanType(planType)

	if email != "" {
		log.Info().
			Str("credential_id", credentialID).
			Str("account_id", accountID).
			Str("email", email).
			Str("plan", planType).
			Msg("creating credential")
	}

	_, err = a.store.CreateCodex(ctx, db.CreateCodexParams{
		ID:           credentialID,
		Status:       "enabled",
		AccessToken:  accessToken,
		Expired:      expired,
		RefreshToken: refreshToken,
		PlanType:     planType,
		PlanExpired:  planExpired,
	})
	if err != nil {
		return "", err
	}
	return credentialID, nil
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
		_, err := a.store.UpdateCodexStatus(ctx, id, req.Status)
		if err != nil {
			errs = append(errs, batchError{
				Input: id,
				Error: storeErrorMessage(err, "credential not found", ""),
			})
			continue
		}
		updated = append(updated, id)
	}

	a.refreshCredentials(ctx)
	c.JSON(http.StatusOK, gin.H{
		"updated": updated,
		"errors":  errs,
	})
}

func (a *AdminHandler) BatchDeleteCodex(c *gin.Context) {
	var req batchDeleteCodexReq
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

	a.refreshCredentials(ctx)
	c.JSON(http.StatusOK, gin.H{
		"deleted": deleted,
		"errors":  errs,
	})
}

func (a *AdminHandler) refreshCredentials(ctx context.Context) {
	if a.credRefresh != nil {
		_ = a.credRefresh.RefreshAvailable(ctx)
	}
}

func serializeCodexRows(rows []db.ListCodexRow) []codexListItem {
	items := make([]codexListItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, codexListItem{
			ID:             row.ID,
			Status:         row.Status,
			Expired:        row.Expired,
			PlanType:       corecodex.NormalizePlanType(row.PlanType),
			PlanExpired:    row.PlanExpired,
			Quota5h:        row.Quota5h,
			Quota7d:        row.Quota7d,
			Reset5h:        row.Reset5h,
			Reset7d:        row.Reset7d,
			ThrottledUntil: row.ThrottledUntil,
			SyncedAt:       row.SyncedAt,
		})
	}
	return items
}

func maskToken(token string) string {
	if len(token) <= 12 {
		return "***"
	}
	return token[:6] + "..." + token[len(token)-6:]
}
