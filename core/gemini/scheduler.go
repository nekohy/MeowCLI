package gemini

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	geminiapi "github.com/nekohy/MeowCLI/api/gemini"
	"github.com/nekohy/MeowCLI/core/scheduling"
	"github.com/nekohy/MeowCLI/internal/settings"
	db "github.com/nekohy/MeowCLI/internal/store"
	"github.com/nekohy/MeowCLI/utils"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/singleflight"
)

var ErrNoAvailableCredential = fmt.Errorf("no available gemini credential")

const quotaRefreshFailureBackoff = time.Minute

// SchedulerStore describes the SQL operations the scheduler depends on.
type SchedulerStore interface {
	ListAvailableGeminiCLI(ctx context.Context) ([]db.ListAvailableGeminiCLIRow, error)
	UpsertGeminiQuota(ctx context.Context, arg db.UpsertGeminiQuotaParams) error
	SetGeminiQuotaThrottled(ctx context.Context, credentialID string, modelTier string, throttledUntil time.Time) error
	UpdateGeminiCLIStatus(ctx context.Context, id string, status string, reason string) (db.GeminiCredential, error)
	UpdateGeminiPlanType(ctx context.Context, id string, planType string) (db.GeminiCredential, error)
}

type planFetcher interface {
	LoadCodeAssistPlan(ctx context.Context, accessToken string, projectID string) (string, error)
}

// throttleState tracks exponential backoff state per credential in memory.
type throttleState struct {
	consecutive int
	lastFail    time.Time
	until       time.Time
}

// availableRow is a single credential in cache; each model tier keeps an independent quota score.
type availableRow struct {
	ID              string
	PlanTypeCode    int
	QuotaPro        float64
	QuotaFlash      float64
	QuotaFlashLite  float64
	ScorePro        float64 // score when serving Pro models
	ScoreFlash      float64 // score when serving Flash models
	ScoreFlashLite  float64 // score when serving FlashLite models
	ResetPro        time.Time
	ResetFlash      time.Time
	ResetFlashLite  time.Time
	WeightPro       float64
	WeightFlash     float64
	WeightFlashlite float64
}

// Scheduler selects the best available credential based on quota ratios and reset time
// priority, and manages throttling/backoff for failed requests.
type Scheduler struct {
	store    SchedulerStore
	logStore db.LogStore
	manager  *Manager
	fetcher  geminiapi.QuotaFetcher
	planAPI  planFetcher
	settings settings.Provider

	mu               sync.Mutex
	throttle         map[string]*throttleState
	checking         map[string]struct{}
	quotaRefreshing  map[string]struct{}
	verifyCredential func(string)
	importSyncMu     sync.Mutex
	available        atomic.Pointer[[]availableRow]
	planTypes        *planTypeCodec
	quotaRefreshSem  chan struct{}
	refreshGroup     singleflight.Group
}

// NewScheduler creates a scheduler connected to the given store and token manager.
func NewScheduler(store SchedulerStore, manager *Manager) *Scheduler {
	return &Scheduler{
		store:           store,
		manager:         manager,
		throttle:        make(map[string]*throttleState),
		checking:        make(map[string]struct{}),
		quotaRefreshing: make(map[string]struct{}),
		planTypes:       newPlanTypeCodec(),
		quotaRefreshSem: make(chan struct{}, 8),
	}
}

// SetQuotaFetcher sets the quota fetcher used for background quota synchronization.
func (s *Scheduler) SetQuotaFetcher(fetcher geminiapi.QuotaFetcher) {
	if s == nil {
		return
	}
	s.fetcher = fetcher
	if planAPI, ok := fetcher.(planFetcher); ok {
		s.planAPI = planAPI
	}
}

func (s *Scheduler) SetSettingsProvider(provider settings.Provider) {
	if s == nil {
		return
	}
	s.settings = provider
}

func (s *Scheduler) SetLogStore(store db.LogStore) {
	if s == nil {
		return
	}
	s.logStore = store
}

// StartQuotaSyncer launches a background goroutine that periodically fetches
// quotas from the upstream API and writes them to the quota table.
// It stops when ctx is cancelled.
func (s *Scheduler) StartQuotaSyncer(ctx context.Context) {
	s.startScoreRefresh(ctx)
	s.quotaSyncer().Start(ctx)
}

func (s *Scheduler) startScoreRefresh(ctx context.Context) {
	if s == nil {
		return
	}
	scheduling.ScoreRefreshLoop{
		Interval:        func() time.Duration { return s.settingsSnapshot().ScoreRefreshInterval() },
		DefaultInterval: settings.DefaultSnapshot().ScoreRefreshInterval(),
		Refresh:         s.refreshAvailableScores,
	}.Start(ctx)
}

func (s *Scheduler) quotaSyncer() scheduling.QuotaSyncer[db.ListAvailableGeminiCLIRow] {
	return scheduling.QuotaSyncer[db.ListAvailableGeminiCLIRow]{
		SyncInterval: func() time.Duration {
			return s.settingsSnapshot().QuotaSyncInterval()
		},
		List: s.store.ListAvailableGeminiCLI,
		CacheRows: func(ctx context.Context, rows []db.ListAvailableGeminiCLIRow) {
			s.refreshAvailableFromRows(ctx, rows)
		},
		Sync:     s.syncQuotaRow,
		RowID:    func(row db.ListAvailableGeminiCLIRow) string { return row.ID },
		SyncedAt: func(row db.ListAvailableGeminiCLIRow) time.Time { return row.SyncedAt },
		ResetAt: func(row db.ListAvailableGeminiCLIRow) time.Time {
			return scheduling.EarliestTime(row.ResetPro, row.ResetFlash, row.ResetFlashlite)
		},
		WithSyncedAt: func(row db.ListAvailableGeminiCLIRow, syncedAt time.Time) db.ListAvailableGeminiCLIRow {
			row.SyncedAt = syncedAt
			return row
		},
		ReportError: func(err error, message string) {
			log.Error().Err(err).Msg(message)
		},
		WarmErrorMessage:    "gemini quota-sync: warm available cache",
		ListErrorMessage:    "gemini quota-sync: list credentials",
		RefreshErrorMessage: "gemini quota-sync: refresh available cache",
	}
}

