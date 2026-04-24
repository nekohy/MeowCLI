package store

import (
	"context"
	"encoding/json"
	"time"
)

type Codex struct {
	ID           string    `json:"id"`
	Status       string    `json:"status"`
	AccessToken  string    `json:"access_token"`
	Expired      time.Time `json:"expired"`
	RefreshToken string    `json:"refresh_token"`
	PlanType     string    `json:"plan_type"`
	Reason       string    `json:"reason"`
}

type GeminiCredential struct {
	ID           string    `json:"id"`
	Status       string    `json:"status"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Expired      time.Time `json:"expired"`
	Email        string    `json:"email"`
	ProjectID    string    `json:"project_id"`
	PlanType     string    `json:"plan_type"`
	Reason       string    `json:"reason"`
	SyncedAt     time.Time `json:"synced_at"`
}

type UpdateCodexTokensParams struct {
	ID           string
	Status       string
	AccessToken  string
	Expired      time.Time
	RefreshToken string
	PlanType     string
}

type UpdateGeminiTokensParams struct {
	ID           string
	Status       string
	AccessToken  string
	RefreshToken string
	Expired      time.Time
	Email        string
	ProjectID    string
	PlanType     string
}

type InsertLogParams struct {
	Handler      string
	CredentialID string
	StatusCode   int32
	Text         string
	ModelTier    string
}

type UpsertQuotaParams struct {
	CredentialID string
	Quota5h      float64
	Quota7d      float64
	QuotaSpark5h float64
	QuotaSpark7d float64
	Reset5h      time.Time
	Reset7d      time.Time
	ResetSpark5h time.Time
	ResetSpark7d time.Time
}

type UpsertGeminiQuotaParams struct {
	CredentialID   string
	QuotaPro       float64
	ResetPro       time.Time
	QuotaFlash     float64
	ResetFlash     time.Time
	QuotaFlashlite float64
	ResetFlashlite time.Time
}

type ReverseInfoFromModelRow struct {
	Origin    string          `json:"origin"`
	Handler   string          `json:"handler"`
	PlanTypes string          `json:"plan_types"`
	Extra     json.RawMessage `json:"extra"`
}

type ListAvailableCodexRow struct {
	ID                  string    `json:"id"`
	PlanType            string    `json:"plan_type"`
	Quota5h             float64   `json:"quota_5h"`
	Quota7d             float64   `json:"quota_7d"`
	QuotaSpark5h        float64   `json:"quota_spark_5h"`
	QuotaSpark7d        float64   `json:"quota_spark_7d"`
	Reset5h             time.Time `json:"reset_5h"`
	Reset7d             time.Time `json:"reset_7d"`
	ResetSpark5h        time.Time `json:"reset_spark_5h"`
	ResetSpark7d        time.Time `json:"reset_spark_7d"`
	ThrottledUntilSpark time.Time `json:"throttled_until_spark"`
	ThrottledUntil      time.Time `json:"throttled_until"`
	SyncedAt            time.Time `json:"synced_at"`
}

type ListCodexRow struct {
	ID             string    `json:"id"`
	Status         string    `json:"status"`
	AccessToken    string    `json:"-"`
	Expired        time.Time `json:"expired"`
	RefreshToken   string    `json:"-"`
	PlanType       string    `json:"plan_type"`
	Reason         string    `json:"reason"`
	Quota5h        float64   `json:"quota_5h"`
	Quota7d        float64   `json:"quota_7d"`
	QuotaSpark5h   float64   `json:"quota_spark_5h"`
	QuotaSpark7d   float64   `json:"quota_spark_7d"`
	Reset5h        time.Time `json:"reset_5h"`
	Reset7d        time.Time `json:"reset_7d"`
	ResetSpark5h   time.Time `json:"reset_spark_5h"`
	ResetSpark7d   time.Time `json:"reset_spark_7d"`
	ThrottledUntil time.Time `json:"throttled_until"`
	SyncedAt       time.Time `json:"synced_at"`
}

type ListAvailableGeminiCLIRow struct {
	ID                      string    `json:"id"`
	Email                   string    `json:"email"`
	ProjectID               string    `json:"project_id"`
	PlanType                string    `json:"plan_type"`
	QuotaPro                float64   `json:"quota_pro"`
	ResetPro                time.Time `json:"reset_pro"`
	QuotaFlash              float64   `json:"quota_flash"`
	ResetFlash              time.Time `json:"reset_flash"`
	QuotaFlashlite          float64   `json:"quota_flashlite"`
	ResetFlashlite          time.Time `json:"reset_flashlite"`
	ThrottledUntilPro       time.Time `json:"throttled_until_pro"`
	ThrottledUntilFlash     time.Time `json:"throttled_until_flash"`
	ThrottledUntilFlashlite time.Time `json:"throttled_until_flashlite"`
	ThrottledUntil          time.Time `json:"throttled_until"`
	SyncedAt                time.Time `json:"synced_at"`
}

type ListGeminiCLIRow struct {
	ID             string    `json:"id"`
	Status         string    `json:"status"`
	AccessToken    string    `json:"-"`
	RefreshToken   string    `json:"-"`
	Expired        time.Time `json:"expired"`
	Email          string    `json:"email"`
	ProjectID      string    `json:"project_id"`
	PlanType       string    `json:"plan_type"`
	Reason         string    `json:"reason"`
	QuotaPro       float64   `json:"quota_pro"`
	ResetPro       time.Time `json:"reset_pro"`
	QuotaFlash     float64   `json:"quota_flash"`
	ResetFlash     time.Time `json:"reset_flash"`
	QuotaFlashlite float64   `json:"quota_flashlite"`
	ResetFlashlite time.Time `json:"reset_flashlite"`
	ThrottledUntil time.Time `json:"throttled_until"`
	SyncedAt       time.Time `json:"synced_at"`
}

type CreateCodexParams struct {
	ID           string
	Status       string
	AccessToken  string
	Expired      time.Time
	RefreshToken string
	PlanType     string
}

type UpsertGeminiCLIParams struct {
	ID           string
	Status       string
	AccessToken  string
	RefreshToken string
	Expired      time.Time
	Email        string
	ProjectID    string
	PlanType     string
	Reason       string
}

type ModelRow struct {
	Alias     string          `json:"alias"`
	Origin    string          `json:"origin"`
	Handler   string          `json:"handler"`
	PlanTypes string          `json:"plan_types"`
	Extra     json.RawMessage `json:"extra"`
}

type CreateModelParams struct {
	Alias     string
	Origin    string
	Handler   string
	PlanTypes string
	Extra     json.RawMessage
}

type UpdateModelParams struct {
	Alias     string
	Origin    string
	Handler   string
	PlanTypes string
	Extra     json.RawMessage
}

type LogRow struct {
	Handler      string    `json:"handler"`
	CredentialID string    `json:"credential_id"`
	StatusCode   int32     `json:"status_code"`
	Text         string    `json:"text"`
	ModelTier    string    `json:"model_tier"`
	CreatedAt    time.Time `json:"created_at"`
}

type CredentialFilterParams struct {
	Search       string
	Status       string
	PlanType     string
	UnsyncedOnly bool
}

type ListCredentialPagedParams struct {
	Limit  int32
	Offset int32
	CredentialFilterParams
}

type ListLogsParams struct {
	Limit  int32
	Offset int32
}

type AuthKey struct {
	Key       string    `json:"key"`
	Role      string    `json:"role"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateAuthKeyParams struct {
	Key  string
	Role string
	Note string
}

type Setting struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UpsertSettingParams struct {
	Key   string
	Value string
}

type LogStore interface {
	InsertLog(ctx context.Context, arg InsertLogParams) error
	ListLogs(ctx context.Context, arg ListLogsParams) ([]LogRow, error)
	CountLogs(ctx context.Context) (int64, error)
	ErrorRatesForCredentials(ctx context.Context, handler string, modelTier string, credentialIDs []string, window time.Duration) (map[string]float64, error)
}

type Store interface {
	CountEnabledCodex(ctx context.Context) (int64, error)
	CountCodex(ctx context.Context) (int64, error)
	CountCodexFiltered(ctx context.Context, filter CredentialFilterParams) (int64, error)
	CountEnabledGeminiCLI(ctx context.Context) (int64, error)
	CountGeminiCLI(ctx context.Context) (int64, error)
	CountGeminiCLIFiltered(ctx context.Context, filter CredentialFilterParams) (int64, error)
	CountModels(ctx context.Context) (int64, error)
	CountModelsByHandler(ctx context.Context, handler string) (int64, error)
	CountAuthKeys(ctx context.Context) (int64, error)
	GetCodex(ctx context.Context, id string) (Codex, error)
	UpdateCodexTokens(ctx context.Context, arg UpdateCodexTokensParams) (Codex, error)
	ListCodex(ctx context.Context) ([]ListCodexRow, error)
	ListCodexPaged(ctx context.Context, arg ListCredentialPagedParams) ([]ListCodexRow, error)
	CreateCodex(ctx context.Context, arg CreateCodexParams) (Codex, error)
	DeleteCodex(ctx context.Context, id string) error
	UpdateCodexStatus(ctx context.Context, id string, status string, reason string) (Codex, error)
	GetGeminiCLI(ctx context.Context, id string) (GeminiCredential, error)
	UpdateGeminiTokens(ctx context.Context, arg UpdateGeminiTokensParams) (GeminiCredential, error)
	ListGeminiCLI(ctx context.Context) ([]ListGeminiCLIRow, error)
	ListGeminiCLIPaged(ctx context.Context, arg ListCredentialPagedParams) ([]ListGeminiCLIRow, error)
	UpsertGeminiCLI(ctx context.Context, arg UpsertGeminiCLIParams) (GeminiCredential, error)
	DeleteGeminiCLI(ctx context.Context, id string) error
	UpdateGeminiCLIStatus(ctx context.Context, id string, status string, reason string) (GeminiCredential, error)

	ReverseInfoFromModel(ctx context.Context, alias string) (ReverseInfoFromModelRow, error)
	ListModels(ctx context.Context) ([]ModelRow, error)
	SaveSettings(ctx context.Context, settings []UpsertSettingParams) error
	CreateModel(ctx context.Context, arg CreateModelParams) (ModelRow, error)
	UpdateModel(ctx context.Context, arg UpdateModelParams) (ModelRow, error)
	DeleteModel(ctx context.Context, alias string) error

	UpsertQuota(ctx context.Context, arg UpsertQuotaParams) error
	SetQuotaThrottled(ctx context.Context, credentialID string, modelTier string, throttledUntil time.Time) error
	DeleteQuota(ctx context.Context, credentialID string) error
	ListAvailableCodex(ctx context.Context) ([]ListAvailableCodexRow, error)
	ListAvailableGeminiCLI(ctx context.Context) ([]ListAvailableGeminiCLIRow, error)

	UpsertGeminiQuota(ctx context.Context, arg UpsertGeminiQuotaParams) error
	SetGeminiQuotaThrottled(ctx context.Context, credentialID string, modelTier string, throttledUntil time.Time) error
	DeleteGeminiQuota(ctx context.Context, credentialID string) error

	ListAuthKeys(ctx context.Context) ([]AuthKey, error)
	GetAuthKey(ctx context.Context, key string) (AuthKey, error)
	CreateAuthKey(ctx context.Context, arg CreateAuthKeyParams) (AuthKey, error)
	CreateInitialAuthKey(ctx context.Context, arg CreateAuthKeyParams) (AuthKey, error)
	UpdateAuthKey(ctx context.Context, key string, role string, note string) (AuthKey, error)
	UpdateAuthKeyChecked(ctx context.Context, key string, role string, note string) (AuthKey, error)
	DeleteAuthKey(ctx context.Context, key string) error
	DeleteAuthKeyChecked(ctx context.Context, key string) error
	CountAuthKeysByRole(ctx context.Context, role string) (int64, error)

	ListSettings(ctx context.Context) ([]Setting, error)
	UpsertSetting(ctx context.Context, arg UpsertSettingParams) (Setting, error)

	Close()
}
