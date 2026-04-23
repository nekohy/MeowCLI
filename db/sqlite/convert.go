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

func geminiCredentialTo(value sqlcsqlite.GeminiCli) db.GeminiCredential {
	return db.GeminiCredential{
		ID:             value.ID,
		Status:         value.Status,
		AccessToken:    value.AccessToken,
		RefreshToken:   value.RefreshToken,
		Expired:        parseTime(value.Expired),
		Email:          value.Email,
		ProjectID:      value.ProjectID,
		PlanType:       value.PlanType,
		Reason:         value.Reason,
		ThrottledUntil: parseTime(value.ThrottledUntil),
		SyncedAt:       parseTime(value.SyncedAt),
	}
}

func reverseInfoTo(origin string, handler string, planTypes string, extra string) db.ReverseInfoFromModelRow {
	return db.ReverseInfoFromModelRow{
		Origin:    origin,
		Handler:   handler,
		PlanTypes: planTypes,
		Extra:     json.RawMessage(extra),
	}
}

func modelRowTo(alias, origin, handler, planTypes, extra string) db.ModelRow {
	return db.ModelRow{
		Alias:     alias,
		Origin:    origin,
		Handler:   handler,
		PlanTypes: planTypes,
		Extra:     json.RawMessage(extra),
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

func listAvailableGeminiCLIRowTo(id, email, projectID, planType, throttledUntil, syncedAt string) db.ListAvailableGeminiCLIRow {
	return db.ListAvailableGeminiCLIRow{
		ID:             id,
		Email:          email,
		ProjectID:      projectID,
		PlanType:       planType,
		ThrottledUntil: parseTime(throttledUntil),
		SyncedAt:       parseTime(syncedAt),
	}
}

func listGeminiCLIRowTo(id, status, accessToken, refreshToken, expired, email, projectID, planType, reason, throttledUntil, syncedAt string) db.ListGeminiCLIRow {
	return db.ListGeminiCLIRow{
		ID:             id,
		Status:         status,
		AccessToken:    accessToken,
		RefreshToken:   refreshToken,
		Expired:        parseTime(expired),
		Email:          email,
		ProjectID:      projectID,
		PlanType:       planType,
		Reason:         reason,
		ThrottledUntil: parseTime(throttledUntil),
		SyncedAt:       parseTime(syncedAt),
	}
}

func settingTo(key, value, updatedAt string) db.Setting {
	return db.Setting{
		Key:       key,
		Value:     value,
		UpdatedAt: parseTime(updatedAt),
	}
}