func (s *Scheduler) syncQuotaRow(ctx context.Context, row db.ListAvailableGeminiCLIRow) {
	if s.fetcher == nil {
		return
	}

	token, err := s.manager.AccessToken(ctx, row.ID, scheduling.UseCached)
	if err != nil {
		log.Error().Err(err).Str("credential", row.ID).Msg("gemini quota-sync: get token")
		return
	}

	quotaCtx, cancel := context.WithTimeout(ctx, s.settingsSnapshot().ImportedCheckTimeout())
	q, err := s.fetcher.FetchQuota(quotaCtx, row.ID, token, row.ProjectID)
	if err != nil {
		cancel()
		if statusCode, body, ok := geminiapi.ParseAPIError(err); ok && isCredentialRejectedStatus(statusCode) {
			s.HandleUnauthorized(ctx, row.ID, int32(statusCode), "", db.LogRequestMetrics{Error: body})
			return
		}
		log.Error().Err(err).Str("credential", row.ID).Msg("gemini quota-sync: fetch quota")
		return
	}
	s.refreshPlanType(quotaCtx, row.ID, token, row.ProjectID, row.PlanType)
	cancel()

	log.Info().
		Str("credential", row.ID).
		Float64("quota_pro", q.QuotaPro).
		Float64("quota_flash", q.QuotaFlash).
		Float64("quota_flashlite", q.QuotaFlashlite).
		Time("reset_pro", q.ResetPro).
		Time("reset_flash", q.ResetFlash).
		Time("reset_flashlite", q.ResetFlashlite).
		Msg("gemini quota-sync: fetched")

	s.UpdateQuota(ctx, row.ID, q)
}

// SelectCredential selects the best available credential based on priority scoring.
// The caller can pass a preferred credential ID; if unavailable, no other
// credential is selected.
func (s *Scheduler) SelectCredential(ctx context.Context, selection scheduling.CredentialSelection) (string, error) {
	codec := s.planTypeCodec()
	return s.selectCredential(ctx, s.preferredPlanTypeCodes(selection.Headers), codec.codesFor(selection.AllowedPlanTypes), selection.PreferredCredentialID, selection.ModelTier)
}

func (s *Scheduler) selectCredential(ctx context.Context, preferredCodes []int, allowedCodes []int, preferredCredentialID string, modelTier string) (string, error) {
	rows, err := s.listAvailable(ctx)
	if err != nil {
		return "", err
	}

	allowed := scheduling.PlanTypeCodeSet(allowedCodes)
	if preferredCredentialID != "" {
		for _, row := range rows {
			if row.ID == preferredCredentialID && weightedTierScore(row, modelTier) >= 0 && scheduling.PlanTypeAllowed(row.PlanTypeCode, allowed) {
				return preferredCredentialID, nil
			}
		}
		return "", ErrNoAvailableCredential
	}

	for _, code := range preferredCodes {
		if !scheduling.PlanTypeAllowed(code, allowed) {
			continue
		}
		if row, ok := selectWeightedCredential(rows, modelTier, func(row availableRow) bool {
			return row.PlanTypeCode == code
		}); ok {
			return row.ID, nil
		}
	}

	if row, ok := selectWeightedCredential(rows, modelTier, func(row availableRow) bool {
		return scheduling.PlanTypeAllowed(row.PlanTypeCode, allowed)
	}); ok {
		return row.ID, nil
	}

	return "", ErrNoAvailableCredential
}

func selectWeightedCredential(rows []availableRow, modelTier string, match func(availableRow) bool) (availableRow, bool) {
	return scheduling.PickWeightedFromBest(rows, scheduling.DefaultWeightedBestCount, func(row availableRow) float64 {
		if !match(row) {
			return -1
		}
		return weightedTierScore(row, modelTier)
	})
}

func weightedTierScore(row availableRow, modelTier string) float64 {
	switch modelTier {
	case ModelTierPro:
		return scheduling.AdjustedScore(row.ScorePro, row.WeightPro)
	case ModelTierFlash:
		return scheduling.AdjustedScore(row.ScoreFlash, row.WeightFlash)
	case ModelTierFlashLite:
		return scheduling.AdjustedScore(row.ScoreFlashLite, row.WeightFlashlite)
	default:
		return -1
	}
}

// listAvailable returns the cached available credential list.
// The cache is initialized by RefreshAvailable and updated by
// UpdateQuota / evictCredential events; no TTL expiry is used.
func (s *Scheduler) listAvailable(ctx context.Context) ([]availableRow, error) {
	if rows := s.available.Load(); rows != nil {
		if s.hasExpiredThrottleWindow(time.Now()) {
			if _, err := s.RefreshAvailable(ctx); err == nil {
				if refreshed := s.available.Load(); refreshed != nil {
					return *refreshed, nil
				}
			} else {
				log.Error().Err(err).Msg("gemini scheduler: refresh available after throttle expiry")
			}
		}
		return *rows, nil
	}

	// Cache is empty on first call; load from DB.
	if _, err := s.RefreshAvailable(ctx); err != nil {
		return nil, err
	}
	if rows := s.available.Load(); rows != nil {
		return *rows, nil
	}
	return nil, nil
}

// RefreshAvailable reloads all available credentials from DB and refreshes the in-memory cache.
// Called at startup by StartQuotaSyncer and manually when a full cache rebuild is needed.
func (s *Scheduler) RefreshAvailable(ctx context.Context) ([]availableRow, error) {
	value, err, _ := s.refreshGroup.Do("available", func() (any, error) {
		dbRows, err := s.store.ListAvailableGeminiCLI(ctx)
		if err != nil {
			return nil, fmt.Errorf("list available gemini cli: %w", err)
		}

		return s.refreshAvailableFromRows(ctx, dbRows), nil
	})
	if err != nil {
		return nil, err
	}
	rows, _ := value.([]availableRow)
	return rows, nil
}

