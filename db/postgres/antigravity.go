package postgres

import (
	"context"
	"strings"
	"time"

	sqlcpostgres "github.com/nekohy/MeowCLI/internal/db/postgres"
	db "github.com/nekohy/MeowCLI/internal/store"
)

func (s *Store) CountEnabledAntigravity(ctx context.Context) (int64, error) {
	return s.queries.CountEnabledAntigravity(ctx)
}

func (s *Store) CountAntigravity(ctx context.Context) (int64, error) {
	return s.queries.CountAntigravity(ctx)
}

func (s *Store) CountAntigravityFiltered(ctx context.Context, filter db.CredentialFilterParams) (int64, error) {
	return s.queries.CountAntigravityFiltered(ctx, sqlcpostgres.CountAntigravityFilteredParams{
		Search:       postgresCodexSearchPattern(filter.Search),
		Statuses:     db.CredentialStatusFilterValue(filter.Statuses),
		PlanType:     strings.TrimSpace(filter.PlanType),
		UnsyncedOnly: filter.UnsyncedOnly,
	})
}

func (s *Store) GetAntigravity(ctx context.Context, id string) (db.AntigravityCredential, error) {
	row, err := s.queries.GetAntigravity(ctx, id)
	if err != nil {
		return db.AntigravityCredential{}, wrapError(err)
	}
	return antigravityCredentialTo(row), nil
}

func (s *Store) UpdateAntigravityTokens(ctx context.Context, arg db.UpdateAntigravityTokensParams) (db.AntigravityCredential, error) {
	row, err := s.queries.UpdateAntigravityTokens(ctx, sqlcpostgres.UpdateAntigravityTokensParams{
		Status:       arg.Status,
		AccessToken:  arg.AccessToken,
		RefreshToken: arg.RefreshToken,
		Expired:      tsFrom(arg.Expired),
		Email:        arg.Email,
		ProjectID:    arg.ProjectID,
		PlanType:     arg.PlanType,
		ID:           arg.ID,
	})
	if err != nil {
		return db.AntigravityCredential{}, wrapError(err)
	}
	return antigravityCredentialTo(row), nil
}

func (s *Store) ListAntigravity(ctx context.Context) ([]db.ListAntigravityRow, error) {
	rows, err := s.queries.ListAntigravity(ctx)
	if err != nil {
		return nil, err
	}
	resolved := make([]db.ListAntigravityRow, len(rows))
	for i, row := range rows {
		resolved[i] = antigravityListRowTo(sqlcpostgres.ListAntigravityPagedRow(row))
	}
	return resolved, nil
}

func (s *Store) ListAntigravityPaged(ctx context.Context, arg db.ListCredentialPagedParams) ([]db.ListAntigravityRow, error) {
	rows, err := s.queries.ListAntigravityPaged(ctx, sqlcpostgres.ListAntigravityPagedParams{
		Search:       postgresCodexSearchPattern(arg.Search),
		Statuses:     db.CredentialStatusFilterValue(arg.Statuses),
		PlanType:     strings.TrimSpace(arg.PlanType),
		UnsyncedOnly: arg.UnsyncedOnly,
		PageOffset:   arg.Offset,
		PageLimit:    arg.Limit,
	})
	if err != nil {
		return nil, err
	}
	resolved := make([]db.ListAntigravityRow, len(rows))
	for i, row := range rows {
		resolved[i] = antigravityListRowTo(row)
	}
	return resolved, nil
}

func (s *Store) ListAntigravityPlanTypes(ctx context.Context, filter db.CredentialFilterParams) ([]string, error) {
	return s.queries.ListAntigravityPlanTypes(ctx, sqlcpostgres.ListAntigravityPlanTypesParams{
		Search:       postgresCodexSearchPattern(filter.Search),
		Statuses:     db.CredentialStatusFilterValue(filter.Statuses),
		UnsyncedOnly: filter.UnsyncedOnly,
	})
}

func (s *Store) UpsertAntigravity(ctx context.Context, arg db.UpsertAntigravityParams) (db.AntigravityCredential, error) {
	row, err := s.queries.UpsertAntigravity(ctx, sqlcpostgres.UpsertAntigravityParams{
		ID:           arg.ID,
		Status:       arg.Status,
		AccessToken:  arg.AccessToken,
		RefreshToken: arg.RefreshToken,
		Expired:      tsFrom(arg.Expired),
		Email:        arg.Email,
		ProjectID:    arg.ProjectID,
		PlanType:     arg.PlanType,
		Reason:       arg.Reason,
	})
	if err != nil {
		return db.AntigravityCredential{}, wrapError(err)
	}
	return antigravityCredentialTo(row), nil
}

func (s *Store) DeleteAntigravity(ctx context.Context, id string) error {
	affected, err := s.queries.DeleteAntigravity(ctx, id)
	if err != nil {
		return wrapError(err)
	}
	if affected == 0 {
		return db.ErrNotFound
	}
	return nil
}

