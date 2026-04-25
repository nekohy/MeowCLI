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
)

var ErrNoAvailableCredential = fmt.Errorf("no available gemini credential")

// SchedulerStore describes the SQL operations the scheduler depends on.
type SchedulerStore interface {
	ListAvailableGeminiCLI(ctx context.Context) ([]db.ListAvailableGeminiCLIRow, error)
	UpsertGeminiQuota(ctx context.Context, arg db.UpsertGeminiQuotaParams) error
	SetGeminiQuotaThrottled(ctx context.Context, credentialID string, modelTier string, throttledUntil time.Time) error
	UpdateGeminiCLIStatus(ctx context.Context, id string, status string, reason string) (db.GeminiCredential, error)
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
	settings settings.Provider

	mu               sync.Mutex
	throttle         map[string]*throttleState
	checking         map[string]struct{}
	verifyCredential func(string)
	importSyncMu     sync.Mutex
	available        atomic.Pointer[[]availableRow]
	planTypes        *planTypeCodec
}

// NewScheduler creates a scheduler connected to the given store and token manager.
func NewScheduler(store SchedulerStore, manager *Manager) *Scheduler {
	return &Scheduler{
		store:     store,
		manager:   manager,
		throttle:  make(map[string]*throttleState),
		checking:  make(map[string]struct{}),
		planTypes: newPlanTypeCodec(),
	}
}

// SetQuotaFetcher sets the quota fetcher used for background quota synchronization.
func (s *Scheduler) SetQuotaFetcher(fetcher geminiapi.QuotaFetcher) {
	if s == nil {
		return
	}
	s.fetcher = fetcher
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
	s.quotaSyncer().Start(ctx)
}

