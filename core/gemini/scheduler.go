package gemini

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
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

// availableSnapshot caches the result of ListAvailableGeminiCLI.
type availableSnapshot struct {
	rows           []availableRow
	bestByPlanType map[int]int
}

// availableRow is a single credential in cache; scores are computed by calcScore for sorted selection.
type availableRow struct {
	ID                 string
	PlanTypeCode       int
	Score              float64 // general score (used when no tier specified)
	ScorePro           float64 // score when serving Pro models
	ScoreFlash         float64 // score when serving Flash models
	ScoreFlashLite     float64 // score when serving FlashLite models
	ErrorRatePro       float64
	WeightPro          float64
	ErrorRateFlash     float64
	WeightFlash        float64
	ErrorRateFlashlite float64
	WeightFlashlite    float64
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
	available        atomic.Pointer[availableSnapshot]
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
	go func() {
		// Perform an initial sync immediately.
		s.syncAllQuotas(ctx)

		for {
			timer := time.NewTimer(s.settingsSnapshot().QuotaSyncInterval())
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				s.syncAllQuotas(ctx)
			}
		}
	}()
}

// syncAllQuotas iterates all enabled credentials, fetches their upstream quota,
// and writes the result to the quota table.
func (s *Scheduler) syncAllQuotas(ctx context.Context) {
	if s.fetcher == nil {
		return
	}

	rows, err := s.store.ListAvailableGeminiCLI(ctx)
	if err != nil {
		log.Error().Err(err).Msg("gemini quota-sync: list credentials")
		return
	}

	for _, row := range rows {
		token, err := s.manager.GetAccessToken(ctx, row.ID)
		if err != nil {
			log.Error().Err(err).Str("credential", row.ID).Msg("gemini quota-sync: get token")
			continue
		}

		quotaCtx, cancel := context.WithTimeout(ctx, s.settingsSnapshot().ImportedCheckTimeout())
		q, err := s.fetcher.FetchQuota(quotaCtx, row.ID, token, row.ProjectID)
		cancel()
		if err != nil {
			if statusCode, body, ok := geminiapi.ParseAPIError(err); ok && isUnauthorizedStatus(statusCode) {
				s.HandleUnauthorized(ctx, row.ID, int32(statusCode), body, "")
				continue
			}
			log.Error().Err(err).Str("credential", row.ID).Msg("gemini quota-sync: fetch quota")
			continue
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

	// After sync, refresh available cache to reflect added/removed credentials.
	if _, err := s.RefreshAvailable(ctx); err != nil {
		log.Error().Err(err).Msg("gemini quota-sync: refresh available cache")
	}
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
	snap, err := s.listAvailable(ctx)
	if err != nil {
		return "", err
	}

	allowed := scheduling.PlanTypeCodeSet(allowedCodes)
	if preferredCredentialID != "" {
		if row, ok := availableRowByCredentialID(snap, preferredCredentialID); ok && s.rowScoreForTier(row, modelTier) >= 0 && scheduling.PlanTypeAllowed(row.PlanTypeCode, allowed) {
			return preferredCredentialID, nil
		}
		return "", ErrNoAvailableCredential
	}

	for _, code := range preferredCodes {
		if !scheduling.PlanTypeAllowed(code, allowed) {
			continue
		}
		for _, row := range snap.rows {
			if row.PlanTypeCode != code {
				continue
			}
			if s.rowScoreForTier(row, modelTier) >= 0 {
				return row.ID, nil
			}
		}
	}

	for _, r := range snap.rows {
		if s.rowScoreForTier(r, modelTier) < 0 {
			continue
		}
		if !scheduling.PlanTypeAllowed(r.PlanTypeCode, allowed) {
			continue
		}
		return r.ID, nil
	}

	return "", ErrNoAvailableCredential
}

// rowScoreForTier returns the score for a row considering the requested model tier.
// When modelTier is empty, uses the row's general score.
// When modelTier is set, uses the tier-specific score.
func (s *Scheduler) rowScoreForTier(row availableRow, modelTier string) float64 {
	switch modelTier {
	case ModelTierPro:
		return scheduling.AdjustedScore(row.ScorePro, row.WeightPro)
	case ModelTierFlash:
		return scheduling.AdjustedScore(row.ScoreFlash, row.WeightFlash)
	case ModelTierFlashLite:
		return scheduling.AdjustedScore(row.ScoreFlashLite, row.WeightFlashlite)
	default:
		return calcAdjustedGeneralScore(row)
	}
}

// listAvailable returns the cached available credential list.
// The cache is initialized by RefreshAvailable and updated by
// UpdateQuota / removeFromAvailable events; no TTL expiry is used.
func (s *Scheduler) listAvailable(ctx context.Context) (*availableSnapshot, error) {
	if snap := s.available.Load(); snap != nil {
		if s.hasExpiredThrottle(time.Now()) {
			if _, err := s.RefreshAvailable(ctx); err == nil {
				if refreshed := s.available.Load(); refreshed != nil {
					return refreshed, nil
				}
			} else {
				log.Error().Err(err).Msg("gemini scheduler: refresh available after throttle expiry")
			}
		}
		return snap, nil
	}

	// Cache is empty on first call; load from DB.
	if _, err := s.RefreshAvailable(ctx); err != nil {
		return nil, err
	}
	if snap := s.available.Load(); snap != nil {
		return snap, nil
	}
	return buildAvailableSnapshot(nil), nil
}

// RefreshAvailable reloads all available credentials from DB and refreshes the in-memory cache.
// Called at startup by StartQuotaSyncer and manually when a full cache rebuild is needed.
func (s *Scheduler) RefreshAvailable(ctx context.Context) ([]availableRow, error) {
	dbRows, err := s.store.ListAvailableGeminiCLI(ctx)
	if err != nil {
		return nil, fmt.Errorf("list available gemini cli: %w", err)
	}

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
			Score:           calcScore(r.QuotaPro, r.QuotaFlash, r.ResetPro, r.ResetFlash, ws),
			ScorePro:        CalcScoreForTier(r.QuotaPro, r.QuotaFlash, r.QuotaFlashlite, r.ResetPro, r.ResetFlash, r.ResetFlashlite, ModelTierPro, ws),
			ScoreFlash:      CalcScoreForTier(r.QuotaPro, r.QuotaFlash, r.QuotaFlashlite, r.ResetPro, r.ResetFlash, r.ResetFlashlite, ModelTierFlash, ws),
			ScoreFlashLite:  CalcScoreForTier(r.QuotaPro, r.QuotaFlash, r.QuotaFlashlite, r.ResetPro, r.ResetFlash, r.ResetFlashlite, ModelTierFlashLite, ws),
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
		row.Score = calcAdjustedGeneralScore(row)
		rows = append(rows, row)
	}

	s.computeErrorRates(ctx, rows)

	sort.Slice(rows, func(i, j int) bool {
		return calcAdjustedGeneralScore(rows[i]) > calcAdjustedGeneralScore(rows[j])
	})

	s.available.Store(buildAvailableSnapshot(rows))
	s.pruneExpiredThrottles(time.Now())
	return rows, nil
}

// updateAvailableQuota updates the scores for a specific credential in the cache and re-sorts.
func (s *Scheduler) updateAvailableQuota(id string, q *geminiapi.Quota) {
	s.mu.Lock()
	defer s.mu.Unlock()

	snap := s.available.Load()
	if snap == nil {
		return
	}

	config := s.settingsSnapshot()
	ws := config.QuotaWindowGeminiSeconds()
	updated := make([]availableRow, len(snap.rows))
	copy(updated, snap.rows)

	for i := range updated {
		if updated[i].ID == id {
			newScorePro := CalcScoreForTier(q.QuotaPro, q.QuotaFlash, q.QuotaFlashlite, q.ResetPro, q.ResetFlash, q.ResetFlashlite, ModelTierPro, ws)
			newScoreFlash := CalcScoreForTier(q.QuotaPro, q.QuotaFlash, q.QuotaFlashlite, q.ResetPro, q.ResetFlash, q.ResetFlashlite, ModelTierFlash, ws)
			newScoreFlashLite := CalcScoreForTier(q.QuotaPro, q.QuotaFlash, q.QuotaFlashlite, q.ResetPro, q.ResetFlash, q.ResetFlashlite, ModelTierFlashLite, ws)
			if updated[i].ScorePro < 0 && newScorePro >= 0 {
				updated[i].ErrorRatePro = 0
				updated[i].WeightPro = 1.0
			}
			if updated[i].ScoreFlash < 0 && newScoreFlash >= 0 {
				updated[i].ErrorRateFlash = 0
				updated[i].WeightFlash = 1.0
			}
			if updated[i].ScoreFlashLite < 0 && newScoreFlashLite >= 0 {
				updated[i].ErrorRateFlashlite = 0
				updated[i].WeightFlashlite = 1.0
			}
			updated[i].Score = calcScore(q.QuotaPro, q.QuotaFlash, q.ResetPro, q.ResetFlash, ws)
			updated[i].ScorePro = newScorePro
			updated[i].ScoreFlash = newScoreFlash
			updated[i].ScoreFlashLite = newScoreFlashLite
			break
		}
	}

	sort.Slice(updated, func(i, j int) bool {
		return calcAdjustedGeneralScore(updated[i]) > calcAdjustedGeneralScore(updated[j])
	})

	s.available.Store(buildAvailableSnapshot(updated))
}

// removeFromAvailable removes a credential from the cache (called when throttled).
func (s *Scheduler) removeFromAvailable(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	snap := s.available.Load()
	if snap == nil {
		return
	}

	filtered := make([]availableRow, 0, len(snap.rows))
	for _, r := range snap.rows {
		if r.ID != id {
			filtered = append(filtered, r)
		}
	}

	s.available.Store(buildAvailableSnapshot(filtered))
}

func (s *Scheduler) markTierUnavailable(id string, modelTier string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	snap := s.available.Load()
	if snap == nil {
		return
	}

	updated := make([]availableRow, len(snap.rows))
	copy(updated, snap.rows)
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

	s.available.Store(buildAvailableSnapshot(updated))
}

func buildAvailableSnapshot(rows []availableRow) *availableSnapshot {
	bestByPlanType := make(map[int]int, len(rows))
	for i, row := range rows {
		if calcAdjustedGeneralScore(row) < 0 {
			continue
		}
		if _, ok := bestByPlanType[row.PlanTypeCode]; ok {
			continue
		}
		bestByPlanType[row.PlanTypeCode] = i
	}
	return &availableSnapshot{
		rows:           rows,
		bestByPlanType: bestByPlanType,
	}
}

func availableRowByCredentialID(snap *availableSnapshot, credentialID string) (availableRow, bool) {
	if snap == nil {
		return availableRow{}, false
	}
	for _, row := range snap.rows {
		if row.ID == credentialID {
			return row, true
		}
	}
	return availableRow{}, false
}

func (s *Scheduler) AuthHeaders(ctx context.Context, credentialID string) (http.Header, error) {
	headers, err := s.manager.GetAuthHeaders(ctx, credentialID)
	if err != nil {
		return nil, err
	}
	return headers, nil
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

// calcScore computes a composite priority score based on quota ratios and reset times.
// Higher scores mean higher priority. Layered weighting:
//
//  1. Pro reset urgency (weight 1000) — prefer credentials about to reset
//  2. Pro remaining quota (weight 100) — among equal urgency, prefer fuller quota
//  3. Flash reset urgency (weight 10) — secondary urgency factor
//  4. Flash remaining quota (weight 1) — tiebreaker
//
// If both Pro and Flash quota are fully exhausted (0%), the credential is unusable (-1).
func calcScore(quotaPro, quotaFlash float64, resetPro, resetFlash time.Time, windowSeconds int64) float64 {
	if quotaPro == 0 && quotaFlash == 0 {
		return -1
	}

	now := time.Now().Unix()
	uPro := scheduling.UrgencyFactor(resetPro.Unix(), now, windowSeconds)
	uFlash := scheduling.UrgencyFactor(resetFlash.Unix(), now, windowSeconds)

	return uPro*1000 + quotaPro*100 + uFlash*10 + quotaFlash
}

// CalcScoreForTier computes a priority score for a specific model tier.
// Only the relevant tier's quota is checked; other tiers are ignored.
// If the requested tier's quota is exhausted (0%), returns -1 (unusable for that tier).
func CalcScoreForTier(quotaPro, quotaFlash, quotaFlashlite float64, resetPro, resetFlash, resetFlashlite time.Time, modelTier string, windowSeconds int64) float64 {
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
		return calcScore(quotaPro, quotaFlash, resetPro, resetFlash, windowSeconds)
	}
}

// RecordSuccess records a successful request and resets the backoff state.
func (s *Scheduler) RecordSuccess(_ context.Context, credentialID string, statusCode int32, modelTier string) {
	s.mu.Lock()
	delete(s.throttle, credentialID)
	s.mu.Unlock()

	if err := s.insertLog(context.Background(), db.InsertLogParams{
		Handler:      string(utils.HandlerGemini),
		CredentialID: credentialID,
		StatusCode:   statusCode,
		Text:         "",
		ModelTier:    modelTier,
	}); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("gemini scheduler: insert success log")
	}
}

