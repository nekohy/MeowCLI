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
		Reason:       value.Reason,
	}
}

func geminiCredentialTo(value sqlcpostgres.Gemini) db.GeminiCredential {
	return db.GeminiCredential{
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

func reverseInfoTo(origin string, handler string, planTypes string, plugin string, contentAffinity bool, fillFirst bool, extra json.RawMessage) db.ReverseInfoFromModelRow {
	return db.ReverseInfoFromModelRow{
		Origin:    origin,
		Handler:   handler,
		PlanTypes: planTypes,
		Plugin:    plugin,
		ModelScheduling: db.ModelScheduling{
			ContentAffinity: contentAffinity,
			FillFirst:       fillFirst,
		},
		Extra: extra,
	}
}

func modelRowTo(alias, origin, handler, planTypes string, plugin string, contentAffinity bool, fillFirst bool, extra json.RawMessage) db.ModelRow {
	return db.ModelRow{
		Alias:     alias,
		Origin:    origin,
		Handler:   handler,
		PlanTypes: planTypes,
		Plugin:    plugin,
		ModelScheduling: db.ModelScheduling{
			ContentAffinity: contentAffinity,
			FillFirst:       fillFirst,
		},
		Extra: extra,
	}
}

func listAvailableCodexRowTo(id, planType string, quota5h, quota7d, quota1mo, quotaSpark5h, quotaSpark7d, quotaSpark1mo float64, reset5h, reset7d, reset1mo, resetSpark5h, resetSpark7d, resetSpark1mo, throttledUntil, throttledUntilSpark, syncedAt time.Time) db.ListAvailableCodexRow {
	return db.ListAvailableCodexRow{
		ID:                  id,
		PlanType:            planType,
		Quota5h:             quota5h,
		Quota7d:             quota7d,
		Quota1mo:            quota1mo,
		QuotaSpark5h:        quotaSpark5h,
		QuotaSpark7d:        quotaSpark7d,
		QuotaSpark1mo:       quotaSpark1mo,
		Reset5h:             reset5h,
		Reset7d:             reset7d,
		Reset1mo:            reset1mo,
		ResetSpark5h:        resetSpark5h,
		ResetSpark7d:        resetSpark7d,
		ResetSpark1mo:       resetSpark1mo,
		ThrottledUntil:      throttledUntil,
		ThrottledUntilSpark: throttledUntilSpark,
		SyncedAt:            syncedAt,
	}
}

func listCodexRowTo(id, status, accessToken string, expired time.Time, refreshToken, planType string, reason string, quota5h, quota7d, quota1mo, quotaSpark5h, quotaSpark7d, quotaSpark1mo float64, reset5h, reset7d, reset1mo, resetSpark5h, resetSpark7d, resetSpark1mo, throttledUntilDefault, throttledUntilSpark, syncedAt time.Time, resetCreditsCount int) db.ListCodexRow {
	return db.ListCodexRow{
		ID:                    id,
		Status:                status,
		AccessToken:           accessToken,
		Expired:               expired,
		RefreshToken:          refreshToken,
		PlanType:              planType,
		Reason:                reason,
		Quota5h:               quota5h,
		Quota7d:               quota7d,
		Quota1mo:              quota1mo,
		QuotaSpark5h:          quotaSpark5h,
		QuotaSpark7d:          quotaSpark7d,
		QuotaSpark1mo:         quotaSpark1mo,
		Reset5h:               reset5h,
		Reset7d:               reset7d,
		Reset1mo:              reset1mo,
		ResetSpark5h:          resetSpark5h,
		ResetSpark7d:          resetSpark7d,
		ResetSpark1mo:         resetSpark1mo,
		ResetCreditsCount:     resetCreditsCount,
		ThrottledUntilDefault: throttledUntilDefault,
		ThrottledUntilSpark:   throttledUntilSpark,
		SyncedAt:              syncedAt,
	}
}

func listGeminiCLIRowTo(id, status, accessToken, refreshToken string, expired time.Time, email, projectID, planType, reason string, quotaPro, quotaFlash, quotaFlashlite float64, resetPro, resetFlash, resetFlashlite, throttledUntilPro, throttledUntilFlash, throttledUntilFlashlite, syncedAt time.Time) db.ListGeminiCLIRow {
	return db.ListGeminiCLIRow{
		ID:                      id,
		Status:                  status,
		AccessToken:             accessToken,
		RefreshToken:            refreshToken,
		Expired:                 expired,
		Email:                   email,
		ProjectID:               projectID,
		PlanType:                planType,
		Reason:                  reason,
		QuotaPro:                quotaPro,
		ResetPro:                resetPro,
		QuotaFlash:              quotaFlash,
		ResetFlash:              resetFlash,
		QuotaFlashlite:          quotaFlashlite,
		ResetFlashlite:          resetFlashlite,
		ThrottledUntilPro:       throttledUntilPro,
		ThrottledUntilFlash:     throttledUntilFlash,
		ThrottledUntilFlashlite: throttledUntilFlashlite,
		SyncedAt:                syncedAt,
	}
}

func settingTo(key, value string, updatedAt time.Time) db.Setting {
	return db.Setting{
		Key:       key,
		Value:     value,
		UpdatedAt: updatedAt,
	}
}