func (s *Store) UpdateAntigravityStatus(ctx context.Context, id string, status string, reason string) (db.AntigravityCredential, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.AntigravityCredential{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.Background())
		}
	}()

	queries := s.queries.WithTx(tx)
	row, err := queries.UpdateAntigravityStatus(ctx, sqlcpostgres.UpdateAntigravityStatusParams{
		Status: status,
		Reason: reason,
		ID:     id,
	})
	if err != nil {
		return db.AntigravityCredential{}, wrapError(err)
	}
	if db.ShouldClearCredentialThrottle(status) {
		if err := queries.ClearAntigravityQuotaThrottle(ctx, id); err != nil {
			return db.AntigravityCredential{}, wrapError(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return db.AntigravityCredential{}, err
	}
	committed = true
	return antigravityCredentialTo(row), nil
}

func (s *Store) RestoreExpiredThrottledAntigravity(ctx context.Context) error {
	return wrapError(s.queries.RestoreExpiredThrottledAntigravity(ctx))
}

func (s *Store) NextAntigravityThrottleDeadline(ctx context.Context) (time.Time, error) {
	value, err := s.queries.NextAntigravityThrottleDeadline(ctx)
	if err != nil {
		return time.Time{}, wrapError(err)
	}
	return tsTo(value), nil
}

func (s *Store) ListAvailableAntigravity(ctx context.Context) ([]db.ListAvailableAntigravityRow, error) {
	rows, err := s.queries.ListAvailableAntigravity(ctx)
	if err != nil {
		return nil, err
	}
	resolved := make([]db.ListAvailableAntigravityRow, len(rows))
	for i, row := range rows {
		resolved[i] = availableAntigravityRowTo(row)
	}
	return resolved, nil
}

func (s *Store) UpsertAntigravityQuota(ctx context.Context, arg db.UpsertAntigravityQuotaParams) error {
	_, err := s.queries.UpsertAntigravityQuota(ctx, sqlcpostgres.UpsertAntigravityQuotaParams{
		CredentialID:   arg.CredentialID,
		QuotaClaude:    arg.QuotaClaude,
		ResetClaude:    tsFrom(arg.ResetClaude),
		QuotaPro:       arg.QuotaPro,
		ResetPro:       tsFrom(arg.ResetPro),
		QuotaFlash:     arg.QuotaFlash,
		ResetFlash:     tsFrom(arg.ResetFlash),
		QuotaFlashlite: arg.QuotaFlashlite,
		ResetFlashlite: tsFrom(arg.ResetFlashlite),
		QuotaTab:       arg.QuotaTab,
		ResetTab:       tsFrom(arg.ResetTab),
		QuotaImage:     arg.QuotaImage,
		ResetImage:     tsFrom(arg.ResetImage),
	})
	if err != nil {
		return err
	}
	if !arg.CreditsSynced {
		return nil
	}
	_, err = s.queries.UpsertAntigravityCredits(ctx, sqlcpostgres.UpsertAntigravityCreditsParams{
		CredentialID:  arg.CredentialID,
		CreditsAmount: arg.CreditsAmount,
		CreditTypes:   arg.CreditTypes,
	})
	return err
}

func (s *Store) SetAntigravityQuotaThrottled(ctx context.Context, credentialID string, modelTier string, throttledUntil time.Time) error {
	value := tsFrom(throttledUntil)
	switch normalizeAntigravityModelTier(modelTier) {
	case "claude":
		return s.queries.SetAntigravityQuotaThrottledClaude(ctx, sqlcpostgres.SetAntigravityQuotaThrottledClaudeParams{
			CredentialID:         credentialID,
			ThrottledUntilClaude: value,
		})
	case "pro":
		return s.queries.SetAntigravityQuotaThrottledPro(ctx, sqlcpostgres.SetAntigravityQuotaThrottledProParams{
			CredentialID:      credentialID,
			ThrottledUntilPro: value,
		})
	case "flash":
		return s.queries.SetAntigravityQuotaThrottledFlash(ctx, sqlcpostgres.SetAntigravityQuotaThrottledFlashParams{
			CredentialID:        credentialID,
			ThrottledUntilFlash: value,
		})
	case "flashlite":
		return s.queries.SetAntigravityQuotaThrottledFlashLite(ctx, sqlcpostgres.SetAntigravityQuotaThrottledFlashLiteParams{
			CredentialID:            credentialID,
			ThrottledUntilFlashlite: value,
		})
	case "tab":
		return s.queries.SetAntigravityQuotaThrottledTab(ctx, sqlcpostgres.SetAntigravityQuotaThrottledTabParams{
			CredentialID:      credentialID,
			ThrottledUntilTab: value,
		})
	case "image":
		return s.queries.SetAntigravityQuotaThrottledImage(ctx, sqlcpostgres.SetAntigravityQuotaThrottledImageParams{
			CredentialID:        credentialID,
			ThrottledUntilImage: value,
		})
	default:
		return s.queries.SetAntigravityQuotaThrottledAll(ctx, sqlcpostgres.SetAntigravityQuotaThrottledAllParams{
			CredentialID:            credentialID,
			ThrottledUntilClaude:    value,
			ThrottledUntilPro:       value,
			ThrottledUntilFlash:     value,
			ThrottledUntilFlashlite: value,
			ThrottledUntilTab:       value,
			ThrottledUntilImage:     value,
		})
	}
}

func (s *Store) DeleteAntigravityQuota(ctx context.Context, credentialID string) error {
	affected, err := s.queries.DeleteAntigravityQuota(ctx, credentialID)
	if err != nil {
		return err
	}
	if affected == 0 {
		return db.ErrNotFound
	}
	return nil
}

func antigravityCredentialTo(value sqlcpostgres.Antigravity) db.AntigravityCredential {
	return db.AntigravityCredential{
		ID:           value.ID,
		Status:       value.Status,
		AccessToken:  value.AccessToken,
		RefreshToken: value.RefreshToken,
		Expired:      tsTo(value.Expired),
		Email:        value.Email,
		ProjectID:    value.ProjectID,
		PlanType:     value.PlanType,
		Reason:       value.Reason,
		SyncedAt:     tsTo(value.SyncedAt),
	}
}

func antigravityListRowTo(value sqlcpostgres.ListAntigravityPagedRow) db.ListAntigravityRow {
	return db.ListAntigravityRow{
		ID:                      value.ID,
		Status:                  value.Status,
		AccessToken:             value.AccessToken,
		RefreshToken:            value.RefreshToken,
		Expired:                 tsTo(value.Expired),
		Email:                   value.Email,
		ProjectID:               value.ProjectID,
		PlanType:                value.PlanType,
		Reason:                  value.Reason,
		QuotaClaude:             value.QuotaClaude,
		ResetClaude:             tsTo(value.ResetClaude),
		QuotaPro:                value.QuotaPro,
		ResetPro:                tsTo(value.ResetPro),
		QuotaFlash:              value.QuotaFlash,
		ResetFlash:              tsTo(value.ResetFlash),
		QuotaFlashlite:          value.QuotaFlashlite,
		ResetFlashlite:          tsTo(value.ResetFlashlite),
		QuotaTab:                value.QuotaTab,
		ResetTab:                tsTo(value.ResetTab),
		QuotaImage:              value.QuotaImage,
		ResetImage:              tsTo(value.ResetImage),
		CreditsAmount:           value.CreditsAmount,
		CreditTypes:             value.CreditTypes,
		ThrottledUntilClaude:    tsTo(value.ThrottledUntilClaude),
		ThrottledUntilPro:       tsTo(value.ThrottledUntilPro),
		ThrottledUntilFlash:     tsTo(value.ThrottledUntilFlash),
		ThrottledUntilFlashlite: tsTo(value.ThrottledUntilFlashlite),
		ThrottledUntilTab:       tsTo(value.ThrottledUntilTab),
		ThrottledUntilImage:     tsTo(value.ThrottledUntilImage),
		SyncedAt:                tsTo(value.SyncedAt),
	}
}

func availableAntigravityRowTo(value sqlcpostgres.ListAvailableAntigravityRow) db.ListAvailableAntigravityRow {
	return db.ListAvailableAntigravityRow{
		ID:                      value.ID,
		Email:                   value.Email,
		ProjectID:               value.ProjectID,
		PlanType:                value.PlanType,
		QuotaClaude:             value.QuotaClaude,
		ResetClaude:             tsTo(value.ResetClaude),
		QuotaPro:                value.QuotaPro,
		ResetPro:                tsTo(value.ResetPro),
		QuotaFlash:              value.QuotaFlash,
		ResetFlash:              tsTo(value.ResetFlash),
		QuotaFlashlite:          value.QuotaFlashlite,
		ResetFlashlite:          tsTo(value.ResetFlashlite),
		QuotaTab:                value.QuotaTab,
		ResetTab:                tsTo(value.ResetTab),
		QuotaImage:              value.QuotaImage,
		ResetImage:              tsTo(value.ResetImage),
		CreditsAmount:           value.CreditsAmount,
		CreditTypes:             value.CreditTypes,
		ThrottledUntilClaude:    tsTo(value.ThrottledUntilClaude),
		ThrottledUntilPro:       tsTo(value.ThrottledUntilPro),
		ThrottledUntilFlash:     tsTo(value.ThrottledUntilFlash),
		ThrottledUntilFlashlite: tsTo(value.ThrottledUntilFlashlite),
		ThrottledUntilTab:       tsTo(value.ThrottledUntilTab),
		ThrottledUntilImage:     tsTo(value.ThrottledUntilImage),
		ThrottledUntil:          tsTo(value.ThrottledUntil),
		SyncedAt:                tsTo(value.SyncedAt),
	}
}

func normalizeAntigravityModelTier(modelTier string) string {
	switch strings.ToLower(strings.TrimSpace(modelTier)) {
	case "claude", "pro", "flash", "flashlite", "tab", "image":
		return strings.ToLower(strings.TrimSpace(modelTier))
	default:
		return ""
	}
}
