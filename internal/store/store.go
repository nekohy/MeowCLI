package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/nekohy/MeowCLI/core/scheduling"
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

type AntigravityCredential struct {
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

type OpenCodeGoCredential struct {
	ID         string    `json:"id"`
	Status     string    `json:"status"`
	APIKey     string    `json:"-"`
	AuthCookie string    `json:"-"`
	Reason     string    `json:"reason"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
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

type UpdateAntigravityTokensParams struct {
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
	ModelTier    string
	Model        string
	APIType      string
	Stream       bool
	FirstByte    float64
	Duration     float64
	Error        string
}

func CredentialStatusFilterValue(statuses []string) string {
	values := make([]string, 0, len(statuses))
	for _, status := range statuses {
		status = strings.TrimSpace(strings.ToLower(status))
		if status != "" {
			values = append(values, status)
		}
	}
	return strings.Join(values, ",")
}

func ShouldClearCredentialThrottle(status string) bool {
	switch strings.TrimSpace(status) {
	case "enabled", "disabled":
		return true
	default:
		return false
	}
}

func LogJSONError(body string) string {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" || !json.Valid([]byte(trimmed)) {
		return ""
	}
	return body
}

type LogRequestMetrics struct {
	Model     string
	APIType   string
	Stream    bool
	FirstByte float64
	Duration  float64
	Error     string
}

func NewInsertLogParams(handler string, credentialID string, statusCode int32, modelTier string, metrics LogRequestMetrics) InsertLogParams {
	return InsertLogParams{
		Handler:      handler,
		CredentialID: credentialID,
		StatusCode:   statusCode,
		ModelTier:    modelTier,
		Model:        metrics.Model,
		APIType:      metrics.APIType,
		Stream:       metrics.Stream,
		FirstByte:    metrics.FirstByte,
		Duration:     metrics.Duration,
		Error:        metrics.Error,
	}
}

type ErrorRateSince struct {
	CredentialID string
	Since        time.Time
}

type UpsertQuotaParams struct {
	CredentialID      string
	Quota5h           float64
	Quota7d           float64
	Quota1mo          float64
	QuotaSpark5h      float64
	QuotaSpark7d      float64
	QuotaSpark1mo     float64
	Reset5h           time.Time
	Reset7d           time.Time
	Reset1mo          time.Time
	ResetSpark5h      time.Time
	ResetSpark7d      time.Time
	ResetSpark1mo     time.Time
	ResetCreditsCount int
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

type UpsertAntigravityQuotaParams struct {
	CredentialID   string
	QuotaClaude    float64
	ResetClaude    time.Time
	QuotaPro       float64
	ResetPro       time.Time
	QuotaFlash     float64
	ResetFlash     time.Time
	QuotaFlashlite float64
	ResetFlashlite time.Time
	QuotaTab       float64
	ResetTab       time.Time
	QuotaImage     float64
	ResetImage     time.Time
	CreditsAmount  float64
	CreditTypes    string
	CreditsSynced  bool
}

type UpsertOpenCodeGoQuotaParams struct {
	CredentialID string
	Quota5h      float64
	Quota7d      float64
	Quota1mo     float64
	Reset5h      time.Time
	Reset7d      time.Time
	Reset1mo     time.Time
	RewardsCount int
}

type ModelScheduling = scheduling.ModelScheduling

type ReverseInfoFromModelRow struct {
	Origin    string `json:"origin"`
	Handler   string `json:"handler"`
	PlanTypes string `json:"plan_types"`
	Plugin    string `json:"plugin"`
	ModelScheduling
	Extra json.RawMessage `json:"extra"`
}

type ListAvailableCodexRow struct {
	ID                  string    `json:"id"`
	PlanType            string    `json:"plan_type"`
	Quota5h             float64   `json:"quota_5h"`
	Quota7d             float64   `json:"quota_7d"`
	Quota1mo            float64   `json:"quota_1mo"`
	QuotaSpark5h        float64   `json:"quota_spark_5h"`
	QuotaSpark7d        float64   `json:"quota_spark_7d"`
	QuotaSpark1mo       float64   `json:"quota_spark_1mo"`
	Reset5h             time.Time `json:"reset_5h"`
	Reset7d             time.Time `json:"reset_7d"`
	Reset1mo            time.Time `json:"reset_1mo"`
	ResetSpark5h        time.Time `json:"reset_spark_5h"`
	ResetSpark7d        time.Time `json:"reset_spark_7d"`
	ResetSpark1mo       time.Time `json:"reset_spark_1mo"`
	ThrottledUntilSpark time.Time `json:"throttled_until_spark"`
	ThrottledUntil      time.Time `json:"throttled_until"`
	SyncedAt            time.Time `json:"synced_at"`
}

type ListCodexRow struct {
	ID                    string    `json:"id"`
	Status                string    `json:"status"`
	AccessToken           string    `json:"-"`
	Expired               time.Time `json:"expired"`
	RefreshToken          string    `json:"-"`
	PlanType              string    `json:"plan_type"`
	Reason                string    `json:"reason"`
	Quota5h               float64   `json:"quota_5h"`
	Quota7d               float64   `json:"quota_7d"`
	Quota1mo              float64   `json:"quota_1mo"`
	QuotaSpark5h          float64   `json:"quota_spark_5h"`
	QuotaSpark7d          float64   `json:"quota_spark_7d"`
	QuotaSpark1mo         float64   `json:"quota_spark_1mo"`
	Reset5h               time.Time `json:"reset_5h"`
	Reset7d               time.Time `json:"reset_7d"`
	Reset1mo              time.Time `json:"reset_1mo"`
	ResetSpark5h          time.Time `json:"reset_spark_5h"`
	ResetSpark7d          time.Time `json:"reset_spark_7d"`
	ResetSpark1mo         time.Time `json:"reset_spark_1mo"`
	ResetCreditsCount     int       `json:"reset_credits_count"`
	ThrottledUntilDefault time.Time `json:"throttled_until_default"`
	ThrottledUntilSpark   time.Time `json:"throttled_until_spark"`
	SyncedAt              time.Time `json:"synced_at"`
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
	ID                      string    `json:"id"`
	Status                  string    `json:"status"`
	AccessToken             string    `json:"-"`
	RefreshToken            string    `json:"-"`
	Expired                 time.Time `json:"expired"`
	Email                   string    `json:"email"`
	ProjectID               string    `json:"project_id"`
	PlanType                string    `json:"plan_type"`
	Reason                  string    `json:"reason"`
	QuotaPro                float64   `json:"quota_pro"`
	ResetPro                time.Time `json:"reset_pro"`
	QuotaFlash              float64   `json:"quota_flash"`
	ResetFlash              time.Time `json:"reset_flash"`
	QuotaFlashlite          float64   `json:"quota_flashlite"`
	ResetFlashlite          time.Time `json:"reset_flashlite"`
	ThrottledUntilPro       time.Time `json:"throttled_until_pro"`
	ThrottledUntilFlash     time.Time `json:"throttled_until_flash"`
	ThrottledUntilFlashlite time.Time `json:"throttled_until_flashlite"`
	SyncedAt                time.Time `json:"synced_at"`
}

type ListAvailableAntigravityRow struct {
	ID                      string    `json:"id"`
	Email                   string    `json:"email"`
	ProjectID               string    `json:"project_id"`
	PlanType                string    `json:"plan_type"`
	QuotaClaude             float64   `json:"quota_claude"`
	ResetClaude             time.Time `json:"reset_claude"`
	QuotaPro                float64   `json:"quota_pro"`
	ResetPro                time.Time `json:"reset_pro"`
	QuotaFlash              float64   `json:"quota_flash"`
	ResetFlash              time.Time `json:"reset_flash"`
	QuotaFlashlite          float64   `json:"quota_flashlite"`
	ResetFlashlite          time.Time `json:"reset_flashlite"`
	QuotaTab                float64   `json:"quota_tab"`
	ResetTab                time.Time `json:"reset_tab"`
	QuotaImage              float64   `json:"quota_image"`
	ResetImage              time.Time `json:"reset_image"`
	CreditsAmount           float64   `json:"credits_amount"`
	CreditTypes             string    `json:"credit_types"`
	ThrottledUntilClaude    time.Time `json:"throttled_until_claude"`
	ThrottledUntilPro       time.Time `json:"throttled_until_pro"`
	ThrottledUntilFlash     time.Time `json:"throttled_until_flash"`
	ThrottledUntilFlashlite time.Time `json:"throttled_until_flashlite"`
	ThrottledUntilTab       time.Time `json:"throttled_until_tab"`
	ThrottledUntilImage     time.Time `json:"throttled_until_image"`
	ThrottledUntil          time.Time `json:"throttled_until"`
	SyncedAt                time.Time `json:"synced_at"`
}

type ListAntigravityRow struct {
	ID                      string    `json:"id"`
	Status                  string    `json:"status"`
	AccessToken             string    `json:"-"`
	RefreshToken            string    `json:"-"`
	Expired                 time.Time `json:"expired"`
	Email                   string    `json:"email"`
	ProjectID               string    `json:"project_id"`
	PlanType                string    `json:"plan_type"`
	Reason                  string    `json:"reason"`
	QuotaClaude             float64   `json:"quota_claude"`
	ResetClaude             time.Time `json:"reset_claude"`
	QuotaPro                float64   `json:"quota_pro"`
	ResetPro                time.Time `json:"reset_pro"`
	QuotaFlash              float64   `json:"quota_flash"`
	ResetFlash              time.Time `json:"reset_flash"`
	QuotaFlashlite          float64   `json:"quota_flashlite"`
	ResetFlashlite          time.Time `json:"reset_flashlite"`
	QuotaTab                float64   `json:"quota_tab"`
	ResetTab                time.Time `json:"reset_tab"`
	QuotaImage              float64   `json:"quota_image"`
	ResetImage              time.Time `json:"reset_image"`
	CreditsAmount           float64   `json:"credits_amount"`
	CreditTypes             string    `json:"credit_types"`
	ThrottledUntilClaude    time.Time `json:"throttled_until_claude"`
	ThrottledUntilPro       time.Time `json:"throttled_until_pro"`
	ThrottledUntilFlash     time.Time `json:"throttled_until_flash"`
	ThrottledUntilFlashlite time.Time `json:"throttled_until_flashlite"`
	ThrottledUntilTab       time.Time `json:"throttled_until_tab"`
	ThrottledUntilImage     time.Time `json:"throttled_until_image"`
	SyncedAt                time.Time `json:"synced_at"`
}

type ListAvailableOpenCodeGoRow struct {
	ID             string    `json:"id"`
	AuthCookie     string    `json:"-"`
	Quota5h        float64   `json:"quota_5h"`
	Quota7d        float64   `json:"quota_7d"`
	Quota1mo       float64   `json:"quota_1mo"`
	Reset5h        time.Time `json:"reset_5h"`
	Reset7d        time.Time `json:"reset_7d"`
	Reset1mo       time.Time `json:"reset_1mo"`
	RewardsCount   int       `json:"rewards_count"`
	ThrottledUntil time.Time `json:"throttled_until"`
	SyncedAt       time.Time `json:"synced_at"`
}

type ListOpenCodeGoRow struct {
	ID             string    `json:"id"`
	Status         string    `json:"status"`
	APIKey         string    `json:"-"`
	AuthCookie     string    `json:"-"`
	Reason         string    `json:"reason"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Quota5h        float64   `json:"quota_5h"`
	Quota7d        float64   `json:"quota_7d"`
	Quota1mo       float64   `json:"quota_1mo"`
	Reset5h        time.Time `json:"reset_5h"`
	Reset7d        time.Time `json:"reset_7d"`
	Reset1mo       time.Time `json:"reset_1mo"`
	RewardsCount   int       `json:"rewards_count"`
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

type UpsertAntigravityParams struct {
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

type UpsertOpenCodeGoParams struct {
	ID         string
	Status     string
	APIKey     string
	AuthCookie string
	Reason     string
}

type ModelRow struct {
	Alias     string `json:"alias"`
	Origin    string `json:"origin"`
	Handler   string `json:"handler"`
	PlanTypes string `json:"plan_types"`
	Plugin    string `json:"plugin"`
	ModelScheduling
	Extra json.RawMessage `json:"extra"`
}

type CreateModelParams struct {
	Alias      string
	Origin     string
	Handler    string
	PlanTypes  string
	Plugin     string
	Scheduling ModelScheduling
	Extra      json.RawMessage
}

type UpdateModelParams struct {
	Alias      string
	Origin     string
	Handler    string
	PlanTypes  string
	Plugin     string
	Scheduling ModelScheduling
	Extra      json.RawMessage
}

type LogRow struct {
	Handler      string    `json:"handler"`
	CredentialID string    `json:"credential_id"`
	StatusCode   int32     `json:"status_code"`
	ModelTier    string    `json:"model_tier"`
	Model        string    `json:"model"`
	APIType      string    `json:"api_type"`
	Stream       bool      `json:"stream"`
	FirstByte    float64   `json:"first_byte"`
	Duration     float64   `json:"duration"`
	Error        string    `json:"error"`
	CreatedAt    time.Time `json:"created_at"`
}

type CredentialFilterParams struct {
	Search       string
	Statuses     []string
	PlanType     string
	UnsyncedOnly bool
}

type ListCredentialPagedParams struct {
	Limit  int32
	Offset int32
	CredentialFilterParams
}

type LogFilterParams struct {
	Search        string
	Handler       string
	StatusCode    int32
	HasStatusCode bool
}

type ListLogsParams struct {
	Limit  int32
	Offset int32
	LogFilterParams
}

type AuthKey struct {
	Key       string    `json:"key"`
	Role      string    `json:"role"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

type LogStatusCount struct {
	StatusCode int32 `json:"status_code"`
	Total      int64 `json:"total"`
}

type LogStats struct {
	Total       int64            `json:"total"`
	StatusCodes []LogStatusCount `json:"status_codes"`
}

type LogQueryResult struct {
	Rows          []LogRow
	FilteredStats LogStats
	TotalStats    LogStats
	StatusStats   LogStats
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
	QueryLogs(ctx context.Context, arg ListLogsParams) (LogQueryResult, error)
	ErrorRatesForCredentials(ctx context.Context, handler string, modelTier string, since []ErrorRateSince, minSamples int) (map[string]float64, error)
}

type Store interface {
	CountEnabledCodex(ctx context.Context) (int64, error)
	CountCodex(ctx context.Context) (int64, error)
	CountCodexFiltered(ctx context.Context, filter CredentialFilterParams) (int64, error)
	CountEnabledGeminiCLI(ctx context.Context) (int64, error)
	CountGeminiCLI(ctx context.Context) (int64, error)
	CountGeminiCLIFiltered(ctx context.Context, filter CredentialFilterParams) (int64, error)
	CountEnabledAntigravity(ctx context.Context) (int64, error)
	CountAntigravity(ctx context.Context) (int64, error)
	CountAntigravityFiltered(ctx context.Context, filter CredentialFilterParams) (int64, error)
	CountModels(ctx context.Context) (int64, error)
	CountModelsByHandler(ctx context.Context, handler string) (int64, error)
	CountAuthKeys(ctx context.Context) (int64, error)
	GetCodex(ctx context.Context, id string) (Codex, error)
	UpdateCodexTokens(ctx context.Context, arg UpdateCodexTokensParams) (Codex, error)
	UpdateCodexPlanType(ctx context.Context, id string, planType string) (Codex, error)
	ListCodex(ctx context.Context) ([]ListCodexRow, error)
	ListCodexPaged(ctx context.Context, arg ListCredentialPagedParams) ([]ListCodexRow, error)
	ListCodexPlanTypes(ctx context.Context, filter CredentialFilterParams) ([]string, error)
	CreateCodex(ctx context.Context, arg CreateCodexParams) (Codex, error)
	DeleteCodex(ctx context.Context, id string) error
	UpdateCodexStatus(ctx context.Context, id string, status string, reason string) (Codex, error)
	RestoreExpiredThrottledCodex(ctx context.Context) error
	NextCodexThrottleDeadline(ctx context.Context) (time.Time, error)
	GetGeminiCLI(ctx context.Context, id string) (GeminiCredential, error)
	UpdateGeminiTokens(ctx context.Context, arg UpdateGeminiTokensParams) (GeminiCredential, error)
	UpdateGeminiPlanType(ctx context.Context, id string, planType string) (GeminiCredential, error)
	ListGeminiCLI(ctx context.Context) ([]ListGeminiCLIRow, error)
	ListGeminiCLIPaged(ctx context.Context, arg ListCredentialPagedParams) ([]ListGeminiCLIRow, error)
	ListGeminiCLIPlanTypes(ctx context.Context, filter CredentialFilterParams) ([]string, error)
	UpsertGeminiCLI(ctx context.Context, arg UpsertGeminiCLIParams) (GeminiCredential, error)
	DeleteGeminiCLI(ctx context.Context, id string) error
	UpdateGeminiCLIStatus(ctx context.Context, id string, status string, reason string) (GeminiCredential, error)
	RestoreExpiredThrottledGeminiCLI(ctx context.Context) error
	NextGeminiThrottleDeadline(ctx context.Context) (time.Time, error)
	GetAntigravity(ctx context.Context, id string) (AntigravityCredential, error)
	UpdateAntigravityTokens(ctx context.Context, arg UpdateAntigravityTokensParams) (AntigravityCredential, error)
	ListAntigravityPaged(ctx context.Context, arg ListCredentialPagedParams) ([]ListAntigravityRow, error)
	ListAntigravityPlanTypes(ctx context.Context, filter CredentialFilterParams) ([]string, error)
	UpsertAntigravity(ctx context.Context, arg UpsertAntigravityParams) (AntigravityCredential, error)
	DeleteAntigravity(ctx context.Context, id string) error
	UpdateAntigravityStatus(ctx context.Context, id string, status string, reason string) (AntigravityCredential, error)
	RestoreExpiredThrottledAntigravity(ctx context.Context) error
	NextAntigravityThrottleDeadline(ctx context.Context) (time.Time, error)
	CountEnabledOpenCodeGo(ctx context.Context) (int64, error)
	CountOpenCodeGo(ctx context.Context) (int64, error)
	CountOpenCodeGoFiltered(ctx context.Context, filter CredentialFilterParams) (int64, error)
	GetOpenCodeGo(ctx context.Context, id string) (OpenCodeGoCredential, error)
	ListOpenCodeGo(ctx context.Context) ([]ListOpenCodeGoRow, error)
	ListOpenCodeGoPaged(ctx context.Context, arg ListCredentialPagedParams) ([]ListOpenCodeGoRow, error)
	UpsertOpenCodeGo(ctx context.Context, arg UpsertOpenCodeGoParams) (OpenCodeGoCredential, error)
	DeleteOpenCodeGo(ctx context.Context, id string) error
	UpdateOpenCodeGoStatus(ctx context.Context, id string, status string, reason string) (OpenCodeGoCredential, error)
	RestoreExpiredThrottledOpenCodeGo(ctx context.Context) error
	NextOpenCodeGoThrottleDeadline(ctx context.Context) (time.Time, error)

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
	ListAvailableAntigravity(ctx context.Context) ([]ListAvailableAntigravityRow, error)
	ListAvailableOpenCodeGo(ctx context.Context) ([]ListAvailableOpenCodeGoRow, error)

	UpsertGeminiQuota(ctx context.Context, arg UpsertGeminiQuotaParams) error
	SetGeminiQuotaThrottled(ctx context.Context, credentialID string, modelTier string, throttledUntil time.Time) error
	DeleteGeminiQuota(ctx context.Context, credentialID string) error
	UpsertAntigravityQuota(ctx context.Context, arg UpsertAntigravityQuotaParams) error
	SetAntigravityQuotaThrottled(ctx context.Context, credentialID string, modelTier string, throttledUntil time.Time) error
	DeleteAntigravityQuota(ctx context.Context, credentialID string) error
	UpsertOpenCodeGoQuota(ctx context.Context, arg UpsertOpenCodeGoQuotaParams) error
	SetOpenCodeGoQuotaThrottled(ctx context.Context, credentialID string, throttledUntil time.Time) error
	DeleteOpenCodeGoQuota(ctx context.Context, credentialID string) error
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
