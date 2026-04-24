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
		Reason:       value.Reason,
	}
}

func geminiCredentialTo(value sqlcsqlite.Gemini) db.GeminiCredential {
	return db.GeminiCredential{
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

func listAvailableCodexRowTo(id, planType string, quota5h, quota7d, quotaSpark5h, quotaSpark7d float64, reset5h, reset7d, resetSpark5h, resetSpark7d, throttledUntil, throttledUntilSpark, syncedAt string) db.ListAvailableCodexRow {
	return db.ListAvailableCodexRow{
		ID:                  id,
		PlanType:            planType,
		Quota5h:             quota5h,
		Quota7d:             quota7d,
		QuotaSpark5h:        quotaSpark5h,
		QuotaSpark7d:        quotaSpark7d,
		Reset5h:             parseTime(reset5h),
		Reset7d:             parseTime(reset7d),
		ResetSpark5h:        parseTime(resetSpark5h),
		ResetSpark7d:        parseTime(resetSpark7d),
		ThrottledUntil:      parseTime(throttledUntil),
		ThrottledUntilSpark: parseTime(throttledUntilSpark),
		SyncedAt:            parseTime(syncedAt),
	}
}

func listCodexRowTo(id, status, accessToken, expired, refreshToken, planType, reason string, quota5h, quota7d, quotaSpark5h, quotaSpark7d float64, reset5h, reset7d, resetSpark5h, resetSpark7d, throttledUntil, syncedAt string) db.ListCodexRow {
	return db.ListCodexRow{
		ID:             id,
		Status:         status,
		AccessToken:    accessToken,
		Expired:        parseTime(expired),
		RefreshToken:   refreshToken,
		PlanType:       planType,
		Reason:         reason,
		Quota5h:        quota5h,
		Quota7d:        quota7d,
		QuotaSpark5h:   quotaSpark5h,
		QuotaSpark7d:   quotaSpark7d,
		Reset5h:        parseTime(reset5h),
		Reset7d:        parseTime(reset7d),
		ResetSpark5h:   parseTime(resetSpark5h),
		ResetSpark7d:   parseTime(resetSpark7d),
		ThrottledUntil: parseTime(throttledUntil),
		SyncedAt:       parseTime(syncedAt),
	}
}

func listGeminiCLIRowTo(id, status, accessToken, refreshToken, expired, email, projectID, planType, reason string, quotaPro, quotaFlash, quotaFlashlite float64, resetPro, resetFlash, resetFlashlite, throttledUntil, syncedAt string) db.ListGeminiCLIRow {
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
		QuotaPro:       quotaPro,
		ResetPro:       parseTime(resetPro),
		QuotaFlash:     quotaFlash,
		ResetFlash:     parseTime(resetFlash),
		QuotaFlashlite: quotaFlashlite,
		ResetFlashlite: parseTime(resetFlashlite),
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
