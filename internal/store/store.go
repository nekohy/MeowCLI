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
	PlanExpired  time.Time `json:"plan_expired"`
	Reason       string    `json:"reason"`
}

type GeminiCredential struct {
	ID             string    `json:"id"`
	Status         string    `json:"status"`
	AccessToken    string    `json:"access_token"`
	RefreshToken   string    `json:"refresh_token"`
	Expired        time.Time `json:"expired"`
	Email          string    `json:"email"`
	ProjectID      string    `json:"project_id"`
	PlanType       string    `json:"plan_type"`
	Reason         string    `json:"reason"`
	ThrottledUntil time.Time `json:"throttled_until"`
	SyncedAt       time.Time `json:"synced_at"`
}

type UpdateCodexTokensParams struct {
	ID           string
	Status       string
	AccessToken  string
	Expired      time.Time
	RefreshToken string
	PlanType     string
	PlanExpired  time.Time
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
}

type UpsertQuotaParams struct {
	CredentialID string
	Quota5h      float64
	Quota7d      float64
	Reset5h      time.Time
	Reset7d      time.Time
}

type ReverseInfoFromModelRow struct {
	Origin    string          `json:"origin"`
	Handler   string          `json:"handler"`
	PlanTypes string          `json:"plan_types"`
	Extra     json.RawMessage `json:"extra"`
}

type ListAvailableCodexRow struct {
	ID             string    `json:"id"`
	PlanType       string    `json:"plan_type"`
	Quota5h        float64   `json:"quota_5h"`
	Quota7d        float64   `json:"quota_7d"`
	Reset5h        time.Time `json:"reset_5h"`
	Reset7d        time.Time `json:"reset_7d"`
	ThrottledUntil time.Time `json:"throttled_until"`
	SyncedAt       time.Time `json:"synced_at"`
}

type ListCodexRow struct {
	ID             string    `json:"id"`
	Status         string    `json:"status"`
	AccessToken    string    `json:"-"`
	Expired        time.Time `json:"expired"`
	RefreshToken   string    `json:"-"`
	PlanType       string    `json:"plan_type"`
	PlanExpired    time.Time `json:"plan_expired"`
	Reason         string    `json:"reason"`
	Quota5h        float64   `json:"quota_5h"`
	Quota7d        float64   `json:"quota_7d"`
	Reset5h        time.Time `json:"reset_5h"`
	Reset7d        time.Time `json:"reset_7d"`
	ThrottledUntil time.Time `json:"throttled_until"`
	SyncedAt       time.Time `json:"synced_at"`
}

type ListAvailableGeminiCLIRow struct {
	ID             string    `json:"id"`
	Email          string    `json:"email"`
	ProjectID      string    `json:"project_id"`
	PlanType       string    `json:"plan_type"`
	ThrottledUntil time.Time `json:"throttled_until"`
	SyncedAt       time.Time `json:"synced_at"`
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
	PlanExpired  time.Time
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
	SetQuotaThrottled(ctx context.Context, credentialID string, throttledUntil time.Time) error
	DeleteQuota(ctx context.Context, credentialID string) error
	ListAvailableCodex(ctx context.Context) ([]ListAvailableCodexRow, error)
	SetGeminiCLIThrottled(ctx context.Context, credentialID string, throttledUntil time.Time) error
	ListAvailableGeminiCLI(ctx context.Context) ([]ListAvailableGeminiCLIRow, error)

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