func (s *Scheduler) refreshAvailableFromRows(ctx context.Context, dbRows []db.ListAvailableGeminiCLIRow) []availableRow {
	config := s.settingsSnapshot()
	planTypes := s.planTypeCodec()
	ws := config.QuotaWindowGeminiSeconds()
	rows := make([]availableRow, 0, len(dbRows))
	now := time.Now()
	for _, r := range dbRows {
		if r.SyncedAt.IsZero() {
			continue
		}
		if s.isCredentialUnderValidation(r.ID) {
			continue
		}
		row := availableRow{
			ID:              r.ID,
			PlanTypeCode:    planTypes.code(r.PlanType),
			QuotaPro:        r.QuotaPro,
			QuotaFlash:      r.QuotaFlash,
			QuotaFlashLite:  r.QuotaFlashlite,
			ScorePro:        CalcScore(r.QuotaPro, r.QuotaFlash, r.QuotaFlashlite, r.ResetPro, r.ResetFlash, r.ResetFlashlite, ModelTierPro, ws),
			ScoreFlash:      CalcScore(r.QuotaPro, r.QuotaFlash, r.QuotaFlashlite, r.ResetPro, r.ResetFlash, r.ResetFlashlite, ModelTierFlash, ws),
			ScoreFlashLite:  CalcScore(r.QuotaPro, r.QuotaFlash, r.QuotaFlashlite, r.ResetPro, r.ResetFlash, r.ResetFlashlite, ModelTierFlashLite, ws),
			ResetPro:        r.ResetPro,
			ResetFlash:      r.ResetFlash,
			ResetFlashLite:  r.ResetFlashlite,
			WeightPro:       1.0,
			WeightFlash:     1.0,
			WeightFlashlite: 1.0,
		}
		if !r.ThrottledUntilPro.IsZero() && now.Before(r.ThrottledUntilPro) {
			row.ScorePro = -1
		}
		if !r.ThrottledUntilFlash.IsZero() && now.Before(r.ThrottledUntilFlash) {
			row.ScoreFlash = -1
		}
		if !r.ThrottledUntilFlashlite.IsZero() && now.Before(r.ThrottledUntilFlashlite) {
			row.ScoreFlashLite = -1
		}
		rows = append(rows, row)
	}

	s.computeErrorRates(ctx, rows)

	s.available.Store(&rows)
	s.clearExpiredThrottles(time.Now())
	s.manager.PruneStaleEntries()
	return rows
}

func (s *Scheduler) applyQuotaToAvailable(id string, q *geminiapi.Quota) bool {
	config := s.settingsSnapshot()
	ws := config.QuotaWindowGeminiSeconds()
	newScorePro := CalcScore(q.QuotaPro, q.QuotaFlash, q.QuotaFlashlite, q.ResetPro, q.ResetFlash, q.ResetFlashlite, ModelTierPro, ws)
	newScoreFlash := CalcScore(q.QuotaPro, q.QuotaFlash, q.QuotaFlashlite, q.ResetPro, q.ResetFlash, q.ResetFlashlite, ModelTierFlash, ws)
	newScoreFlashLite := CalcScore(q.QuotaPro, q.QuotaFlash, q.QuotaFlashlite, q.ResetPro, q.ResetFlash, q.ResetFlashlite, ModelTierFlashLite, ws)

	for {
		snap := s.available.Load()
		if snap == nil {
			return false
		}

		updated := make([]availableRow, len(*snap))
		copy(updated, *snap)

		found := false
		for i := range updated {
			if updated[i].ID != id {
				continue
			}
			if updated[i].ScorePro < 0 && newScorePro >= 0 {
				updated[i].WeightPro = 1.0
			}
			if updated[i].ScoreFlash < 0 && newScoreFlash >= 0 {
				updated[i].WeightFlash = 1.0
			}
			if updated[i].ScoreFlashLite < 0 && newScoreFlashLite >= 0 {
				updated[i].WeightFlashlite = 1.0
			}
			updated[i].QuotaPro = q.QuotaPro
			updated[i].QuotaFlash = q.QuotaFlash
			updated[i].QuotaFlashLite = q.QuotaFlashlite
			updated[i].ScorePro = newScorePro
			updated[i].ScoreFlash = newScoreFlash
			updated[i].ScoreFlashLite = newScoreFlashLite
			updated[i].ResetPro = q.ResetPro
			updated[i].ResetFlash = q.ResetFlash
			updated[i].ResetFlashLite = q.ResetFlashlite
			found = true
			break
		}
		if !found {
			return false
		}

		next := updated
		if s.available.CompareAndSwap(snap, &next) {
			return true
		}
	}
}

func (s *Scheduler) refreshAvailableScores() {
	config := s.settingsSnapshot()
	ws := config.QuotaWindowGeminiSeconds()
	scheduling.RefreshRows(
		s.available.Load,
		s.available.CompareAndSwap,
		func(snap *[]availableRow) []availableRow {
			rows := make([]availableRow, len(*snap))
			copy(rows, *snap)
			return rows
		},
		func(rows []availableRow) *[]availableRow {
			return &rows
		},
		func(row *availableRow) {
			if row.ScorePro >= 0 {
				row.ScorePro = CalcScore(row.QuotaPro, row.QuotaFlash, row.QuotaFlashLite, row.ResetPro, row.ResetFlash, row.ResetFlashLite, ModelTierPro, ws)
			}
			if row.ScoreFlash >= 0 {
				row.ScoreFlash = CalcScore(row.QuotaPro, row.QuotaFlash, row.QuotaFlashLite, row.ResetPro, row.ResetFlash, row.ResetFlashLite, ModelTierFlash, ws)
			}
			if row.ScoreFlashLite >= 0 {
				row.ScoreFlashLite = CalcScore(row.QuotaPro, row.QuotaFlash, row.QuotaFlashLite, row.ResetPro, row.ResetFlash, row.ResetFlashLite, ModelTierFlashLite, ws)
			}
		},
	)
}

