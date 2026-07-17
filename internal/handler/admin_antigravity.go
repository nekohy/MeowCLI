package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	antigravityapi "github.com/nekohy/MeowCLI/api/antigravity"
	coreantigravity "github.com/nekohy/MeowCLI/core/antigravity"
	"github.com/nekohy/MeowCLI/core/scheduling"
	db "github.com/nekohy/MeowCLI/internal/store"
	"github.com/nekohy/MeowCLI/utils"

	"github.com/gin-gonic/gin"
)

type antigravityListItem struct {
	Handler   string                `json:"handler"`
	ID        string                `json:"id"`
	Status    []string              `json:"status"`
	Email     string                `json:"email"`
	ProjectID string                `json:"project_id"`
	PlanType  string                `json:"plan_type"`
	Expired   time.Time             `json:"expired"`
	Reason    string                `json:"reason"`
	SyncedAt  time.Time             `json:"synced_at"`
	Claude    quotaSchedulingMetric `json:"claude"`
	Pro       quotaSchedulingMetric `json:"pro"`
	Flash     quotaSchedulingMetric `json:"flash"`
	Flashlite quotaSchedulingMetric `json:"flashlite"`
	Tab       quotaSchedulingMetric `json:"tab"`
	Image     quotaSchedulingMetric `json:"image"`
	Credits   antigravityCredits    `json:"credits"`
}

type antigravityCredits struct {
	Available bool     `json:"available"`
	Amount    float64  `json:"amount"`
	Types     []string `json:"types"`
}

func (a *AdminHandler) ListAntigravity(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(defaultCredentialPageSize)))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = defaultCredentialPageSize
	}

	filters := antigravityCredentialFiltersFromRequest(c)
	sortOptions := credentialSortOptionsFromRequest(c.Query, antigravityCredentialSortCapabilities)
	planTypes, err := a.store.ListAntigravityPlanTypes(c.Request.Context(), credentialPlanTypeFilter(filters))
	if err != nil {
		writeInternalError(c, err)
		return
	}
	total, rows, err := a.listAntigravityCredentials(c.Request.Context(), page, pageSize, filters, sortOptions)
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

func antigravityCredentialFiltersFromRequest(c *gin.Context) db.CredentialFilterParams {
	return db.CredentialFilterParams{
		Search:   strings.TrimSpace(c.Query("search")),
		Statuses: credentialStatusesFromRequest(c, antigravityThrottleStatusTiers),
		PlanType: utils.NormalizeCodeAssistPlanType(c.Query("plan_type")),
	}
}

var antigravityThrottleStatusTiers = stringSet(
	coreantigravity.ModelTierClaude,
	coreantigravity.ModelTierPro,
	coreantigravity.ModelTierFlash,
	coreantigravity.ModelTierFlashLite,
	coreantigravity.ModelTierTab,
	coreantigravity.ModelTierImage,
)

func (a *AdminHandler) BatchCreateAntigravity(c *gin.Context) {
	if a == nil || a.antigravityAPI == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "antigravity backend is unavailable"})
		return
	}

	var req batchCreateCredentialReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	job := a.importJobs.Start(context.Background(), utils.HandlerAntigravity, req.Tokens, func(ctx context.Context, token string) (string, error) {
		credential, err := a.upsertAntigravityCredential(ctx, token)
		if err != nil {
			return "", err
		}
		return credential.ID, nil
	}, func(id string) {
		a.invalidateCredentials(utils.HandlerAntigravity, []string{id})
		a.syncCredentialQuotas(context.Background(), utils.HandlerAntigravity, []string{id})
	})
	c.JSON(http.StatusAccepted, importJobStartResponse(job))
}

func (a *AdminHandler) BatchUpdateAntigravityStatus(c *gin.Context) {
	a.batchUpdateCredentialStatus(c, utils.HandlerAntigravity, func(ctx context.Context, id, status, reason string) error {
		_, err := a.store.UpdateAntigravityStatus(ctx, id, status, reason)
		return err
	})
}

