package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	codexapi "github.com/nekohy/MeowCLI/api/codex"
	corecodex "github.com/nekohy/MeowCLI/core/codex"
	"github.com/nekohy/MeowCLI/core/scheduling"
	db "github.com/nekohy/MeowCLI/internal/store"
	"github.com/nekohy/MeowCLI/utils"

	"github.com/gin-gonic/gin"
)

const defaultCodexPageSize = 6
const whoamiCodexAccessTokenTTL = 100 * 365 * 24 * time.Hour

type batchError struct {
	Input string `json:"input"`
	Error string `json:"error"`
}

type codexListItem struct {
	Handler           string                `json:"handler"`
	ID                string                `json:"id"`
	Status            []string              `json:"status"`
	Expired           time.Time             `json:"expired"`
	PlanType          string                `json:"plan_type"`
	Reason            string                `json:"reason"`
	SyncedAt          time.Time             `json:"synced_at"`
	Default           codexSchedulingMetric `json:"default"`
	Spark             codexSchedulingMetric `json:"spark"`
	ResetCreditsCount int                   `json:"reset_credits_count"`
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
	sortOptions := credentialSortOptionsFromRequest(c.Query, codexCredentialSortKeys)

	planTypes, err := a.store.ListCodexPlanTypes(c.Request.Context(), credentialPlanTypeFilter(filters))
	if err != nil {
		writeInternalError(c, err)
		return
	}

	total, err := a.store.CountCodexFiltered(c.Request.Context(), filters)
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
	rows, err := a.store.ListCodexPaged(c.Request.Context(), db.ListCredentialPagedParams{
		Limit:                  limit,
		Offset:                 offset,
		CredentialFilterParams: filters,
	})
	if err != nil {
		writeInternalError(c, err)
		return
	}
	items := a.serializeCodexRows(c.Request.Context(), rows)
	if sortOptions.enabled() {
		sortCodexListItems(items, sortOptions)
		items = paginateCodexListItems(items, page, pageSize)
	}

	c.JSON(http.StatusOK, gin.H{
		"total":      total,
		"page":       page,
		"page_size":  pageSize,
		"plan_types": planTypes,
		"data":       items,
	})
}

func credentialPlanTypeFilter(filters db.CredentialFilterParams) db.CredentialFilterParams {
	filters.PlanType = ""
	return filters
}

var codexThrottleStatusTiers = stringSet(corecodex.ModelTierDefault, corecodex.ModelTierSpark)