func (s *Scheduler) evictCredential(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	snap := s.available.Load()
	if snap == nil {
		return
	}

	filtered := make([]availableRow, 0, len(*snap))
	for _, r := range *snap {
		if r.ID != id {
			filtered = append(filtered, r)
		}
	}

	s.available.Store(&filtered)
}

func (s *Scheduler) suspendCredentialTier(id string, modelTier string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	snap := s.available.Load()
	if snap == nil {
		return
	}

	updated := make([]availableRow, len(*snap))
	copy(updated, *snap)
	for i := range updated {
		if updated[i].ID != id {
			continue
		}
		switch modelTier {
		case ModelTierPro:
			updated[i].ScorePro = -1
		case ModelTierFlash:
			updated[i].ScoreFlash = -1
		case ModelTierFlashLite:
			updated[i].ScoreFlashLite = -1
		default:
			return
		}
		break
	}

	s.available.Store(&updated)
}

func (s *Scheduler) startQuotaRefresh(id string, modelTier string) bool {
	if s == nil || id == "" {
		return false
	}
	key := quotaRefreshKey(id, modelTier)

	s.mu.Lock()
	if s.quotaRefreshing == nil {
		s.quotaRefreshing = make(map[string]struct{})
	}
	if _, ok := s.quotaRefreshing[key]; ok {
		s.mu.Unlock()
		return false
	}
	s.quotaRefreshing[key] = struct{}{}
	s.mu.Unlock()

	if modelTier != "" {
		s.suspendCredentialTier(id, modelTier)
	} else {
		s.evictCredential(id)
	}
	return true
}

func (s *Scheduler) completeQuotaRefresh(id string, modelTier string) {
	if s == nil || id == "" {
		return
	}
	key := quotaRefreshKey(id, modelTier)
	s.mu.Lock()
	delete(s.quotaRefreshing, key)
	s.mu.Unlock()
}

func quotaRefreshKey(id string, modelTier string) string {
	return id + "\x00" + modelTier
}

func (s *Scheduler) AuthHeaders(ctx context.Context, credentialID string) (http.Header, error) {
	return s.manager.AuthHeaders(ctx, credentialID, scheduling.UseCached)
}

func (s *Scheduler) ProjectID(ctx context.Context, credentialID string) (string, error) {
	if s == nil || s.manager == nil {
		return "", fmt.Errorf("gemini scheduler manager is unavailable")
	}
	return s.manager.GetProjectID(ctx, credentialID)
}

func (s *Scheduler) InvalidateCredential(credentialID string) {
	if s == nil || s.manager == nil || credentialID == "" {
		return
	}
	s.manager.InvalidateCredential(credentialID)
}

func (s *Scheduler) planTypeCodec() *planTypeCodec {
	if s.planTypes == nil {
		s.planTypes = newPlanTypeCodec()
	}
	return s.planTypes
}

func (s *Scheduler) preferredPlanTypeCodes(headers http.Header) []int {
	snapshot := s.settingsSnapshot()
	codec := s.planTypeCodec()

	return scheduling.MergePlanTypeCodes(
		headerPlanTypeCodes(headers, snapshot.GeminiAllowUserPlanTypeHeader, codec),
		codec.codesFor(ParsePlanTypeList(snapshot.GeminiPreferredPlanTypes)),
	)
}

func headerPlanTypeCodes(headers http.Header, enabled bool, codec *planTypeCodec) []int {
	if !enabled {
		return nil
	}
	return codec.codesFor(ParsePlanTypeList(strings.Join(headers.Values(utils.HeaderPlanTypePreference), ",")))
}

// CalcScore computes a priority score for a specific model tier.
// Only the relevant tier's quota is checked; other tiers are ignored.
// If the requested tier's quota is exhausted (0%), returns -1 (unusable for that tier).
func CalcScore(quotaPro, quotaFlash, quotaFlashlite float64, resetPro, resetFlash, resetFlashlite time.Time, modelTier string, windowSeconds int64) float64 {
	now := time.Now()
	switch modelTier {
	case ModelTierPro:
		if quotaPro == 0 {
			return -1
		}
		return scheduling.QuotaPressureScore(quotaPro, resetPro, now, windowSeconds)
	case ModelTierFlash:
		if quotaFlash == 0 {
			return -1
		}
		return scheduling.QuotaPressureScore(quotaFlash, resetFlash, now, windowSeconds)
	case ModelTierFlashLite:
		if quotaFlashlite == 0 {
			return -1
		}
		return scheduling.QuotaPressureScore(quotaFlashlite, resetFlashlite, now, windowSeconds)
	default:
		return -1
	}
}

func ErrorRateSince(resetAt time.Time, windowSeconds int64) time.Time {
	return scheduling.WindowStart(resetAt, windowSeconds)
}

// RecordSuccess records a successful request and resets the backoff state.
func (s *Scheduler) RecordSuccess(_ context.Context, credentialID string, statusCode int32, modelTier string, metrics db.LogRequestMetrics) {
	s.mu.Lock()
	delete(s.throttle, credentialID)
	s.mu.Unlock()

	if err := s.recordResponse(context.Background(), credentialID, statusCode, modelTier, metrics); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("gemini scheduler: insert success log")
	} else {
		s.refreshCredentialWeight(context.Background(), credentialID, modelTier)
	}
}