func (a *AdminHandler) BatchDeleteAntigravity(c *gin.Context) {
	a.batchDeleteCredentials(c, utils.HandlerAntigravity, a.store.DeleteAntigravity)
}

func (a *AdminHandler) listAntigravityCredentials(ctx context.Context, page, pageSize int, filters db.CredentialFilterParams, sortOptions credentialSortOptions) (int64, []antigravityListItem, error) {
	total, err := a.store.CountAntigravityFiltered(ctx, filters)
	if err != nil {
		return 0, nil, err
	}
	offset := int32((page - 1) * pageSize)
	limit := int32(pageSize)
	if sortOptions.enabled() {
		offset = 0
		limit = credentialFetchLimit(total)
	}
	rows, err := a.store.ListAntigravityPaged(ctx, db.ListCredentialPagedParams{
		Limit:                  limit,
		Offset:                 offset,
		CredentialFilterParams: filters,
	})
	if err != nil {
		return 0, nil, err
	}
	ws := a.currentSettings().QuotaWindowCodeAssistSeconds()

	var ratesClaude, ratesPro, ratesFlash, ratesFlashlite, ratesTab, ratesImage map[string]float64
	if a.logStore != nil && len(rows) > 0 {
		claudeSince := make([]db.ErrorRateSince, 0, len(rows))
		proSince := make([]db.ErrorRateSince, 0, len(rows))
		flashSince := make([]db.ErrorRateSince, 0, len(rows))
		flashliteSince := make([]db.ErrorRateSince, 0, len(rows))
		tabSince := make([]db.ErrorRateSince, 0, len(rows))
		imageSince := make([]db.ErrorRateSince, 0, len(rows))
		for _, row := range rows {
			if since := coreantigravity.ErrorRateSince(row.ResetClaude, ws); !since.IsZero() {
				claudeSince = append(claudeSince, db.ErrorRateSince{CredentialID: row.ID, Since: since})
			}
			if since := coreantigravity.ErrorRateSince(row.ResetPro, ws); !since.IsZero() {
				proSince = append(proSince, db.ErrorRateSince{CredentialID: row.ID, Since: since})
			}
			if since := coreantigravity.ErrorRateSince(row.ResetFlash, ws); !since.IsZero() {
				flashSince = append(flashSince, db.ErrorRateSince{CredentialID: row.ID, Since: since})
			}
			if since := coreantigravity.ErrorRateSince(row.ResetFlashlite, ws); !since.IsZero() {
				flashliteSince = append(flashliteSince, db.ErrorRateSince{CredentialID: row.ID, Since: since})
			}
			if since := coreantigravity.ErrorRateSince(row.ResetTab, ws); !since.IsZero() {
				tabSince = append(tabSince, db.ErrorRateSince{CredentialID: row.ID, Since: since})
			}
			if since := coreantigravity.ErrorRateSince(row.ResetImage, ws); !since.IsZero() {
				imageSince = append(imageSince, db.ErrorRateSince{CredentialID: row.ID, Since: since})
			}
		}
		ratesClaude, _ = a.logStore.ErrorRatesForCredentials(ctx, string(utils.HandlerAntigravity), coreantigravity.ModelTierClaude, claudeSince, scheduling.MinErrorRateSamples)
		ratesPro, _ = a.logStore.ErrorRatesForCredentials(ctx, string(utils.HandlerAntigravity), coreantigravity.ModelTierPro, proSince, scheduling.MinErrorRateSamples)
		ratesFlash, _ = a.logStore.ErrorRatesForCredentials(ctx, string(utils.HandlerAntigravity), coreantigravity.ModelTierFlash, flashSince, scheduling.MinErrorRateSamples)
		ratesFlashlite, _ = a.logStore.ErrorRatesForCredentials(ctx, string(utils.HandlerAntigravity), coreantigravity.ModelTierFlashLite, flashliteSince, scheduling.MinErrorRateSamples)
		ratesTab, _ = a.logStore.ErrorRatesForCredentials(ctx, string(utils.HandlerAntigravity), coreantigravity.ModelTierTab, tabSince, scheduling.MinErrorRateSamples)
		ratesImage, _ = a.logStore.ErrorRatesForCredentials(ctx, string(utils.HandlerAntigravity), coreantigravity.ModelTierImage, imageSince, scheduling.MinErrorRateSamples)
	}

	items := make([]antigravityListItem, len(rows))
	for i, row := range rows {
		scoreClaude := coreantigravity.CalcScore(row.QuotaClaude, row.ResetClaude, ws)
		scorePro := coreantigravity.CalcScore(row.QuotaPro, row.ResetPro, ws)
		scoreFlash := coreantigravity.CalcScore(row.QuotaFlash, row.ResetFlash, ws)
		scoreFlashlite := coreantigravity.CalcScore(row.QuotaFlashlite, row.ResetFlashlite, ws)
		scoreTab := coreantigravity.CalcScore(row.QuotaTab, row.ResetTab, ws)
		scoreImage := coreantigravity.CalcScore(row.QuotaImage, row.ResetImage, ws)

		var erClaude, erPro, erFlash, erFlashlite, erTab, erImage float64
		if ratesClaude != nil {
			erClaude = ratesClaude[row.ID]
		}
		if ratesPro != nil {
			erPro = ratesPro[row.ID]
		}
		if ratesFlash != nil {
			erFlash = ratesFlash[row.ID]
		}
		if ratesFlashlite != nil {
			erFlashlite = ratesFlashlite[row.ID]
		}
		if ratesTab != nil {
			erTab = ratesTab[row.ID]
		}
		if ratesImage != nil {
			erImage = ratesImage[row.ID]
		}
		wClaude := scheduling.CalcWeight(erClaude)
		wPro := scheduling.CalcWeight(erPro)
		wFlash := scheduling.CalcWeight(erFlash)
		wFlashlite := scheduling.CalcWeight(erFlashlite)
		wTab := scheduling.CalcWeight(erTab)
		wImage := scheduling.CalcWeight(erImage)

		items[i] = antigravityListItem{
			Handler: string(utils.HandlerAntigravity),
			ID:      row.ID,
			Status: credentialStatusList(row.Status,
				throttleStatusDeadline{Tier: coreantigravity.ModelTierClaude, Deadline: row.ThrottledUntilClaude},
				throttleStatusDeadline{Tier: coreantigravity.ModelTierPro, Deadline: row.ThrottledUntilPro},
				throttleStatusDeadline{Tier: coreantigravity.ModelTierFlash, Deadline: row.ThrottledUntilFlash},
				throttleStatusDeadline{Tier: coreantigravity.ModelTierFlashLite, Deadline: row.ThrottledUntilFlashlite},
				throttleStatusDeadline{Tier: coreantigravity.ModelTierTab, Deadline: row.ThrottledUntilTab},
				throttleStatusDeadline{Tier: coreantigravity.ModelTierImage, Deadline: row.ThrottledUntilImage},
			),
			Email:     row.Email,
			ProjectID: row.ProjectID,
			PlanType:  utils.NormalizeCodeAssistPlanType(row.PlanType),
			Expired:   row.Expired,
			Reason:    row.Reason,
			SyncedAt:  row.SyncedAt,
			Claude: quotaSchedulingMetric{
				Available:      scoreClaude >= 0,
				Quota:          row.QuotaClaude,
				Reset:          row.ResetClaude,
				ThrottledUntil: activeThrottleDeadline(row.ThrottledUntilClaude),
				Score:          scoreClaude,
				Weight:         wClaude,
			},
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
			Tab: quotaSchedulingMetric{
				Available:      scoreTab >= 0,
				Quota:          row.QuotaTab,
				Reset:          row.ResetTab,
				ThrottledUntil: activeThrottleDeadline(row.ThrottledUntilTab),
				Score:          scoreTab,
				Weight:         wTab,
			},
			Image: quotaSchedulingMetric{
				Available:      scoreImage >= 0,
				Quota:          row.QuotaImage,
				Reset:          row.ResetImage,
				ThrottledUntil: activeThrottleDeadline(row.ThrottledUntilImage),
				Score:          scoreImage,
				Weight:         wImage,
			},
			Credits: antigravityCredits{
				Available: row.CreditsAmount > 0 && strings.TrimSpace(row.CreditTypes) != "",
				Amount:    row.CreditsAmount,
				Types:     splitCreditTypes(row.CreditTypes),
			},
		}
	}
	overlayAntigravityQuotaCache(items, a.credRefresh)
	if sortOptions.enabled() {
		sortAntigravityListItems(items, sortOptions)
		items = paginateAntigravityListItems(items, page, pageSize)
	}
	return total, items, nil
}

