package sqlite

import (
	"encoding/json"
	"time"

	sqlcsqlite "github.com/nekohy/MeowCLI/internal/db/sqlite"
	db "github.com/nekohy/MeowCLI/internal/store"
)

func parseTime(value string) time.Time {
	t, _ := time.Parse(timeLayout, value)
	return t
}

func fmtTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(timeLayout)
}

func authKeyTo(value sqlcsqlite.AuthKey) db.AuthKey {
	return db.AuthKey{
		Key:       value.Key,
		Role:      value.Role,
		Note:      value.Note,
		CreatedAt: parseTime(value.CreatedAt),
	}
}

func codexTo(value sqlcsqlite.Codex) db.Codex {
	return db.Codex{
		ID:           value.ID,
		Status:       value.Status,
		AccessToken:  value.AccessToken,
		Expired:      parseTime(value.Expired),
		RefreshToken: value.RefreshToken,
		PlanType:     value.PlanType,
		PlanExpired:  parseTime(value.PlanExpired),
		Reason:       value.Reason,
	}
}

func reverseInfoTo(origin string, handler string, extra string) db.ReverseInfoFromModelRow {
	return db.ReverseInfoFromModelRow{
		Origin:  origin,
		Handler: handler,
		Extra:   json.RawMessage(extra),
	}
}

func modelRowTo(alias, origin, handler, extra string) db.ModelRow {
	return db.ModelRow{
		Alias:   alias,
		Origin:  origin,
		Handler: handler,
		Extra:   json.RawMessage(extra),
	}
}

func listAvailableCodexRowTo(id, planType string, quota5h, quota7d float64, reset5h, reset7d, throttledUntil, syncedAt string) db.ListAvailableCodexRow {
	return db.ListAvailableCodexRow{
		ID:             id,
		PlanType:       planType,
		Quota5h:        quota5h,
		Quota7d:        quota7d,
		Reset5h:        parseTime(reset5h),
		Reset7d:        parseTime(reset7d),
		ThrottledUntil: parseTime(throttledUntil),
		SyncedAt:       parseTime(syncedAt),
	}
}

func listCodexRowTo(id, status, accessToken, expired, refreshToken, planType, planExpired, reason string, quota5h, quota7d float64, reset5h, reset7d, throttledUntil, syncedAt string) db.ListCodexRow {
	return db.ListCodexRow{
		ID:             id,
		Status:         status,
		AccessToken:    accessToken,
		Expired:        parseTime(expired),
		RefreshToken:   refreshToken,
		PlanType:       planType,
		PlanExpired:    parseTime(planExpired),
		Reason:         reason,
		Quota5h:        quota5h,
		Quota7d:        quota7d,
		Reset5h:        parseTime(reset5h),
		Reset7d:        parseTime(reset7d),
		ThrottledUntil: parseTime(throttledUntil),
		SyncedAt:       parseTime(syncedAt),
	}
}

func logRowTo(handler, credentialID string, statusCode int64, text, createdAt string) db.LogRow {
	return db.LogRow{
		Handler:      handler,
		CredentialID: credentialID,
		StatusCode:   int32(statusCode),
		Text:         text,
		CreatedAt:    parseTime(createdAt),
	}
}

func settingTo(key, value, updatedAt string) db.Setting {
	return db.Setting{
		Key:       key,
		Value:     value,
		UpdatedAt: parseTime(updatedAt),
	}
}