// RecordFailure records the response for error-rate weighting.
// Throttling is reserved for explicit 429 retry windows or repeated consecutive failures.
func (s *Scheduler) RecordFailure(_ context.Context, credentialID string, statusCode int32, modelTier string, retryAfter time.Duration, metrics db.LogRequestMetrics) {
	bgCtx := context.Background()

	if err := s.recordResponse(bgCtx, credentialID, statusCode, modelTier, metrics); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("gemini scheduler: insert failure log")
	} else {
		s.refreshCredentialWeight(bgCtx, credentialID, modelTier)
	}

	now := time.Now()

	s.mu.Lock()
	state, ok := s.throttle[credentialID]
	if !ok {
		state = &throttleState{}
		s.throttle[credentialID] = state
	}
	if !state.until.IsZero() && !now.Before(state.until) {
		state.consecutive = 0
	}
	state.consecutive++
	state.lastFail = now
	consecutive := state.consecutive
	s.mu.Unlock()

	config := s.settingsSnapshot()
	decision := scheduling.DecideFailureThrottle(statusCode, retryAfter, consecutive, config.ThrottleBase(), config.ThrottleMax())
	if !decision.Throttle {
		return
	}

	throttleTier := ""
	if decision.ExplicitRetryAfter {
		throttleTier = modelTier
	}
	throttledUntil := now.Add(decision.Backoff)
	if err := s.store.SetGeminiQuotaThrottled(bgCtx, credentialID, throttleTier, throttledUntil); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("gemini scheduler: set throttled")
	}
	s.rememberThrottleUntil(credentialID, throttledUntil)

	if decision.ExplicitRetryAfter && modelTier != "" {
		s.suspendCredentialTier(credentialID, modelTier)
		log.Warn().Str("credential", credentialID).Str("model_tier", modelTier).Dur("backoff", decision.Backoff).Str("reason", decision.Reason).Msg("gemini credential tier throttled")
		return
	}

	// Credential is throttled; remove from cache.
	s.evictCredential(credentialID)
	log.Warn().Str("credential", credentialID).Dur("backoff", decision.Backoff).Str("reason", decision.Reason).Int("consecutive_failures", consecutive).Msg("gemini credential throttled")
}

// HandleUnauthorized handles auth/account terminal statuses outside the error-rate backoff path.
func (s *Scheduler) HandleUnauthorized(ctx context.Context, credentialID string, statusCode int32, modelTier string, metrics db.LogRequestMetrics) bool {
	if isDirectDisableStatus(int(statusCode)) {
		s.recordAuthRejection(context.Background(), credentialID, statusCode, modelTier, metrics)
		s.mu.Lock()
		delete(s.throttle, credentialID)
		delete(s.checking, credentialID)
		s.mu.Unlock()
		s.DisableCredential(context.Background(), credentialID, fmt.Sprintf("credential rejected (%d)", statusCode))
		return true
	}
	if !isRefreshableAuthStatus(int(statusCode)) {
		return false
	}

	s.mu.Lock()
	delete(s.throttle, credentialID)
	_, alreadyChecking := s.checking[credentialID]
	if !alreadyChecking {
		s.checking[credentialID] = struct{}{}
	}
	s.mu.Unlock()

	s.evictCredential(credentialID)
	if alreadyChecking {
		log.Warn().
			Str("credential", credentialID).
			Int32("status", statusCode).
			Msg("gemini credential already under refresh verification after auth rejection response")
		return true
	}

	log.Warn().
		Str("credential", credentialID).
		Int32("status", statusCode).
		Msg("gemini credential removed from available pool after auth rejection response")

	if s.verifyCredential != nil {
		go s.verifyCredential(credentialID)
		return true
	}
	if s.manager == nil {
		s.recordAuthRejection(context.Background(), credentialID, statusCode, modelTier, metrics)
		log.Warn().
			Str("credential", credentialID).
			Int32("status", statusCode).
			Msg("gemini credential verification skipped because manager is unavailable")
		return true
	}
	go s.validateCredentialAfterAuthFailure(credentialID, statusCode, modelTier, metrics)
	return true
}

// UpdateQuota updates credential quota from an API response (writes to DB + refreshes cache).
func (s *Scheduler) UpdateQuota(ctx context.Context, credentialID string, q *geminiapi.Quota) {
	if q == nil {
		return
	}
	cacheUpdated := s.applyQuotaToAvailable(credentialID, q)
	if err := s.store.UpsertGeminiQuota(ctx, db.UpsertGeminiQuotaParams{
		CredentialID:   credentialID,
		QuotaPro:       q.QuotaPro,
		ResetPro:       q.ResetPro,
		QuotaFlash:     q.QuotaFlash,
		ResetFlash:     q.ResetFlash,
		QuotaFlashlite: q.QuotaFlashlite,
		ResetFlashlite: q.ResetFlashlite,
	}); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("gemini scheduler: upsert quota")
		return
	}
	if !cacheUpdated {
		if _, err := s.RefreshAvailable(ctx); err != nil {
			log.Error().Err(err).Str("credential", credentialID).Msg("gemini scheduler: refresh available after quota insert")
		}
	}
}

func (s *Scheduler) refreshPlanType(ctx context.Context, credentialID, accessToken, projectID, fallback string) {
	if s == nil || s.planAPI == nil || s.store == nil || credentialID == "" {
		return
	}
	current := NormalizePlanType(fallback)
	currentKnown := current != ""
	if current == "" {
		current = PlanTypeFree
	}
	loaded, err := s.planAPI.LoadCodeAssistPlan(ctx, accessToken, projectID)
	if err != nil {
		log.Warn().Err(err).Str("credential", credentialID).Msg("gemini quota-sync: load plan")
		return
	}
	planType := NormalizePlanType(loaded)
	if planType == "" || (currentKnown && planType == current) {
		return
	}
	if _, err := s.store.UpdateGeminiPlanType(ctx, credentialID, planType); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Str("plan_type", planType).Msg("gemini quota-sync: update plan type")
		return
	}
	if !s.applyPlanTypeToAvailable(credentialID, planType) {
		if _, err := s.RefreshAvailable(ctx); err != nil {
			log.Error().Err(err).Str("credential", credentialID).Msg("gemini quota-sync: refresh available after plan update")
		}
	}
}

func (s *Scheduler) applyPlanTypeToAvailable(id string, planType string) bool {
	code := s.planTypeCodec().code(planType)
	if code == planTypeCodeUnknown {
		return false
	}
	for {
		snap := s.available.Load()
		if snap == nil {
			return false
		}

		updated := make([]availableRow, len(*snap))
		copy(updated, *snap)

		found := false
		for i := range updated {
			if updated[i].ID != id {
				continue
			}
			updated[i].PlanTypeCode = code
			found = true
			break
		}
		if !found {
			return false
		}

		next := updated
		if s.available.CompareAndSwap(snap, &next) {
			return true
		}
	}
}

