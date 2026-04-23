package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	coregemini "github.com/nekohy/MeowCLI/core/gemini"
	db "github.com/nekohy/MeowCLI/internal/store"
	"github.com/nekohy/MeowCLI/utils"

	"github.com/gin-gonic/gin"
	geminiapi "github.com/nekohy/MeowCLI/api/gemini"
)

const defaultGeminiPageSize = 25

type geminiCredentialInput struct {
	ID           string `json:"id"`
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type batchCreateGeminiReq struct {
	Credentials []geminiCredentialInput `json:"credentials" binding:"required,min=1"`
}

type geminiListItem struct {
	Handler        string    `json:"handler"`
	ID             string    `json:"id"`
	Status         string    `json:"status"`
	Email          string    `json:"email"`
	ProjectID      string    `json:"project_id"`
	PlanType       string    `json:"plan_type"`
	Expired        time.Time `json:"expired"`
	Reason         string    `json:"reason"`
	SyncedAt       time.Time `json:"synced_at"`
	ThrottledUntil time.Time `json:"throttled_until"`
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

	for _, input := range req.Credentials {
		id, err := a.createGeminiCredential(c.Request.Context(), input)
		if err != nil {
			errs = append(errs, batchError{
				Input: firstNonEmpty(strings.TrimSpace(input.ID), "gemini"),
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

	a.refreshCredentials(ctx)
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

	a.refreshCredentials(ctx)
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

	items := make([]geminiListItem, len(rows))
	for i, row := range rows {
		items[i] = geminiListItem{
			Handler:        string(utils.HandlerGemini),
			ID:             row.ID,
			Status:         row.Status,
			Email:          row.Email,
			ProjectID:      row.ProjectID,
			PlanType:       row.PlanType,
			Expired:        row.Expired,
			Reason:         row.Reason,
			SyncedAt:       row.SyncedAt,
			ThrottledUntil: row.ThrottledUntil,
		}
	}
	return total, items, nil
}

func (a *AdminHandler) createGeminiCredential(ctx context.Context, input geminiCredentialInput) (string, error) {
	saved, err := a.upsertGeminiCredential(
		ctx,
		strings.TrimSpace(input.ID),
		strings.TrimSpace(input.RefreshToken),
	)
	if err != nil {
		return "", err
	}
	return saved.ID, nil
}

func (a *AdminHandler) upsertGeminiCredential(ctx context.Context, preferredID string, refreshToken string) (db.GeminiCredential, error) {
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
	return a.upsertGeminiCredentialFromTokenData(ctx, preferredID, tokenData)
}

func (a *AdminHandler) upsertGeminiCredentialFromTokenData(ctx context.Context, preferredID string, tokenData *geminiapi.TokenData) (db.GeminiCredential, error) {
	email, err := a.geminiAPI.FetchUserEmail(ctx, tokenData.AccessToken)
	if err != nil {
		return db.GeminiCredential{}, err
	}
	credentialID := strings.TrimSpace(preferredID)
	if credentialID == "" {
		credentialID = strings.ToLower(strings.TrimSpace(email))
	}
	if credentialID == "" {
		return db.GeminiCredential{}, fmt.Errorf("gemini credential id is required")
	}
	projectID, _ := a.geminiAPI.ResolveProjectID(ctx, tokenData.AccessToken)
	planType := coregemini.PlanTypeFree
	if current, getErr := a.store.GetGeminiCLI(ctx, credentialID); getErr == nil {
		if projectID == "" {
			projectID = strings.TrimSpace(current.ProjectID)
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
