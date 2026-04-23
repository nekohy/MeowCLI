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
	PlanExpired    time.Time `json:"plan_expired"`
	Reason         string    `json:"reason"`
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
		"data":      serializeCodexRows(rows),
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

	var created []batchCreateResult
	var errs []batchError

	for _, token := range req.Tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		id, err := a.processOneToken(c.Request.Context(), token)
		if err != nil {
			errs = append(errs, batchError{
				Input: maskToken(token),
				Error: err.Error(),
			})
			continue
		}
		created = append(created, batchCreateResult{ID: id})
	}

	a.refreshCredentials(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{
		"created": created,
		"errors":  errs,
	})
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
	PlanExpired  time.Time
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

	credentialID := accessClaims.GetCredentialID()
	planType := accessClaims.GetPlanType()
	expired := accessClaims.GetExpiry()
	var planExpired time.Time

	email := accessClaims.GetEmail()
	if idToken != "" {
		idClaims, idErr := utils.ParseJWT(idToken)
		if idErr == nil {
			if idEmail := idClaims.GetEmail(); idEmail != "" {
				email = idEmail
			}
		}
	}
	if credentialID == "" {
		return nil, fmt.Errorf("could not extract chatgpt account user id from token")
	}
	accountID := utils.AccountIDFromCredentialID(credentialID)
	if accountID == "" {
		return nil, fmt.Errorf("could not extract chatgpt account id from token")
	}

	planType = corecodex.NormalizePlanType(planType)

	return &codexCredentialPayload{
		CredentialID: credentialID,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Expired:      expired,
		PlanType:     planType,
		PlanExpired:  planExpired,
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
		PlanExpired:  payload.PlanExpired,
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
		PlanExpired:  payload.PlanExpired,
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

	a.refreshCredentials(ctx)
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

	a.refreshCredentials(ctx)
	c.JSON(http.StatusOK, gin.H{
		"deleted": deleted,
		"errors":  errs,
	})
}

func serializeCodexRows(rows []db.ListCodexRow) []codexListItem {
	items := make([]codexListItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, codexListItem{
			Handler:        string(utils.HandlerCodex),
			ID:             row.ID,
			Status:         row.Status,
			Expired:        row.Expired,
			PlanType:       corecodex.NormalizePlanType(row.PlanType),
			PlanExpired:    row.PlanExpired,
			Reason:         row.Reason,
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

type batchUpdateStatusReq struct {
	IDs    []string `json:"ids" binding:"required,min=1"`
	Status string   `json:"status" binding:"required,oneof=enabled disabled"`
}

func (a *AdminHandler) refreshCredentials(ctx context.Context) {
	if a == nil || a.credRefresh == nil {
		return
	}
	_ = a.credRefresh.RefreshAvailable(ctx)
}