// QueueQuotaRefresh temporarily removes the credential tier from the in-memory
// scheduling pool and refreshes quota in the background without writing DB state.
func (s *Scheduler) QueueQuotaRefresh(_ context.Context, credentialID string, modelTier string) {
	if s == nil || credentialID == "" || s.manager == nil || s.fetcher == nil {
		return
	}
	select {
	case s.quotaRefreshSem <- struct{}{}:
	default:
		log.Warn().Str("credential", credentialID).Str("model_tier", modelTier).Msg("gemini scheduler: quota refresh skipped: concurrent limit reached")
		return
	}
	if !s.startQuotaRefresh(credentialID, modelTier) {
		<-s.quotaRefreshSem
		return
	}

	go func() {
		defer func() { <-s.quotaRefreshSem }()
		defer s.completeQuotaRefresh(credentialID, modelTier)

		refreshCtx, cancel := context.WithTimeout(context.Background(), s.settingsSnapshot().ImportedCheckTimeout())
		defer cancel()

		token, err := s.manager.AccessToken(refreshCtx, credentialID, scheduling.UseCached)
		if err != nil {
			s.rememberThrottleUntil(credentialID, time.Now().Add(quotaRefreshFailureBackoff))
			log.Warn().Err(err).Str("credential", credentialID).Str("model_tier", modelTier).Msg("gemini scheduler: quota refresh get token")
			return
		}

		projectID, err := s.manager.GetProjectID(refreshCtx, credentialID)
		if err != nil {
			s.rememberThrottleUntil(credentialID, time.Now().Add(quotaRefreshFailureBackoff))
			log.Warn().Err(err).Str("credential", credentialID).Str("model_tier", modelTier).Msg("gemini scheduler: quota refresh get project")
			return
		}

		q, err := s.fetcher.FetchQuota(refreshCtx, credentialID, token, projectID)
		if err != nil {
			s.rememberThrottleUntil(credentialID, time.Now().Add(quotaRefreshFailureBackoff))
			log.Warn().Err(err).Str("credential", credentialID).Str("model_tier", modelTier).Msg("gemini scheduler: quota refresh fetch")
			return
		}
		if q == nil {
			s.rememberThrottleUntil(credentialID, time.Now().Add(quotaRefreshFailureBackoff))
			log.Warn().Str("credential", credentialID).Str("model_tier", modelTier).Msg("gemini scheduler: quota refresh returned empty quota")
			return
		}

		s.applyQuotaToAvailable(credentialID, q)
	}()
}

// SyncCredentials asynchronously performs an initial quota check for newly imported credentials.
func (s *Scheduler) SyncCredentials(_ context.Context, ids []string) {
	if s.fetcher == nil || s.manager == nil || len(ids) == 0 {
		return
	}

	ids = append([]string(nil), ids...)
	go func(ids []string) {
		s.importSyncMu.Lock()
		defer s.importSyncMu.Unlock()

		for _, id := range ids {
			validationCtx, cancel := context.WithTimeout(context.Background(), s.settingsSnapshot().ImportedCheckTimeout())
			s.syncImportedCredential(validationCtx, id)
			cancel()
		}

		if _, err := s.RefreshAvailable(context.Background()); err != nil {
			log.Error().Err(err).Msg("gemini sync-credentials: refresh available cache")
		}
	}(ids)
}

func (s *Scheduler) hasExpiredThrottleWindow(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, state := range s.throttle {
		if !state.until.IsZero() && !now.Before(state.until) {
			return true
		}
	}
	return false
}

func (s *Scheduler) clearExpiredThrottles(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, state := range s.throttle {
		if state.until.IsZero() || now.Before(state.until) {
			continue
		}
		delete(s.throttle, id)
	}
}

func (s *Scheduler) rememberThrottleUntil(credentialID string, throttledUntil time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.throttle[credentialID]
	if !ok {
		state = &throttleState{}
		s.throttle[credentialID] = state
	}
	state.until = throttledUntil
}

func (s *Scheduler) isCredentialUnderValidation(credentialID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.checking[credentialID]
	return ok
}

func (s *Scheduler) finishCredentialValidation(credentialID string) {
	s.mu.Lock()
	delete(s.checking, credentialID)
	s.mu.Unlock()
}

func (s *Scheduler) validateCredentialAfterAuthFailure(credentialID string, statusCode int32, modelTier string, metrics db.LogRequestMetrics) {
	ctx, cancel := context.WithTimeout(context.Background(), s.settingsSnapshot().UnauthorizedCheckTimeout())
	defer cancel()
	defer s.finishCredentialValidation(credentialID)

	err := s.manager.RefreshCredential(ctx, credentialID)
	switch {
	case err == nil:
		log.Info().Str("credential", credentialID).Msg("gemini credential refresh verification succeeded after auth rejection response")
	default:
		if statusCode == http.StatusUnauthorized {
			s.recordAuthRejection(ctx, credentialID, statusCode, modelTier, metrics)
		}
		s.DisableCredential(ctx, credentialID, fmt.Sprintf("refresh verification failed after auth rejection (%d)", statusCode))
		log.Warn().Err(err).Str("credential", credentialID).Msg("gemini credential refresh verification finished with error after auth rejection response")
	}
	if err == nil {
		s.verifyCredentialUsable(ctx, credentialID)
	}

	if _, refreshErr := s.RefreshAvailable(context.Background()); refreshErr != nil {
		log.Error().Err(refreshErr).Msg("gemini scheduler: refresh available after unauthorized verification")
	}
}