// RecordFailure records a failed request and temporarily disables the credential.
// When retryAfter > 0 it uses that duration directly; otherwise exponential backoff:
// base 1 minute * 2^(consecutive failures - 1), capped at 30 minutes.
func (s *Scheduler) RecordFailure(_ context.Context, credentialID string, statusCode int32, text string, retryAfter time.Duration, modelTier string) {
	bgCtx := context.Background()

	if err := s.insertLog(bgCtx, db.InsertLogParams{
		Handler:      string(utils.HandlerGemini),
		CredentialID: credentialID,
		StatusCode:   statusCode,
		Text:         text,
		ModelTier:    modelTier,
	}); err != nil {
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

	if retryAfter <= 0 {
		retryAfter = utils.CalcBackoff(consecutive, s.settingsSnapshot().ThrottleBase(), s.settingsSnapshot().ThrottleMax())
	}

	throttledUntil := now.Add(retryAfter)
	if err := s.store.SetGeminiQuotaThrottled(bgCtx, credentialID, modelTier, throttledUntil); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("gemini scheduler: set throttled")
	}
	s.setThrottleUntil(credentialID, throttledUntil)

	if statusCode == http.StatusTooManyRequests && modelTier != "" {
		s.markTierUnavailable(credentialID, modelTier)
		log.Warn().Str("credential", credentialID).Str("model_tier", modelTier).Dur("backoff", retryAfter).Msg("gemini credential tier throttled")
		return
	}

	// Credential is throttled; remove from cache.
	s.removeFromAvailable(credentialID)
}