func splitCreditTypes(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		out = append(out, part)
	}
	return out
}

func (a *AdminHandler) upsertAntigravityCredential(ctx context.Context, refreshToken string) (db.AntigravityCredential, error) {
	if a.antigravityAPI == nil {
		return db.AntigravityCredential{}, fmt.Errorf("antigravity backend is unavailable")
	}

	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return db.AntigravityCredential{}, fmt.Errorf("refresh_token is required")
	}
	tokenData, err := a.antigravityAPI.RefreshAccessToken(ctx, refreshToken)
	if err != nil {
		return db.AntigravityCredential{}, err
	}
	return a.upsertAntigravityCredentialFromTokenData(ctx, tokenData)
}

func (a *AdminHandler) upsertAntigravityCredentialFromTokenData(ctx context.Context, tokenData *antigravityapi.TokenData) (db.AntigravityCredential, error) {
	if a.antigravityAPI == nil {
		return db.AntigravityCredential{}, fmt.Errorf("antigravity backend is unavailable")
	}
	if tokenData == nil {
		return db.AntigravityCredential{}, fmt.Errorf("antigravity token data is required")
	}

	email, err := a.antigravityAPI.FetchUserEmail(ctx, tokenData.AccessToken)
	if err != nil {
		return db.AntigravityCredential{}, err
	}
	profile, err := a.antigravityAPI.ResolveProjectProfile(ctx, tokenData.AccessToken)
	if err != nil {
		return db.AntigravityCredential{}, fmt.Errorf("resolve antigravity project_id: %w", err)
	}
	projectID := ""
	planType := utils.NormalizeCodeAssistPlanType("unknown")
	if profile != nil {
		projectID = profile.ProjectID
		if normalized := utils.NormalizeCodeAssistPlanType(profile.PlanType); normalized != "" {
			planType = normalized
		}
	}
	credentialID, projectID, err := antigravityCredentialIdentity(email, projectID)
	if err != nil {
		return db.AntigravityCredential{}, err
	}
	if current, getErr := a.store.GetAntigravity(ctx, credentialID); getErr == nil {
		if normalized := utils.NormalizeCodeAssistPlanType(current.PlanType); normalized != "" {
			planType = normalized
		}
	} else if !errors.Is(getErr, db.ErrNotFound) {
		return db.AntigravityCredential{}, getErr
	}
	return a.store.UpsertAntigravity(ctx, db.UpsertAntigravityParams{
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

func antigravityCredentialIdentity(email, projectID string) (string, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	projectID = strings.TrimSpace(projectID)
	if email == "" {
		return "", "", fmt.Errorf("antigravity email is required")
	}
	if projectID == "" {
		return "", "", fmt.Errorf("antigravity project_id is required")
	}
	credentialID := utils.DefaultProjectCredentialID(email, projectID)
	if credentialID == "" {
		return "", "", fmt.Errorf("antigravity credential id is required")
	}
	return credentialID, projectID, nil
}

func (a *AdminHandler) antigravityCounts(ctx context.Context) (int64, int64, error) {
	total, err := a.store.CountAntigravity(ctx)
	if err != nil {
		return 0, 0, err
	}
	enabled, err := a.store.CountEnabledAntigravity(ctx)
	if err != nil {
		return 0, 0, err
	}
	return total, enabled, nil
}