func (s *Scheduler) verifyCredentialUsable(ctx context.Context, credentialID string) {
	if s.manager == nil || s.fetcher == nil {
		return
	}

	token, err := s.manager.AccessToken(ctx, credentialID, scheduling.UseCached)
	if err != nil {
		log.Warn().Err(err).Str("credential", credentialID).Msg("gemini credential usage verification skipped because access token could not be loaded after refresh")
		return
	}

	projectID, err := s.manager.GetProjectID(ctx, credentialID)
	if err != nil {
		log.Warn().Err(err).Str("credential", credentialID).Msg("gemini credential usage verification skipped because project id could not be loaded after refresh")
		return
	}

	q, err := s.fetcher.FetchQuota(ctx, credentialID, token, projectID)
	if err != nil {
		if statusCode, body, ok := geminiapi.ParseAPIError(err); ok && isCredentialRejectedStatus(statusCode) {
			if err := s.insertLog(ctx, db.InsertLogParams{
				Handler:      string(utils.HandlerGemini),
				CredentialID: credentialID,
				StatusCode:   int32(statusCode),
				Error:        body,
			}); err != nil {
				log.Error().Err(err).Str("credential", credentialID).Msg("gemini scheduler: insert usage verification failure log")
			}
			s.DisableCredential(ctx, credentialID, fmt.Sprintf("usage rejected after refresh (%d)", statusCode))
			log.Warn().
				Str("credential", credentialID).
				Int("status", statusCode).
				Msg("gemini credential disabled because usage fetch still returned auth rejection after refresh")
			return
		}
		log.Warn().Err(err).Str("credential", credentialID).Msg("gemini credential usage verification failed after refresh")
		return
	}

	s.refreshPlanType(ctx, credentialID, token, projectID, "")
	s.UpdateQuota(ctx, credentialID, q)
	log.Info().Str("credential", credentialID).Msg("gemini credential usage verification succeeded after refresh")
}

func (s *Scheduler) syncImportedCredential(ctx context.Context, credentialID string) {
	token, err := s.manager.AccessToken(ctx, credentialID, scheduling.UseCached)
	if err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("gemini sync-credentials: get token")
		return
	}

	projectID, err := s.manager.GetProjectID(ctx, credentialID)
	if err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("gemini sync-credentials: get project")
		return
	}

	q, err := s.fetcher.FetchQuota(ctx, credentialID, token, projectID)
	if err != nil {
		if statusCode, body, ok := geminiapi.ParseAPIError(err); ok && isCredentialRejectedStatus(statusCode) {
			if logErr := s.insertLog(ctx, db.InsertLogParams{
				Handler:      string(utils.HandlerGemini),
				CredentialID: credentialID,
				StatusCode:   int32(statusCode),
				Error:        body,
			}); logErr != nil {
				log.Error().Err(logErr).Str("credential", credentialID).Msg("gemini scheduler: insert import validation failure log")
			}
			s.evictCredential(credentialID)
			s.DisableCredential(ctx, credentialID, fmt.Sprintf("initial quota validation rejected (%d)", statusCode))
			log.Warn().
				Str("credential", credentialID).
				Int("status", statusCode).
				Msg("gemini credential disabled because initial quota validation returned auth rejection")
			return
		}
		log.Error().Err(err).Str("credential", credentialID).Msg("gemini sync-credentials: fetch quota")
		return
	}

	log.Info().
		Str("credential", credentialID).
		Float64("quota_pro", q.QuotaPro).
		Float64("quota_flash", q.QuotaFlash).
		Float64("quota_flashlite", q.QuotaFlashlite).
		Msg("gemini sync-credentials: fetched")
	s.refreshPlanType(ctx, credentialID, token, projectID, "")
	s.UpdateQuota(ctx, credentialID, q)
}

// DisableCredential sets the credential status to disabled and removes it from cache and scheduling.
func (s *Scheduler) DisableCredential(ctx context.Context, id string, reason string) {
	_, err := s.store.UpdateGeminiCLIStatus(ctx, id, string(utils.StatusDisabled), reason)
	switch {
	case err == nil:
	case errors.Is(err, db.ErrNotFound):
		log.Warn().Str("credential", id).Msg("gemini credential already absent while disabling")
	default:
		log.Error().Err(err).Str("credential", id).Msg("gemini disable credential in DB failed")
		return
	}

	s.InvalidateCredential(id)
	s.evictCredential(id)

	event := log.Warn().Str("credential", id)
	if reason != "" {
		event = event.Str("reason", reason)
	}
	event.Msg("gemini credential disabled")
}

func isCredentialRejectedStatus(statusCode int) bool {
	return isRefreshableAuthStatus(statusCode) || isDirectDisableStatus(statusCode)
}

func isRefreshableAuthStatus(statusCode int) bool {
	return statusCode == http.StatusUnauthorized
}

func isDirectDisableStatus(statusCode int) bool {
	return statusCode == http.StatusPaymentRequired
}

func (s *Scheduler) RetryDecision(statusCode int32, text string, headers http.Header) scheduling.RetryDecision {
	if statusCode != http.StatusTooManyRequests {
		return scheduling.RetryDecision{}
	}
	if retryAfter := geminiapi.ParseRetryAfterHeader(headers); retryAfter > 0 {
		return geminiRetryDecision(retryAfter)
	}
	return geminiRetryDecision(geminiapi.ParseRetryDelayText(text))
}

func geminiRetryDecision(delay time.Duration) scheduling.RetryDecision {
	if delay <= 0 {
		return scheduling.RetryDecision{}
	}
	if delay > 5*time.Second {
		return scheduling.RetryDecision{Delay: delay}
	}
	return scheduling.RetryDecision{Delay: delay + 100*time.Millisecond, SameCredential: true}
}

func (s *Scheduler) settingsSnapshot() settings.Snapshot {
	if s == nil || s.settings == nil {
		return settings.DefaultSnapshot()
	}
	return s.settings.Snapshot()
}

func (s *Scheduler) insertLog(ctx context.Context, arg db.InsertLogParams) error {
	if s == nil || s.logStore == nil {
		return nil
	}
	return s.logStore.InsertLog(ctx, arg)
}