func codexFiltersFromRequest(c *gin.Context) db.CredentialFilterParams {
	return db.CredentialFilterParams{
		Search:       strings.TrimSpace(c.Query("search")),
		Statuses:     credentialStatusesFromRequest(c, codexThrottleStatusTiers),
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

	job := a.importJobs.Start(context.Background(), utils.HandlerCodex, req.Tokens, func(ctx context.Context, token string) (string, error) {
		return a.processOneToken(ctx, token)
	}, func(id string) {
		a.invalidateCredentials(utils.HandlerCodex, []string{id})
		a.syncCredentialQuotas(context.Background(), utils.HandlerCodex, []string{id})
	})

	c.JSON(http.StatusAccepted, importJobStartResponse(job))
}

type batchCreateCodexReq struct {
	Tokens []string `json:"tokens" binding:"required,min=1"`
}

func (a *AdminHandler) processOneToken(ctx context.Context, token string) (string, error) {
	token = strings.TrimSpace(token)
	switch {
	case strings.HasPrefix(token, "rt"), strings.HasPrefix(token, "oaistb"):
		tokenData, _, err := a.codexAPI.RefreshAccessToken(ctx, token)
		if err != nil {
			return "", fmt.Errorf("failed to refresh refresh_token: %w", err)
		}
		return a.upsertCodexFromTokenData(ctx, tokenData.AccessToken, tokenData.RefreshToken, tokenData.IDToken)
	case strings.HasPrefix(token, "at-"):
		whoami, err := a.codexAPI.FetchWhoami(ctx, token)
		if err != nil {
			return "", err
		}
		return a.upsertCodexFromWhoami(ctx, token, whoami)
	case strings.HasPrefix(token, "eyJ"):
		return a.upsertCodexFromTokenData(ctx, token, "", "")
	default:
		return "", fmt.Errorf("unsupported token format: expected refresh_token starting with rt_/oaistb or access_token starting with eyJ/at-")
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
	accessToken = strings.TrimSpace(accessToken)
	refreshToken = strings.TrimSpace(refreshToken)
	idToken = strings.TrimSpace(idToken)

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

func whoamiCodexTokenPayload(accessToken string, whoami *codexapi.WhoamiData) (*codexCredentialPayload, error) {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return nil, fmt.Errorf("access_token is required")
	}
	if whoami == nil {
		return nil, fmt.Errorf("chatgpt whoami is required")
	}
	email := strings.ToLower(strings.TrimSpace(whoami.Email))
	accountID := strings.TrimSpace(whoami.AccountID)
	if email == "" {
		return nil, fmt.Errorf("chatgpt whoami missing email")
	}
	if accountID == "" {
		return nil, fmt.Errorf("chatgpt whoami missing chatgpt_account_id")
	}
	planType := corecodex.NormalizePlanType(whoami.PlanType)
	if planType == "" {
		planType = corecodex.NormalizePlanType("unknown")
	}

	return &codexCredentialPayload{
		CredentialID: email + "__" + accountID,
		AccessToken:  accessToken,
		RefreshToken: "",
		Expired:      time.Now().UTC().Add(whoamiCodexAccessTokenTTL),
		PlanType:     planType,
		Email:        email,
	}, nil
}

func (a *AdminHandler) upsertCodexFromWhoami(ctx context.Context, accessToken string, whoami *codexapi.WhoamiData) (string, error) {
	payload, err := whoamiCodexTokenPayload(accessToken, whoami)
	if err != nil {
		return "", err
	}
	return a.upsertCodexPayload(ctx, payload)
}

func (a *AdminHandler) upsertCodexFromTokenData(ctx context.Context, accessToken, refreshToken, idToken string) (string, error) {
	payload, err := a.parseCodexTokenData(accessToken, refreshToken, idToken)
	if err != nil {
		return "", err
	}
	return a.upsertCodexPayload(ctx, payload)
}

func (a *AdminHandler) upsertCodexPayload(ctx context.Context, payload *codexCredentialPayload) (string, error) {
	_, err := a.store.CreateCodex(ctx, db.CreateCodexParams{
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
	a.batchUpdateCredentialStatus(c, utils.HandlerCodex, func(ctx context.Context, id, status, reason string) error {
		_, err := a.store.UpdateCodexStatus(ctx, id, status, reason)
		return err
	})
}

func (a *AdminHandler) BatchDeleteCodex(c *gin.Context) {
	a.batchDeleteCredentials(c, utils.HandlerCodex, a.store.DeleteCodex)
}

func (a *AdminHandler) serializeCodexRows(ctx context.Context, rows []db.ListCodexRow) []codexListItem {
	snap := a.currentSettings()
	w5h := snap.QuotaWindow5hSeconds()
	w7d := snap.QuotaWindow7dSeconds()

	var ratesDefault, ratesSpark map[string]float64
	if a.logStore != nil && len(rows) > 0 {
		defaultSince := make([]db.ErrorRateSince, 0, len(rows))
		sparkSince := make([]db.ErrorRateSince, 0, len(rows))
		for _, row := range rows {
			if since := corecodex.ErrorRateSince(row.Reset5h, row.Reset7d, row.Reset1mo, w5h, w7d); !since.IsZero() {
				defaultSince = append(defaultSince, db.ErrorRateSince{CredentialID: row.ID, Since: since})
			}
			if since := corecodex.ErrorRateSince(row.ResetSpark5h, row.ResetSpark7d, row.ResetSpark1mo, w5h, w7d); !since.IsZero() {
				sparkSince = append(sparkSince, db.ErrorRateSince{CredentialID: row.ID, Since: since})
			}
		}
		ratesDefault, _ = a.logStore.ErrorRatesForCredentials(ctx, string(utils.HandlerCodex), corecodex.ModelTierDefault, defaultSince, scheduling.MinErrorRateSamples)
		ratesSpark, _ = a.logStore.ErrorRatesForCredentials(ctx, string(utils.HandlerCodex), corecodex.ModelTierSpark, sparkSince, scheduling.MinErrorRateSamples)
	}

	items := make([]codexListItem, 0, len(rows))
	for _, row := range rows {
		score := corecodex.CalcScore(row.Quota5h, row.Quota7d, row.Quota1mo, row.Reset5h, row.Reset7d, row.Reset1mo, w5h, w7d)
		scoreSpark := corecodex.CalcScoreSpark(row.QuotaSpark5h, row.QuotaSpark7d, row.QuotaSpark1mo, row.ResetSpark5h, row.ResetSpark7d, row.ResetSpark1mo, w5h, w7d)

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
			Handler: string(utils.HandlerCodex),
			ID:      row.ID,
			Status: credentialStatusList(row.Status,
				throttleStatusDeadline{Tier: corecodex.ModelTierDefault, Deadline: row.ThrottledUntilDefault},
				throttleStatusDeadline{Tier: corecodex.ModelTierSpark, Deadline: row.ThrottledUntilSpark},
			),
			Expired:  row.Expired,
			PlanType: corecodex.NormalizePlanType(row.PlanType),
			Reason:   row.Reason,
			SyncedAt: row.SyncedAt,
			Default: codexSchedulingMetric{
				Available:      score >= 0,
				Quota5h:        row.Quota5h,
				Quota7d:        row.Quota7d,
				Quota1mo:       row.Quota1mo,
				Reset5h:        row.Reset5h,
				Reset7d:        row.Reset7d,
				Reset1mo:       row.Reset1mo,
				ThrottledUntil: activeThrottleDeadline(row.ThrottledUntilDefault),
				Score:          score,
				Weight:         w,
			},
			Spark: codexSchedulingMetric{
				Available:      scoreSpark >= 0,
				Quota5h:        row.QuotaSpark5h,
				Quota7d:        row.QuotaSpark7d,
				Quota1mo:       row.QuotaSpark1mo,
				Reset5h:        row.ResetSpark5h,
				Reset7d:        row.ResetSpark7d,
				Reset1mo:       row.ResetSpark1mo,
				ThrottledUntil: activeThrottleDeadline(row.ThrottledUntilSpark),
				Score:          scoreSpark,
				Weight:         wSpark,
			},
			ResetCreditsCount: row.ResetCreditsCount,
		})
	}
	overlayCodexQuotaCache(items, a.credRefresh)
	return items
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

func (a *AdminHandler) ListCodexRateLimitResetCredits(c *gin.Context) {
	credentialID := strings.TrimSpace(c.Query("credential_id"))
	if credentialID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "credential_id is required"})
		return
	}
	credits, err := a.credRefresh.ListCodexRateLimitResetCredits(c.Request.Context(), credentialID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, credits)
}

func (a *AdminHandler) ConsumeCodexRateLimitResetCredit(c *gin.Context) {
	var req struct {
		CredentialID string `json:"credential_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "credential_id is required"})
		return
	}
	credentialID := strings.TrimSpace(req.CredentialID)
	if credentialID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "credential_id is required"})
		return
	}
	resp, err := a.credRefresh.ConsumeCodexRateLimitResetCredit(c.Request.Context(), credentialID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}
