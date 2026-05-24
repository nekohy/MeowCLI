package sqlite

import (
	"context"
	"strings"
	"time"

	sqlcsqlite "github.com/nekohy/MeowCLI/internal/db/sqlite"
	db "github.com/nekohy/MeowCLI/internal/store"
)

func (s *Store) CountEnabledAntigravity(ctx context.Context) (int64, error) {
	return s.queries.CountEnabledAntigravity(ctx)
}

func (s *Store) CountAntigravity(ctx context.Context) (int64, error) {
	return s.queries.CountAntigravity(ctx)
}

func (s *Store) CountAntigravityFiltered(ctx context.Context, filter db.CredentialFilterParams) (int64, error) {
	return s.queries.CountAntigravityFiltered(ctx, sqlcsqlite.CountAntigravityFilteredParams{
		Search:       sqliteCodexSearchPattern(filter.Search),
		Status:       strings.TrimSpace(filter.Status),
		PlanType:     strings.TrimSpace(filter.PlanType),
		UnsyncedOnly: sqliteBool(filter.UnsyncedOnly),
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
	row, err := s.queries.UpdateAntigravityTokens(ctx, sqlcsqlite.UpdateAntigravityTokensParams{
		Status:       arg.Status,
		AccessToken:  arg.AccessToken,
		RefreshToken: arg.RefreshToken,
		Expired:      fmtTime(arg.Expired),
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

func (s *Store) ListAntigravityPaged(ctx context.Context, arg db.ListCredentialPagedParams) ([]db.ListAntigravityRow, error) {
	rows, err := s.queries.ListAntigravityPaged(ctx, sqlcsqlite.ListAntigravityPagedParams{
		Search:       sqliteCodexSearchPattern(arg.Search),
		Status:       strings.TrimSpace(arg.Status),
		PlanType:     strings.TrimSpace(arg.PlanType),
		UnsyncedOnly: sqliteBool(arg.UnsyncedOnly),
		PageOffset:   int64(arg.Offset),
		PageLimit:    int64(arg.Limit),
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
	return s.queries.ListAntigravityPlanTypes(ctx, sqlcsqlite.ListAntigravityPlanTypesParams{
		Search:       sqliteCodexSearchPattern(filter.Search),
		Status:       strings.TrimSpace(filter.Status),
		UnsyncedOnly: sqliteBool(filter.UnsyncedOnly),
	})
}

func (s *Store) UpsertAntigravity(ctx context.Context, arg db.UpsertAntigravityParams) (db.AntigravityCredential, error) {
	row, err := s.queries.UpsertAntigravity(ctx, sqlcsqlite.UpsertAntigravityParams{
		ID:           arg.ID,
		Status:       arg.Status,
		AccessToken:  arg.AccessToken,
		RefreshToken: arg.RefreshToken,
		Expired:      fmtTime(arg.Expired),
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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return db.AntigravityCredential{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	queries := sqlcsqlite.New(tx)
	row, err := queries.UpdateAntigravityStatus(ctx, sqlcsqlite.UpdateAntigravityStatusParams{
		Status: status,
		Reason: reason,
		ID:     id,
	})
	if err != nil {
		return db.AntigravityCredential{}, wrapError(err)
	}
	if shouldClearCredentialThrottle(status) {
		if err := queries.ClearAntigravityQuotaThrottle(ctx, id); err != nil {
			return db.AntigravityCredential{}, wrapError(err)
		}
	}
	if err := tx.Commit(); err != nil {
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
	return parseTime(value), nil
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
	_, err := s.queries.UpsertAntigravityQuota(ctx, sqlcsqlite.UpsertAntigravityQuotaParams{
		CredentialID:   arg.CredentialID,
		QuotaClaude:    arg.QuotaClaude,
		ResetClaude:    fmtTime(arg.ResetClaude),
		QuotaPro:       arg.QuotaPro,
		ResetPro:       fmtTime(arg.ResetPro),
		QuotaFlash:     arg.QuotaFlash,
		ResetFlash:     fmtTime(arg.ResetFlash),
		QuotaFlashlite: arg.QuotaFlashlite,
		ResetFlashlite: fmtTime(arg.ResetFlashlite),
		QuotaTab:       arg.QuotaTab,
		ResetTab:       fmtTime(arg.ResetTab),
		QuotaImage:     arg.QuotaImage,
		ResetImage:     fmtTime(arg.ResetImage),
	})
	if err != nil {
		return err
	}
	if !arg.CreditsSynced {
		return nil
	}
	_, err = s.queries.UpsertAntigravityCredits(ctx, sqlcsqlite.UpsertAntigravityCreditsParams{
		CredentialID:  arg.CredentialID,
		CreditsAmount: arg.CreditsAmount,
		CreditTypes:   arg.CreditTypes,
	})
	return err
}

func (s *Store) SetAntigravityQuotaThrottled(ctx context.Context, credentialID string, modelTier string, throttledUntil time.Time) error {
	value := fmtTime(throttledUntil)
	switch normalizeAntigravityModelTier(modelTier) {
	case "claude":
		return s.queries.SetAntigravityQuotaThrottledClaude(ctx, sqlcsqlite.SetAntigravityQuotaThrottledClaudeParams{
			CredentialID:         credentialID,
			ThrottledUntilClaude: value,
		})
	case "pro":
		return s.queries.SetAntigravityQuotaThrottledPro(ctx, sqlcsqlite.SetAntigravityQuotaThrottledProParams{
			CredentialID:      credentialID,
			ThrottledUntilPro: value,
		})
	case "flash":
		return s.queries.SetAntigravityQuotaThrottledFlash(ctx, sqlcsqlite.SetAntigravityQuotaThrottledFlashParams{
			CredentialID:        credentialID,
			ThrottledUntilFlash: value,
		})
	case "flashlite":
		return s.queries.SetAntigravityQuotaThrottledFlashLite(ctx, sqlcsqlite.SetAntigravityQuotaThrottledFlashLiteParams{
			CredentialID:            credentialID,
			ThrottledUntilFlashlite: value,
		})
	case "tab":
		return s.queries.SetAntigravityQuotaThrottledTab(ctx, sqlcsqlite.SetAntigravityQuotaThrottledTabParams{
			CredentialID:      credentialID,
			ThrottledUntilTab: value,
		})
	case "image":
		return s.queries.SetAntigravityQuotaThrottledImage(ctx, sqlcsqlite.SetAntigravityQuotaThrottledImageParams{
			CredentialID:        credentialID,
			ThrottledUntilImage: value,
		})
	default:
		return s.queries.SetAntigravityQuotaThrottledAll(ctx, sqlcsqlite.SetAntigravityQuotaThrottledAllParams{
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

func antigravityCredentialTo(value sqlcsqlite.Antigravity) db.AntigravityCredential {
	return db.AntigravityCredential{
		ID:           value.ID,
		Status:       value.Status,
		AccessToken:  value.AccessToken,
		RefreshToken: value.RefreshToken,
		Expired:      parseTime(value.Expired),
		Email:        value.Email,
		ProjectID:    value.ProjectID,
		PlanType:     value.PlanType,
		Reason:       value.Reason,
		SyncedAt:     parseTime(value.SyncedAt),
	}
}

func antigravityListRowTo(value sqlcsqlite.ListAntigravityPagedRow) db.ListAntigravityRow {
	return db.ListAntigravityRow{
		ID:             value.ID,
		Status:         value.Status,
		AccessToken:    value.AccessToken,
		RefreshToken:   value.RefreshToken,
		Expired:        parseTime(value.Expired),
		Email:          value.Email,
		ProjectID:      value.ProjectID,
		PlanType:       value.PlanType,
		Reason:         value.Reason,
		QuotaClaude:    value.QuotaClaude,
		ResetClaude:    parseTime(value.ResetClaude),
		QuotaPro:       value.QuotaPro,
		ResetPro:       parseTime(value.ResetPro),
		QuotaFlash:     value.QuotaFlash,
		ResetFlash:     parseTime(value.ResetFlash),
		QuotaFlashlite: value.QuotaFlashlite,
		ResetFlashlite: parseTime(value.ResetFlashlite),
		QuotaTab:       value.QuotaTab,
		ResetTab:       parseTime(value.ResetTab),
		QuotaImage:     value.QuotaImage,
		ResetImage:     parseTime(value.ResetImage),
		CreditsAmount:  value.CreditsAmount,
		CreditTypes:    value.CreditTypes,
		ThrottledUntil: parseTime(value.ThrottledUntil),
		SyncedAt:       parseTime(value.SyncedAt),
	}
}

func availableAntigravityRowTo(value sqlcsqlite.ListAvailableAntigravityRow) db.ListAvailableAntigravityRow {
	return db.ListAvailableAntigravityRow{
		ID:                      value.ID,
		Email:                   value.Email,
		ProjectID:               value.ProjectID,
		PlanType:                value.PlanType,
		QuotaClaude:             value.QuotaClaude,
		ResetClaude:             parseTime(value.ResetClaude),
		QuotaPro:                value.QuotaPro,
		ResetPro:                parseTime(value.ResetPro),
		QuotaFlash:              value.QuotaFlash,
		ResetFlash:              parseTime(value.ResetFlash),
		QuotaFlashlite:          value.QuotaFlashlite,
		ResetFlashlite:          parseTime(value.ResetFlashlite),
		QuotaTab:                value.QuotaTab,
		ResetTab:                parseTime(value.ResetTab),
		QuotaImage:              value.QuotaImage,
		ResetImage:              parseTime(value.ResetImage),
		CreditsAmount:           value.CreditsAmount,
		CreditTypes:             value.CreditTypes,
		ThrottledUntilClaude:    parseTime(value.ThrottledUntilClaude),
		ThrottledUntilPro:       parseTime(value.ThrottledUntilPro),
		ThrottledUntilFlash:     parseTime(value.ThrottledUntilFlash),
		ThrottledUntilFlashlite: parseTime(value.ThrottledUntilFlashlite),
		ThrottledUntilTab:       parseTime(value.ThrottledUntilTab),
		ThrottledUntilImage:     parseTime(value.ThrottledUntilImage),
		ThrottledUntil:          parseTime(value.ThrottledUntil),
		SyncedAt:                parseTime(value.SyncedAt),
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