func (s *Scheduler) recordResponse(ctx context.Context, credentialID string, statusCode int32, modelTier string, metrics db.LogRequestMetrics) error {
	return s.insertLog(ctx, db.NewInsertLogParams(string(utils.HandlerGemini), credentialID, statusCode, modelTier, metrics))
}

func (s *Scheduler) recordAuthRejection(ctx context.Context, credentialID string, statusCode int32, modelTier string, metrics db.LogRequestMetrics) {
	if err := s.recordResponse(ctx, credentialID, statusCode, modelTier, metrics); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("gemini scheduler: insert auth rejection log")
	}
}

func (s *Scheduler) computeErrorRates(ctx context.Context, rows []availableRow) {
	if s.logStore == nil || len(rows) == 0 {
		return
	}

	windowSeconds := s.settingsSnapshot().QuotaWindowGeminiSeconds()
	proSince := make([]db.ErrorRateSince, 0, len(rows))
	flashSince := make([]db.ErrorRateSince, 0, len(rows))
	flashliteSince := make([]db.ErrorRateSince, 0, len(rows))
	for _, row := range rows {
		if since := ErrorRateSince(row.ResetPro, windowSeconds); !since.IsZero() {
			proSince = append(proSince, db.ErrorRateSince{CredentialID: row.ID, Since: since})
		}
		if since := ErrorRateSince(row.ResetFlash, windowSeconds); !since.IsZero() {
			flashSince = append(flashSince, db.ErrorRateSince{CredentialID: row.ID, Since: since})
		}
		if since := ErrorRateSince(row.ResetFlashLite, windowSeconds); !since.IsZero() {
			flashliteSince = append(flashliteSince, db.ErrorRateSince{CredentialID: row.ID, Since: since})
		}
	}

	ratesPro, err := s.logStore.ErrorRatesForCredentials(ctx, string(utils.HandlerGemini), ModelTierPro, proSince, scheduling.MinErrorRateSamples)
	if err != nil {
		log.Warn().Err(err).Msg("gemini scheduler: compute pro error rates")
		return
	}
	ratesFlash, err := s.logStore.ErrorRatesForCredentials(ctx, string(utils.HandlerGemini), ModelTierFlash, flashSince, scheduling.MinErrorRateSamples)
	if err != nil {
		log.Warn().Err(err).Msg("gemini scheduler: compute flash error rates")
		return
	}
	ratesFlashlite, err := s.logStore.ErrorRatesForCredentials(ctx, string(utils.HandlerGemini), ModelTierFlashLite, flashliteSince, scheduling.MinErrorRateSamples)
	if err != nil {
		log.Warn().Err(err).Msg("gemini scheduler: compute flashlite error rates")
		return
	}

	for i := range rows {
		if rate, ok := ratesPro[rows[i].ID]; ok && rate > 0 {
			rows[i].WeightPro = scheduling.CalcWeight(rate)
		}
		if rate, ok := ratesFlash[rows[i].ID]; ok && rate > 0 {
			rows[i].WeightFlash = scheduling.CalcWeight(rate)
		}
		if rate, ok := ratesFlashlite[rows[i].ID]; ok && rate > 0 {
			rows[i].WeightFlashlite = scheduling.CalcWeight(rate)
		}
	}
}

func (s *Scheduler) refreshCredentialWeight(ctx context.Context, credentialID string, modelTier string) {
	if s == nil || s.logStore == nil || credentialID == "" {
		return
	}

	snap := s.available.Load()
	if snap == nil {
		return
	}

	row, ok := availableRowByCredentialID(snap, credentialID)
	if !ok {
		return
	}

	since, ok := s.errorRateSinceForTier(row, modelTier)
	if !ok {
		return
	}

	weight := 1.0
	if !since.IsZero() {
		rates, err := s.logStore.ErrorRatesForCredentials(ctx, string(utils.HandlerGemini), modelTier, []db.ErrorRateSince{{
			CredentialID: credentialID,
			Since:        since,
		}}, scheduling.MinErrorRateSamples)
		if err != nil {
			log.Warn().Err(err).Str("credential", credentialID).Str("model_tier", modelTier).Msg("gemini scheduler: refresh credential weight")
			return
		}
		if rate, ok := rates[credentialID]; ok && rate > 0 {
			weight = scheduling.CalcWeight(rate)
		}
	}

	s.applyCredentialWeight(credentialID, modelTier, weight)
}

func (s *Scheduler) errorRateSinceForTier(row availableRow, modelTier string) (time.Time, bool) {
	windowSeconds := s.settingsSnapshot().QuotaWindowGeminiSeconds()
	switch modelTier {
	case ModelTierPro:
		return ErrorRateSince(row.ResetPro, windowSeconds), true
	case ModelTierFlash:
		return ErrorRateSince(row.ResetFlash, windowSeconds), true
	case ModelTierFlashLite:
		return ErrorRateSince(row.ResetFlashLite, windowSeconds), true
	default:
		return time.Time{}, false
	}
}

func (s *Scheduler) applyCredentialWeight(credentialID string, modelTier string, weight float64) {
	for {
		snap := s.available.Load()
		if snap == nil {
			return
		}

		updated := make([]availableRow, len(*snap))
		copy(updated, *snap)

		found := false
		for i := range updated {
			if updated[i].ID != credentialID {
				continue
			}
			switch modelTier {
			case ModelTierPro:
				updated[i].WeightPro = weight
			case ModelTierFlash:
				updated[i].WeightFlash = weight
			case ModelTierFlashLite:
				updated[i].WeightFlashlite = weight
			default:
				return
			}
			found = true
			break
		}
		if !found {
			return
		}

		next := updated
		if s.available.CompareAndSwap(snap, &next) {
			return
		}
	}
}

func availableRowByCredentialID(rows *[]availableRow, credentialID string) (availableRow, bool) {
	if rows == nil {
		return availableRow{}, false
	}
	for _, row := range *rows {
		if row.ID == credentialID {
			return row, true
		}
	}
	return availableRow{}, false
}