func (s *Scheduler) quotaSyncer() scheduling.QuotaSyncer[db.ListAvailableGeminiCLIRow] {
	return scheduling.QuotaSyncer[db.ListAvailableGeminiCLIRow]{
		Interval: func() time.Duration {
			return s.settingsSnapshot().QuotaSyncInterval()
		},
		List: s.store.ListAvailableGeminiCLI,
		Refresh: func(ctx context.Context, rows []db.ListAvailableGeminiCLIRow) {
			s.refreshAvailableFromRows(ctx, rows)
		},
		Sync:     s.syncQuotaRow,
		RowID:    func(row db.ListAvailableGeminiCLIRow) string { return row.ID },
		SyncedAt: func(row db.ListAvailableGeminiCLIRow) time.Time { return row.SyncedAt },
		WithSyncedAt: func(row db.ListAvailableGeminiCLIRow, syncedAt time.Time) db.ListAvailableGeminiCLIRow {
			row.SyncedAt = syncedAt
			return row
		},
		LogError: func(err error, message string) {
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
	cancel()
	if err != nil {
		if statusCode, body, ok := geminiapi.ParseAPIError(err); ok && isCredentialRejectedStatus(statusCode) {
			s.HandleUnauthorized(ctx, row.ID, int32(statusCode), body, "")
			return
		}
		log.Error().Err(err).Str("credential", row.ID).Msg("gemini quota-sync: fetch quota")
		return
	}

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

// Pick selects the best available credential based on priority scoring.
// The caller can pass a preferred credentialID; if unavailable, no other
// credential is selected.
func (s *Scheduler) Pick(ctx context.Context, headers http.Header, preferredCredentialID string, allowedPlanTypes []string) (string, error) {
	codec := s.planTypeCodec()
	return s.pickPreferred(ctx, s.preferredPlanTypeCodes(headers), codec.codesFor(allowedPlanTypes), preferredCredentialID, "")
}

// PickWithTier selects the best available credential for a specific model tier.
// When modelTier is non-empty, scoring only considers the quota for that tier,
// allowing credentials with exhausted Pro quota to still serve Flash/FlashLite requests.
func (s *Scheduler) PickWithTier(ctx context.Context, headers http.Header, preferredCredentialID string, allowedPlanTypes []string, modelTier string) (string, error) {
	codec := s.planTypeCodec()
	return s.pickPreferred(ctx, s.preferredPlanTypeCodes(headers), codec.codesFor(allowedPlanTypes), preferredCredentialID, modelTier)
}

func (s *Scheduler) pickPreferred(ctx context.Context, preferredCodes []int, allowedCodes []int, preferredCredentialID string, modelTier string) (string, error) {
	rows, err := s.listAvailable(ctx)
	if err != nil {
		return "", err
	}

	allowed := scheduling.PlanTypeCodeSet(allowedCodes)
	scoreForTier := func(row availableRow) float64 {
		switch modelTier {
		case ModelTierPro:
			return scheduling.AdjustedScore(row.ScorePro, row.WeightPro)
		case ModelTierFlash:
			return scheduling.AdjustedScore(row.ScoreFlash, row.WeightFlash)
		case ModelTierFlashLite:
			return scheduling.AdjustedScore(row.ScoreFlashLite, row.WeightFlashlite)
		default:
			return 0
		}
	}
	bestMatch := func(match func(availableRow) bool) (availableRow, bool) {
		var best availableRow
		bestScore := -1.0
		for _, row := range rows {
			if !match(row) {
				continue
			}
			score := scoreForTier(row)
			if score < 0 || score <= bestScore {
				continue
			}
			best = row
			bestScore = score
		}
		return best, bestScore >= 0
	}

	if preferredCredentialID != "" {
		for _, row := range rows {
			if row.ID == preferredCredentialID && scoreForTier(row) >= 0 && scheduling.PlanTypeAllowed(row.PlanTypeCode, allowed) {
				return preferredCredentialID, nil
			}
		}
		return "", ErrNoAvailableCredential
	}

	for _, code := range preferredCodes {
		if !scheduling.PlanTypeAllowed(code, allowed) {
			continue
		}
		if row, ok := bestMatch(func(row availableRow) bool {
			return row.PlanTypeCode == code
		}); ok {
			return row.ID, nil
		}
	}

	if row, ok := bestMatch(func(row availableRow) bool {
		return scheduling.PlanTypeAllowed(row.PlanTypeCode, allowed)
	}); ok {
		return row.ID, nil
	}

	return "", ErrNoAvailableCredential
}

// listAvailable returns the cached available credential list.
// The cache is initialized by RefreshAvailable and updated by
// UpdateQuota / removeFromAvailable events; no TTL expiry is used.
func (s *Scheduler) listAvailable(ctx context.Context) ([]availableRow, error) {
	if rows := s.available.Load(); rows != nil {
		if s.hasExpiredThrottle(time.Now()) {
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
	dbRows, err := s.store.ListAvailableGeminiCLI(ctx)
	if err != nil {
		return nil, fmt.Errorf("list available gemini cli: %w", err)
	}

	return s.refreshAvailableFromRows(ctx, dbRows), nil
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
		if s.isChecking(r.ID) {
			continue
		}
		row := availableRow{
			ID:              r.ID,
			PlanTypeCode:    planTypes.code(r.PlanType),
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
	s.pruneExpiredThrottles(time.Now())
	return rows
}

// updateAvailableQuota updates the scores for a specific credential in the cache.
func (s *Scheduler) updateAvailableQuota(id string, q *geminiapi.Quota) {
	s.mu.Lock()
	defer s.mu.Unlock()

	snap := s.available.Load()
	if snap == nil {
		return
	}

	config := s.settingsSnapshot()
	ws := config.QuotaWindowGeminiSeconds()
	updated := make([]availableRow, len(*snap))
	copy(updated, *snap)

	for i := range updated {
		if updated[i].ID == id {
			newScorePro := CalcScore(q.QuotaPro, q.QuotaFlash, q.QuotaFlashlite, q.ResetPro, q.ResetFlash, q.ResetFlashlite, ModelTierPro, ws)
			newScoreFlash := CalcScore(q.QuotaPro, q.QuotaFlash, q.QuotaFlashlite, q.ResetPro, q.ResetFlash, q.ResetFlashlite, ModelTierFlash, ws)
			newScoreFlashLite := CalcScore(q.QuotaPro, q.QuotaFlash, q.QuotaFlashlite, q.ResetPro, q.ResetFlash, q.ResetFlashlite, ModelTierFlashLite, ws)
			if updated[i].ScorePro < 0 && newScorePro >= 0 {
				updated[i].WeightPro = 1.0
			}
			if updated[i].ScoreFlash < 0 && newScoreFlash >= 0 {
				updated[i].WeightFlash = 1.0
			}
			if updated[i].ScoreFlashLite < 0 && newScoreFlashLite >= 0 {
				updated[i].WeightFlashlite = 1.0
			}
			updated[i].ScorePro = newScorePro
			updated[i].ScoreFlash = newScoreFlash
			updated[i].ScoreFlashLite = newScoreFlashLite
			updated[i].ResetPro = q.ResetPro
			updated[i].ResetFlash = q.ResetFlash
			updated[i].ResetFlashLite = q.ResetFlashlite
			break
		}
	}

	s.available.Store(&updated)
}

// removeFromAvailable removes a credential from the cache (called when throttled).
func (s *Scheduler) removeFromAvailable(id string) {
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

func (s *Scheduler) markTierUnavailable(id string, modelTier string) {
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

func (s *Scheduler) AuthHeaders(ctx context.Context, credentialID string) (http.Header, error) {
	return s.manager.AuthHeaders(ctx, credentialID, scheduling.UseCached)
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
	now := time.Now().Unix()
	switch modelTier {
	case ModelTierPro:
		if quotaPro == 0 {
			return -1
		}
		return scheduling.UrgencyFactor(resetPro.Unix(), now, windowSeconds)*100 + quotaPro
	case ModelTierFlash:
		if quotaFlash == 0 {
			return -1
		}
		return scheduling.UrgencyFactor(resetFlash.Unix(), now, windowSeconds)*100 + quotaFlash
	case ModelTierFlashLite:
		if quotaFlashlite == 0 {
			return -1
		}
		return scheduling.UrgencyFactor(resetFlashlite.Unix(), now, windowSeconds)*100 + quotaFlashlite
	default:
		return -1
	}
}

func ErrorRateSince(resetAt time.Time, windowSeconds int64) time.Time {
	return scheduling.WindowStart(resetAt, windowSeconds)
}

// RecordSuccess records a successful request and resets the backoff state.
func (s *Scheduler) RecordSuccess(_ context.Context, credentialID string, statusCode int32, modelTier string) {
	s.mu.Lock()
	delete(s.throttle, credentialID)
	s.mu.Unlock()

	if err := s.recordResponse(context.Background(), credentialID, statusCode, "", modelTier); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("gemini scheduler: insert success log")
	}
}

// RecordFailure records the response for error-rate weighting.
// Throttling is reserved for explicit 429 retry windows or repeated consecutive failures.
func (s *Scheduler) RecordFailure(_ context.Context, credentialID string, statusCode int32, text string, modelTier string, retryAfter time.Duration) {
	bgCtx := context.Background()

	if err := s.recordResponse(bgCtx, credentialID, statusCode, text, modelTier); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("gemini scheduler: insert failure log")
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
	s.setThrottleUntil(credentialID, throttledUntil)

	if decision.ExplicitRetryAfter && modelTier != "" {
		s.markTierUnavailable(credentialID, modelTier)
		log.Warn().Str("credential", credentialID).Str("model_tier", modelTier).Dur("backoff", decision.Backoff).Str("reason", decision.Reason).Msg("gemini credential tier throttled")
		return
	}

	// Credential is throttled; remove from cache.
	s.removeFromAvailable(credentialID)
	log.Warn().Str("credential", credentialID).Dur("backoff", decision.Backoff).Str("reason", decision.Reason).Int("consecutive_failures", consecutive).Msg("gemini credential throttled")
}

// HandleUnauthorized handles auth/account terminal statuses outside the error-rate backoff path.
func (s *Scheduler) HandleUnauthorized(ctx context.Context, credentialID string, statusCode int32, text string, modelTier string) bool {
	if isDirectDisableStatus(int(statusCode)) {
		s.recordAuthRejection(context.Background(), credentialID, statusCode, text, modelTier)
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

	s.removeFromAvailable(credentialID)
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
		s.recordAuthRejection(context.Background(), credentialID, statusCode, text, modelTier)
		log.Warn().
			Str("credential", credentialID).
			Int32("status", statusCode).
			Msg("gemini credential verification skipped because manager is unavailable")
		return true
	}
	go s.verifyCredentialAfterUnauthorized(credentialID, statusCode, text, modelTier)
	return true
}

// UpdateQuota updates credential quota from an API response (writes to DB + refreshes cache).
func (s *Scheduler) UpdateQuota(ctx context.Context, credentialID string, q *geminiapi.Quota) {
	if q == nil {
		return
	}
	s.updateAvailableQuota(credentialID, q)
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
	}
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
			s.validateImportedCredential(validationCtx, id)
			cancel()
		}

		if _, err := s.RefreshAvailable(context.Background()); err != nil {
			log.Error().Err(err).Msg("gemini sync-credentials: refresh available cache")
		}
	}(ids)
}

func (s *Scheduler) hasExpiredThrottle(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, state := range s.throttle {
		if !state.until.IsZero() && !now.Before(state.until) {
			return true
		}
	}
	return false
}

func (s *Scheduler) pruneExpiredThrottles(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, state := range s.throttle {
		if state.until.IsZero() || now.Before(state.until) {
			continue
		}
		delete(s.throttle, id)
	}
}

func (s *Scheduler) setThrottleUntil(credentialID string, throttledUntil time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.throttle[credentialID]
	if !ok {
		state = &throttleState{}
		s.throttle[credentialID] = state
	}
	state.until = throttledUntil
}

func (s *Scheduler) isChecking(credentialID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.checking[credentialID]
	return ok
}

func (s *Scheduler) finishChecking(credentialID string) {
	s.mu.Lock()
	delete(s.checking, credentialID)
	s.mu.Unlock()
}

func (s *Scheduler) verifyCredentialAfterUnauthorized(credentialID string, statusCode int32, text string, modelTier string) {
	ctx, cancel := context.WithTimeout(context.Background(), s.settingsSnapshot().UnauthorizedCheckTimeout())
	defer cancel()
	defer s.finishChecking(credentialID)

	err := s.manager.RefreshCredential(ctx, credentialID)
	switch {
	case err == nil:
		log.Info().Str("credential", credentialID).Msg("gemini credential refresh verification succeeded after auth rejection response")
	default:
		if statusCode == http.StatusUnauthorized {
			s.recordAuthRejection(ctx, credentialID, statusCode, text, modelTier)
		}
		log.Warn().Err(err).Str("credential", credentialID).Msg("gemini credential refresh verification finished with error after auth rejection response")
	}
	if err == nil {
		s.validateCredentialUsageAfterRefresh(ctx, credentialID)
	}

	if _, refreshErr := s.RefreshAvailable(context.Background()); refreshErr != nil {
		log.Error().Err(refreshErr).Msg("gemini scheduler: refresh available after unauthorized verification")
	}
}

func (s *Scheduler) validateCredentialUsageAfterRefresh(ctx context.Context, credentialID string) {
	if s.manager == nil || s.fetcher == nil {
		return
	}

	token, err := s.manager.AccessToken(ctx, credentialID, scheduling.UseCached)
	if err != nil {
		log.Warn().Err(err).Str("credential", credentialID).Msg("gemini credential usage verification skipped because access token could not be loaded after refresh")
		return
	}

	projectID, _ := s.manager.GetProjectID(ctx, credentialID)

	q, err := s.fetcher.FetchQuota(ctx, credentialID, token, projectID)
	if err != nil {
		if statusCode, body, ok := geminiapi.ParseAPIError(err); ok && isCredentialRejectedStatus(statusCode) {
			if err := s.insertLog(ctx, db.InsertLogParams{
				Handler:      string(utils.HandlerGemini),
				CredentialID: credentialID,
				StatusCode:   int32(statusCode),
				Text:         body,
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

	s.UpdateQuota(ctx, credentialID, q)
	log.Info().Str("credential", credentialID).Msg("gemini credential usage verification succeeded after refresh")
}

func (s *Scheduler) validateImportedCredential(ctx context.Context, credentialID string) {
	token, err := s.manager.AccessToken(ctx, credentialID, scheduling.UseCached)
	if err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("gemini sync-credentials: get token")
		return
	}

	projectID, _ := s.manager.GetProjectID(ctx, credentialID)

	q, err := s.fetcher.FetchQuota(ctx, credentialID, token, projectID)
	if err != nil {
		if statusCode, body, ok := geminiapi.ParseAPIError(err); ok && isCredentialRejectedStatus(statusCode) {
			if logErr := s.insertLog(ctx, db.InsertLogParams{
				Handler:      string(utils.HandlerGemini),
				CredentialID: credentialID,
				StatusCode:   int32(statusCode),
				Text:         body,
			}); logErr != nil {
				log.Error().Err(logErr).Str("credential", credentialID).Msg("gemini scheduler: insert import validation failure log")
			}
			s.removeFromAvailable(credentialID)
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
	s.removeFromAvailable(id)

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

func (s *Scheduler) RetryDelay(statusCode int32, text string, headers http.Header) time.Duration {
	if statusCode != http.StatusTooManyRequests {
		return 0
	}
	if retryAfter := geminiapi.ParseRetryAfterHeader(headers); retryAfter > 0 {
		return retryAfter
	}
	return geminiapi.ParseRetryDelayText(text)
}

func (s *Scheduler) GraceRetry(statusCode int32, _ string, retryAfter time.Duration) (time.Duration, bool) {
	if statusCode != http.StatusTooManyRequests || retryAfter <= 0 {
		return 0, false
	}
	if retryAfter > 2*time.Second {
		return 0, false
	}
	return retryAfter + 100*time.Millisecond, true
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

func (s *Scheduler) recordResponse(ctx context.Context, credentialID string, statusCode int32, text string, modelTier string) error {
	return s.insertLog(ctx, db.InsertLogParams{
		Handler:      string(utils.HandlerGemini),
		CredentialID: credentialID,
		StatusCode:   statusCode,
		Text:         text,
		ModelTier:    modelTier,
	})
}

func (s *Scheduler) recordAuthRejection(ctx context.Context, credentialID string, statusCode int32, text string, modelTier string) {
	if err := s.recordResponse(ctx, credentialID, statusCode, text, modelTier); err != nil {
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
