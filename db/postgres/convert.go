package postgres

import (
	"encoding/json"
	"time"

	sqlcpostgres "github.com/nekohy/MeowCLI/internal/db/postgres"
	db "github.com/nekohy/MeowCLI/internal/store"

	"github.com/jackc/pgx/v5/pgtype"
)

func tsTo(value pgtype.Timestamptz) time.Time {
	if !value.Valid || value.Time.Year() <= 1 {
		return time.Time{}
	}
	return value.Time
}

func tsFrom(value time.Time) pgtype.Timestamptz {
	if value.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func authKeyTo(value sqlcpostgres.AuthKey) db.AuthKey {
	return db.AuthKey{
		Key:       value.Key,
		Role:      value.Role,
		Note:      value.Note,
		CreatedAt: tsTo(value.CreatedAt),
	}
}

func codexTo(value sqlcpostgres.Codex) db.Codex {
	return db.Codex{
		ID:           value.ID,
		Status:       value.Status,
		AccessToken:  value.AccessToken,
		Expired:      tsTo(value.Expired),
		RefreshToken: value.RefreshToken,
		PlanType:     value.PlanType,
		PlanExpired:  tsTo(value.PlanExpired),
		Reason:       value.Reason,
	}
}

func reverseInfoTo(origin string, handler string, extra json.RawMessage) db.ReverseInfoFromModelRow {
	return db.ReverseInfoFromModelRow{
		Origin:  origin,
		Handler: handler,
		Extra:   extra,
	}
}

func modelRowTo(alias, origin, handler string, extra json.RawMessage) db.ModelRow {
	return db.ModelRow{
		Alias:   alias,
		Origin:  origin,
		Handler: handler,
		Extra:   extra,
	}
}

func listAvailableCodexRowTo(id, planType string, quota5h, quota7d float64, reset5h, reset7d, throttledUntil, syncedAt time.Time) db.ListAvailableCodexRow {
	return db.ListAvailableCodexRow{
		ID:             id,
		PlanType:       planType,
		Quota5h:        quota5h,
		Quota7d:        quota7d,
		Reset5h:        reset5h,
		Reset7d:        reset7d,
		ThrottledUntil: throttledUntil,
		SyncedAt:       syncedAt,
	}
}

func listCodexRowTo(id, status, accessToken string, expired time.Time, refreshToken, planType string, planExpired time.Time, reason string, quota5h, quota7d float64, reset5h, reset7d, throttledUntil, syncedAt time.Time) db.ListCodexRow {
	return db.ListCodexRow{
		ID:             id,
		Status:         status,
		AccessToken:    accessToken,
		Expired:        expired,
		RefreshToken:   refreshToken,
		PlanType:       planType,
		PlanExpired:    planExpired,
		Reason:         reason,
		Quota5h:        quota5h,
		Quota7d:        quota7d,
		Reset5h:        reset5h,
		Reset7d:        reset7d,
		ThrottledUntil: throttledUntil,
		SyncedAt:       syncedAt,
	}
}

func settingTo(key, value string, updatedAt time.Time) db.Setting {
	return db.Setting{
		Key:       key,
		Value:     value,
		UpdatedAt: updatedAt,
	}
}