// HandleUnauthorized first evicts the 401/403 credential from the available pool,
// then asynchronously verifies it using refresh token + quota check.
func (s *Scheduler) HandleUnauthorized(ctx context.Context, credentialID string, statusCode int32, text string, modelTier string) bool {
	if !isUnauthorizedStatus(int(statusCode)) {
		return false
	}

	if err := s.insertLog(ctx, db.InsertLogParams{
		Handler:      string(utils.HandlerGemini),
		CredentialID: credentialID,
		StatusCode:   statusCode,
		Text:         text,
		ModelTier:    modelTier,
	}); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("gemini scheduler: insert invalidation log")
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
		log.Warn().
			Str("credential", credentialID).
			Int32("status", statusCode).
			Msg("gemini credential verification skipped because manager is unavailable")
		return true
	}
	go s.verifyCredentialAfterUnauthorized(credentialID)
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

func (s *Scheduler) verifyCredentialAfterUnauthorized(credentialID string) {
	ctx, cancel := context.WithTimeout(context.Background(), s.settingsSnapshot().UnauthorizedCheckTimeout())
	defer cancel()
	defer s.finishChecking(credentialID)

	// For Gemini, try to re-obtain the credential (which triggers token refresh if needed).
	_, err := s.manager.ensureCredential(ctx, credentialID)
	switch {
	case err == nil:
		log.Info().Str("credential", credentialID).Msg("gemini credential refresh verification succeeded after auth rejection response")
	default:
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

	token, err := s.manager.GetAccessToken(ctx, credentialID)
	if err != nil {
		log.Warn().Err(err).Str("credential", credentialID).Msg("gemini credential usage verification skipped because access token could not be loaded after refresh")
		return
	}

	projectID, _ := s.manager.GetProjectID(ctx, credentialID)

	q, err := s.fetcher.FetchQuota(ctx, credentialID, token, projectID)
	if err != nil {
		if statusCode, body, ok := geminiapi.ParseAPIError(err); ok && isUnauthorizedStatus(statusCode) {
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
	token, err := s.manager.GetAccessToken(ctx, credentialID)
	if err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("gemini sync-credentials: get token")
		return
	}

	projectID, _ := s.manager.GetProjectID(ctx, credentialID)

	q, err := s.fetcher.FetchQuota(ctx, credentialID, token, projectID)
	if err != nil {
		if statusCode, body, ok := geminiapi.ParseAPIError(err); ok && isUnauthorizedStatus(statusCode) {
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

func isUnauthorizedStatus(statusCode int) bool {
	return statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden
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

func (s *Scheduler) computeErrorRates(ctx context.Context, rows []availableRow) {
	if s.logStore == nil || len(rows) == 0 {
		return
	}

	window := s.settingsSnapshot().ErrorRateWindow()
	ids := make([]string, len(rows))
	for i := range rows {
		ids[i] = rows[i].ID
	}

	ratesPro, err := s.logStore.ErrorRatesForCredentials(ctx, string(utils.HandlerGemini), ModelTierPro, ids, window)
	if err != nil {
		log.Warn().Err(err).Msg("gemini scheduler: compute pro error rates")
		return
	}
	ratesFlash, err := s.logStore.ErrorRatesForCredentials(ctx, string(utils.HandlerGemini), ModelTierFlash, ids, window)
	if err != nil {
		log.Warn().Err(err).Msg("gemini scheduler: compute flash error rates")
		return
	}
	ratesFlashlite, err := s.logStore.ErrorRatesForCredentials(ctx, string(utils.HandlerGemini), ModelTierFlashLite, ids, window)
	if err != nil {
		log.Warn().Err(err).Msg("gemini scheduler: compute flashlite error rates")
		return
	}

	for i := range rows {
		if rate, ok := ratesPro[rows[i].ID]; ok && rate > 0 {
			rows[i].ErrorRatePro = rate
			rows[i].WeightPro = scheduling.CalcWeight(rate)
		}
		if rate, ok := ratesFlash[rows[i].ID]; ok && rate > 0 {
			rows[i].ErrorRateFlash = rate
			rows[i].WeightFlash = scheduling.CalcWeight(rate)
		}
		if rate, ok := ratesFlashlite[rows[i].ID]; ok && rate > 0 {
			rows[i].ErrorRateFlashlite = rate
			rows[i].WeightFlashlite = scheduling.CalcWeight(rate)
		}
	}
}

func calcAdjustedGeneralScore(row availableRow) float64 {
	adjustedPro := scheduling.AdjustedScore(row.ScorePro, row.WeightPro)
	adjustedFlash := scheduling.AdjustedScore(row.ScoreFlash, row.WeightFlash)
	adjustedFlashLite := scheduling.AdjustedScore(row.ScoreFlashLite, row.WeightFlashlite)
	if adjustedPro < 0 && adjustedFlash < 0 && adjustedFlashLite < 0 {
		return -1.0
	}
	total := 0.0
	if adjustedPro >= 0 {
		total += adjustedPro
	}
	if adjustedFlash >= 0 {
		total += adjustedFlash
	}
	if adjustedFlashLite >= 0 {
		total += adjustedFlashLite
	}
	return total
}
