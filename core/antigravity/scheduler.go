package antigravity

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	antigravityapi "github.com/nekohy/MeowCLI/api/antigravity"
	"github.com/nekohy/MeowCLI/core/scheduling"
	"github.com/nekohy/MeowCLI/internal/settings"
	db "github.com/nekohy/MeowCLI/internal/store"
	"github.com/nekohy/MeowCLI/utils"
	"github.com/rs/zerolog/log"
)

var ErrNoAvailableCredential = fmt.Errorf("no available antigravity credential")

const quotaRefreshFailureBackoff = time.Minute

const creditsFallbackBaseScore = 0.001

type availableRow struct {
	ID              string
	PlanType        string
	QuotaClaude     float64
	QuotaPro        float64
	QuotaFlash      float64
	QuotaFlashlite  float64
	QuotaTab        float64
	QuotaImage      float64
	ResetClaude     time.Time
	ResetPro        time.Time
	ResetFlash      time.Time
	ResetFlashlite  time.Time
	ResetTab        time.Time
	ResetImage      time.Time
	ThrottledClaude time.Time
	ThrottledPro    time.Time
	ThrottledFlash  time.Time
	ThrottledLite   time.Time
	ThrottledTab    time.Time
	ThrottledImage  time.Time
	ScoreClaude     float64
	ScorePro        float64
	ScoreFlash      float64
	ScoreFlashlite  float64
	ScoreTab        float64
	ScoreImage      float64
	WeightClaude    float64
	WeightPro       float64
	WeightFlash     float64
	WeightFlashlite float64
	WeightTab       float64
	WeightImage     float64
	CreditsAmount   float64
	CreditTypes     []string
}

type Scheduler struct {
	store    SchedulerStore
	manager  *Manager
	fetcher  antigravityapi.QuotaFetcher
	logStore db.LogStore
	settings settings.Provider

	mu              sync.Mutex
	available       []availableRow
	throttle        map[string]time.Time
	failures        map[string]int
	quotaRefreshing map[string]struct{}
	quotaRefreshSem chan struct{}
	planTypes       *planTypeCodec
}

type SchedulerStore interface {
	ListAvailableAntigravity(ctx context.Context) ([]db.ListAvailableAntigravityRow, error)
	UpsertAntigravityQuota(ctx context.Context, arg db.UpsertAntigravityQuotaParams) error
	SetAntigravityQuotaThrottled(ctx context.Context, credentialID string, modelTier string, throttledUntil time.Time) error
	UpdateAntigravityStatus(ctx context.Context, id string, status string, reason string) (db.AntigravityCredential, error)
	RestoreExpiredThrottledAntigravity(ctx context.Context) error
	NextAntigravityThrottleDeadline(ctx context.Context) (time.Time, error)
}

func NewScheduler(store SchedulerStore, manager *Manager) *Scheduler {
	return &Scheduler{
		store:           store,
		manager:         manager,
		throttle:        make(map[string]time.Time),
		failures:        make(map[string]int),
		quotaRefreshing: make(map[string]struct{}),
		quotaRefreshSem: make(chan struct{}, 8),
		planTypes:       newPlanTypeCodec(),
	}
}

func (s *Scheduler) SetQuotaFetcher(fetcher antigravityapi.QuotaFetcher) {
	s.fetcher = fetcher
}

func (s *Scheduler) SetSettingsProvider(provider settings.Provider) {
	s.settings = provider
}

func (s *Scheduler) SetLogStore(store db.LogStore) {
	s.logStore = store
}

func (s *Scheduler) StartQuotaSyncer(ctx context.Context) {
	s.startScoreRefresh(ctx)
	s.startThrottleDeadlineRefresh(ctx)
	s.quotaSyncer().Start(ctx)
}

func (s *Scheduler) startThrottleDeadlineRefresh(ctx context.Context) {
	scheduling.StartThrottleDeadlineRefresh(ctx, scheduling.ThrottleDeadlineRefreshConfig{
		Component: "antigravity scheduler",
		Refresh: func(ctx context.Context) error {
			_, err := s.RefreshAvailable(ctx)
			return err
		},
		NextDeadline: s.store.NextAntigravityThrottleDeadline,
		ReportError:  func(err error, message string) { log.Error().Err(err).Msg(message) },
	})
}

func (s *Scheduler) startScoreRefresh(ctx context.Context) {
	scheduling.ScoreRefreshLoop{
		Interval:        func() time.Duration { return s.settingsSnapshot().ScoreRefreshInterval() },
		DefaultInterval: settings.DefaultSnapshot().ScoreRefreshInterval(),
		Refresh:         s.refreshAvailableScores,
	}.Start(ctx)
}

func (s *Scheduler) quotaSyncer() scheduling.QuotaSyncer[db.ListAvailableAntigravityRow] {
	return scheduling.QuotaSyncer[db.ListAvailableAntigravityRow]{
		SyncInterval: func() time.Duration {
			return s.settingsSnapshot().QuotaSyncInterval()
		},
		List: s.store.ListAvailableAntigravity,
		CacheRows: func(ctx context.Context, rows []db.ListAvailableAntigravityRow) {
			s.refreshAvailableFromRows(ctx, rows)
		},
		Sync:     s.syncQuotaRow,
		RowID:    func(row db.ListAvailableAntigravityRow) string { return row.ID },
		SyncedAt: func(row db.ListAvailableAntigravityRow) time.Time { return row.SyncedAt },
		ResetAt: func(row db.ListAvailableAntigravityRow) time.Time {
			return scheduling.EarliestTime(row.ResetClaude, row.ResetPro, row.ResetFlash, row.ResetFlashlite, row.ResetTab, row.ResetImage)
		},
		WithSyncedAt: func(row db.ListAvailableAntigravityRow, syncedAt time.Time) db.ListAvailableAntigravityRow {
			row.SyncedAt = syncedAt
			return row
		},
		ReportError: func(err error, message string) {
			log.Error().Err(err).Msg(message)
		},
		WarmErrorMessage:    "antigravity quota-sync: warm available cache",
		ListErrorMessage:    "antigravity quota-sync: list credentials",
		RefreshErrorMessage: "antigravity quota-sync: refresh available cache",
	}
}

func (s *Scheduler) syncQuotaRow(ctx context.Context, row db.ListAvailableAntigravityRow) {
	if s.fetcher == nil {
		return
	}
	token, err := s.manager.AccessToken(ctx, row.ID, scheduling.UseCached)
	if err != nil {
		log.Error().Err(err).Str("credential", row.ID).Msg("antigravity quota-sync: get token")
		return
	}

	quotaCtx, cancel := context.WithTimeout(ctx, s.settingsSnapshot().ImportedCheckTimeout())
	q, err := s.fetcher.FetchQuota(quotaCtx, row.ID, token, row.ProjectID)
	cancel()
	if err != nil {
		if statusCode, body, ok := antigravityapi.ParseAPIError(err); ok && isCredentialRejectedStatus(statusCode) {
			s.HandleUnauthorized(ctx, row.ID, int32(statusCode), "", db.LogRequestMetrics{Error: body})
			return
		}
		log.Error().Err(err).Str("credential", row.ID).Msg("antigravity quota-sync: fetch quota")
		return
	}

	log.Info().
		Str("credential", row.ID).
		Float64("quota_claude", q.QuotaClaude).
		Float64("quota_pro", q.QuotaPro).
		Float64("quota_flash", q.QuotaFlash).
		Float64("quota_flashlite", q.QuotaFlashlite).
		Float64("quota_tab", q.QuotaTab).
		Float64("quota_image", q.QuotaImage).
		Msg("antigravity quota-sync: fetched")

	s.UpdateQuota(ctx, row.ID, q)
}

func (s *Scheduler) RefreshAvailable(ctx context.Context) ([]availableRow, error) {
	if err := s.store.RestoreExpiredThrottledAntigravity(ctx); err != nil {
		log.Error().Err(err).Msg("antigravity scheduler: restore expired throttled credentials")
	}
	rows, err := s.store.ListAvailableAntigravity(ctx)
	if err != nil {
		return nil, fmt.Errorf("list available antigravity: %w", err)
	}
	return s.refreshAvailableFromRows(ctx, rows), nil
}

func (s *Scheduler) refreshAvailableFromRows(_ context.Context, dbRows []db.ListAvailableAntigravityRow) []availableRow {
	ws := s.settingsSnapshot().QuotaWindowCodeAssistSeconds()
	now := time.Now()
	rows := make([]availableRow, 0, len(dbRows))
	for _, r := range dbRows {
		if r.SyncedAt.IsZero() {
			continue
		}
		row := availableRow{
			ID:              r.ID,
			PlanType:        r.PlanType,
			QuotaClaude:     r.QuotaClaude,
			QuotaPro:        r.QuotaPro,
			QuotaFlash:      r.QuotaFlash,
			QuotaFlashlite:  r.QuotaFlashlite,
			QuotaTab:        r.QuotaTab,
			QuotaImage:      r.QuotaImage,
			ResetClaude:     r.ResetClaude,
			ResetPro:        r.ResetPro,
			ResetFlash:      r.ResetFlash,
			ResetFlashlite:  r.ResetFlashlite,
			ResetTab:        r.ResetTab,
			ResetImage:      r.ResetImage,
			ThrottledClaude: r.ThrottledUntilClaude,
			ThrottledPro:    r.ThrottledUntilPro,
			ThrottledFlash:  r.ThrottledUntilFlash,
			ThrottledLite:   r.ThrottledUntilFlashlite,
			ThrottledTab:    r.ThrottledUntilTab,
			ThrottledImage:  r.ThrottledUntilImage,
			ScoreClaude:     CalcScore(r.QuotaClaude, r.ResetClaude, ws),
			ScorePro:        CalcScore(r.QuotaPro, r.ResetPro, ws),
			ScoreFlash:      CalcScore(r.QuotaFlash, r.ResetFlash, ws),
			ScoreFlashlite:  CalcScore(r.QuotaFlashlite, r.ResetFlashlite, ws),
			ScoreTab:        CalcScore(r.QuotaTab, r.ResetTab, ws),
			ScoreImage:      CalcScore(r.QuotaImage, r.ResetImage, ws),
			WeightClaude:    1.0,
			WeightPro:       1.0,
			WeightFlash:     1.0,
			WeightFlashlite: 1.0,
			WeightTab:       1.0,
			WeightImage:     1.0,
			CreditsAmount:   r.CreditsAmount,
			CreditTypes:     parseCreditTypes(r.CreditTypes),
		}
		if !r.ThrottledUntilClaude.IsZero() && now.Before(r.ThrottledUntilClaude) {
			row.ScoreClaude = -1
		}
		if !r.ThrottledUntilPro.IsZero() && now.Before(r.ThrottledUntilPro) {
			row.ScorePro = -1
		}
		if !r.ThrottledUntilFlash.IsZero() && now.Before(r.ThrottledUntilFlash) {
			row.ScoreFlash = -1
		}
		if !r.ThrottledUntilFlashlite.IsZero() && now.Before(r.ThrottledUntilFlashlite) {
			row.ScoreFlashlite = -1
		}
		if !r.ThrottledUntilTab.IsZero() && now.Before(r.ThrottledUntilTab) {
			row.ScoreTab = -1
		}
		if !r.ThrottledUntilImage.IsZero() && now.Before(r.ThrottledUntilImage) {
			row.ScoreImage = -1
		}
		rows = append(rows, row)
	}
	s.mu.Lock()
	s.available = rows
	s.clearExpiredThrottlesLocked(now)
	s.mu.Unlock()
	if s.manager != nil {
		s.manager.PruneStaleEntries()
	}
	return rows
}

func (s *Scheduler) SyncCredentials(_ context.Context, ids []string) {
	if s.fetcher == nil || s.manager == nil || len(ids) == 0 {
		return
	}
	ids = append([]string(nil), ids...)
	go func() {
		for _, id := range ids {
			ctx, cancel := context.WithTimeout(context.Background(), s.settingsSnapshot().ImportedCheckTimeout())
			s.syncImportedCredential(ctx, id)
			cancel()
		}
		if _, err := s.RefreshAvailable(context.Background()); err != nil {
			log.Error().Err(err).Msg("antigravity sync-credentials: refresh available cache")
		}
	}()
}

func (s *Scheduler) SelectCredential(ctx context.Context, selection scheduling.CredentialSelection) (string, error) {
	rows, err := s.listAvailable(ctx)
	if err != nil {
		return "", err
	}
	codec := s.planTypeCodec()
	allowedPlans := scheduling.PlanTypeCodeSet(codec.codesFor(selection.AllowedPlanTypes))
	preferredPlans := s.preferredPlanTypeCodes()
	modelTier := normalizeModelTier(selection.ModelTier)
	now := time.Now()
	if preferred := strings.TrimSpace(selection.PreferredCredentialID); preferred != "" {
		for _, row := range rows {
			if row.ID == preferred && s.credentialUsable(row, modelTier, allowedPlans, now) {
				return row.ID, nil
			}
		}
		if s.creditsFallbackEnabled() {
			for _, row := range rows {
				if row.ID == preferred && s.credentialCreditsFallbackUsable(row, modelTier, allowedPlans, now) {
					return row.ID, nil
				}
			}
		}
		return "", ErrNoAvailableCredential
	}
	for _, planCode := range preferredPlans {
		if !scheduling.PlanTypeAllowed(planCode, allowedPlans) {
			continue
		}
		if row, ok := scheduling.PickWeightedFromBest(rows, scheduling.DefaultWeightedBestCount, func(row availableRow) float64 {
			if codec.code(row.PlanType) != planCode || !s.credentialUsable(row, modelTier, allowedPlans, now) {
				return -1
			}
			return weightedTierScore(row, modelTier)
		}); ok {
			return row.ID, nil
		}
	}
	if row, ok := scheduling.PickWeightedFromBest(rows, scheduling.DefaultWeightedBestCount, func(row availableRow) float64 {
		if !s.credentialUsable(row, modelTier, allowedPlans, now) {
			return -1
		}
		return weightedTierScore(row, modelTier)
	}); ok {
		return row.ID, nil
	}
	if s.creditsFallbackEnabled() {
		for _, planCode := range preferredPlans {
			if !scheduling.PlanTypeAllowed(planCode, allowedPlans) {
				continue
			}
			if row, ok := scheduling.PickWeightedFromBest(rows, scheduling.DefaultWeightedBestCount, func(row availableRow) float64 {
				if codec.code(row.PlanType) != planCode || !s.credentialCreditsFallbackUsable(row, modelTier, allowedPlans, now) {
					return -1
				}
				return creditsFallbackScore(row, modelTier)
			}); ok {
				return row.ID, nil
			}
		}
		if row, ok := scheduling.PickWeightedFromBest(rows, scheduling.DefaultWeightedBestCount, func(row availableRow) float64 {
			if !s.credentialCreditsFallbackUsable(row, modelTier, allowedPlans, now) {
				return -1
			}
			return creditsFallbackScore(row, modelTier)
		}); ok {
			return row.ID, nil
		}
	}
	return "", ErrNoAvailableCredential
}

func (s *Scheduler) planTypeCodec() *planTypeCodec {
	if s.planTypes == nil {
		s.planTypes = newPlanTypeCodec()
	}
	return s.planTypes
}

func (s *Scheduler) preferredPlanTypeCodes() []int {
	return s.planTypeCodec().codesFor(utils.ParseCodeAssistPlanTypeList(s.settingsSnapshot().AntigravityPreferredPlanTypes))
}

func (s *Scheduler) AuthHeaders(ctx context.Context, credentialID string) (http.Header, error) {
	return s.manager.AuthHeaders(ctx, credentialID, scheduling.UseCached)
}

func (s *Scheduler) ProjectID(ctx context.Context, credentialID string) (string, error) {
	return s.manager.ProjectID(ctx, credentialID)
}

func (s *Scheduler) CreditTypes(ctx context.Context, credentialID string) ([]string, error) {
	rows, err := s.listAvailable(ctx)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.ID != credentialID {
			continue
		}
		return append([]string(nil), row.CreditTypes...), nil
	}
	return nil, nil
}

func (s *Scheduler) InvalidateCredential(credentialID string) {
	if s.manager == nil || credentialID == "" {
		return
	}
	s.manager.InvalidateCredential(credentialID)
}

func (s *Scheduler) RecordSuccess(_ context.Context, credentialID string, statusCode int32, modelTier string, metrics db.LogRequestMetrics) {
	s.mu.Lock()
	delete(s.failures, credentialID)
	delete(s.throttle, credentialID)
	s.mu.Unlock()
	if err := s.insertLog(context.Background(), db.NewInsertLogParams(string(utils.HandlerAntigravity), credentialID, statusCode, modelTier, metrics)); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("antigravity scheduler: insert success log")
	}
}

func (s *Scheduler) RecordFailure(_ context.Context, credentialID string, statusCode int32, modelTier string, retryAfter time.Duration, metrics db.LogRequestMetrics) {
	s.mu.Lock()
	s.failures[credentialID]++
	consecutive := s.failures[credentialID]
	s.mu.Unlock()

	if err := s.insertLog(context.Background(), db.NewInsertLogParams(string(utils.HandlerAntigravity), credentialID, statusCode, modelTier, metrics)); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("antigravity scheduler: insert failure log")
	}

	decision := scheduling.DecideFailureThrottle(statusCode, retryAfter, consecutive, s.settingsSnapshot().ThrottleBase(), s.settingsSnapshot().ThrottleMax())
	if !decision.Throttle {
		return
	}
	throttleTier := ""
	if strings.TrimSpace(modelTier) != "" {
		throttleTier = normalizeModelTier(modelTier)
	}
	throttledUntil := time.Now().Add(decision.Backoff)
	if err := s.store.SetAntigravityQuotaThrottled(context.Background(), credentialID, throttleTier, throttledUntil); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("antigravity scheduler: set throttled")
	}
	if _, err := s.store.UpdateAntigravityStatus(context.Background(), credentialID, string(utils.StatusThrottled), utils.TemporaryThrottleReason(decision.Reason)); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("antigravity scheduler: update throttled credential status")
	}
	s.rememberThrottleUntil(credentialID, throttledUntil)
	if throttleTier != "" {
		s.suspendCredentialTier(credentialID, throttleTier)
		log.Warn().Str("credential", credentialID).Str("model_tier", throttleTier).Dur("backoff", decision.Backoff).Str("reason", decision.Reason).Msg("antigravity credential tier throttled")
		return
	}
	s.evictCredential(credentialID)
	log.Warn().Str("credential", credentialID).Dur("backoff", decision.Backoff).Str("reason", decision.Reason).Msg("antigravity credential throttled")
}

func (s *Scheduler) HandleUnauthorized(ctx context.Context, credentialID string, statusCode int32, modelTier string, metrics db.LogRequestMetrics) bool {
	if statusCode == http.StatusPaymentRequired || statusCode == http.StatusForbidden {
		s.disableCredential(context.Background(), credentialID, fmt.Sprintf("credential rejected (%d)", statusCode))
		return true
	}
	if statusCode != http.StatusUnauthorized {
		return false
	}
	if err := s.insertLog(context.Background(), db.NewInsertLogParams(string(utils.HandlerAntigravity), credentialID, statusCode, modelTier, metrics)); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("antigravity scheduler: insert auth rejection log")
	}
	s.InvalidateCredential(credentialID)
	go s.validateCredentialAfterUnauthorized(credentialID, statusCode)
	return true
}

func (s *Scheduler) QueueQuotaRefresh(_ context.Context, credentialID string, modelTier string) {
	if credentialID == "" || s.manager == nil || s.fetcher == nil {
		return
	}
	select {
	case s.quotaRefreshSem <- struct{}{}:
	default:
		log.Warn().Str("credential", credentialID).Str("model_tier", modelTier).Msg("antigravity scheduler: quota refresh skipped: concurrent limit reached")
		return
	}
	tier := normalizeModelTier(modelTier)
	if !s.startQuotaRefresh(credentialID, tier) {
		<-s.quotaRefreshSem
		return
	}
	go func() {
		defer func() { <-s.quotaRefreshSem }()
		defer s.completeQuotaRefresh(credentialID, tier)

		refreshCtx, cancel := context.WithTimeout(context.Background(), s.settingsSnapshot().ImportedCheckTimeout())
		defer cancel()
		token, err := s.manager.AccessToken(refreshCtx, credentialID, scheduling.UseCached)
		if err != nil {
			s.rememberThrottleUntil(credentialID, time.Now().Add(quotaRefreshFailureBackoff))
			log.Warn().Err(err).Str("credential", credentialID).Str("model_tier", tier).Msg("antigravity scheduler: quota refresh get token")
			return
		}
		projectID, err := s.manager.ProjectID(refreshCtx, credentialID)
		if err != nil {
			s.rememberThrottleUntil(credentialID, time.Now().Add(quotaRefreshFailureBackoff))
			log.Warn().Err(err).Str("credential", credentialID).Str("model_tier", tier).Msg("antigravity scheduler: quota refresh get project")
			return
		}
		q, err := s.fetcher.FetchQuota(refreshCtx, credentialID, token, projectID)
		if err != nil {
			s.rememberThrottleUntil(credentialID, time.Now().Add(quotaRefreshFailureBackoff))
			log.Warn().Err(err).Str("credential", credentialID).Str("model_tier", tier).Msg("antigravity scheduler: quota refresh fetch")
			return
		}
		if q == nil {
			s.rememberThrottleUntil(credentialID, time.Now().Add(quotaRefreshFailureBackoff))
			log.Warn().Str("credential", credentialID).Str("model_tier", tier).Msg("antigravity scheduler: quota refresh returned empty quota")
			return
		}
		s.applyQuotaToAvailable(credentialID, q)
	}()
}

func (s *Scheduler) RetryDecision(statusCode int32, text string, headers http.Header) scheduling.RetryDecision {
	if statusCode != http.StatusTooManyRequests && statusCode != http.StatusServiceUnavailable {
		return scheduling.RetryDecision{}
	}
	if retryAfter := utils.ParseRetryAfterHeader(headers); retryAfter > 0 {
		return retryDecision(retryAfter)
	}
	return retryDecision(utils.ParseGoogleRetryDelayText(text))
}

func (s *Scheduler) UpdateQuota(ctx context.Context, credentialID string, q *antigravityapi.Quota) {
	if q == nil {
		return
	}
	cacheUpdated := s.applyQuotaToAvailable(credentialID, q)
	if err := s.store.UpsertAntigravityQuota(ctx, db.UpsertAntigravityQuotaParams{
		CredentialID:   credentialID,
		QuotaClaude:    q.QuotaClaude,
		ResetClaude:    q.ResetClaude,
		QuotaPro:       q.QuotaPro,
		ResetPro:       q.ResetPro,
		QuotaFlash:     q.QuotaFlash,
		ResetFlash:     q.ResetFlash,
		QuotaFlashlite: q.QuotaFlashlite,
		ResetFlashlite: q.ResetFlashlite,
		QuotaTab:       q.QuotaTab,
		ResetTab:       q.ResetTab,
		QuotaImage:     q.QuotaImage,
		ResetImage:     q.ResetImage,
		CreditsAmount:  q.CreditsAmount,
		CreditTypes:    joinCreditTypes(q.CreditTypes),
		CreditsSynced:  q.CreditsSynced,
	}); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("antigravity scheduler: upsert quota")
		return
	}
	if !cacheUpdated {
		if _, err := s.RefreshAvailable(ctx); err != nil {
			log.Error().Err(err).Str("credential", credentialID).Msg("antigravity scheduler: refresh available after quota insert")
		}
	}
}

func (s *Scheduler) listAvailable(ctx context.Context) ([]availableRow, error) {
	s.mu.Lock()
	rows := append([]availableRow(nil), s.available...)
	s.mu.Unlock()
	if len(rows) > 0 {
		if s.hasExpiredThrottleWindow(time.Now()) {
			if refreshed, err := s.RefreshAvailable(ctx); err == nil {
				return refreshed, nil
			} else {
				log.Error().Err(err).Msg("antigravity scheduler: refresh available after throttle expiry")
			}
		}
		return rows, nil
	}
	return s.RefreshAvailable(ctx)
}

func (s *Scheduler) credentialUsable(row availableRow, modelTier string, allowedPlans map[int]struct{}, now time.Time) bool {
	if strings.TrimSpace(row.ID) == "" {
		return false
	}
	if !scheduling.PlanTypeAllowed(s.planTypeCodec().code(row.PlanType), allowedPlans) {
		return false
	}
	if weightedTierScore(row, modelTier) < 0 {
		return false
	}
	s.mu.Lock()
	until := s.throttle[row.ID]
	s.mu.Unlock()
	return until.IsZero() || !now.Before(until)
}

func (s *Scheduler) credentialCreditsFallbackUsable(row availableRow, modelTier string, allowedPlans map[int]struct{}, now time.Time) bool {
	if strings.TrimSpace(row.ID) == "" {
		return false
	}
	if !scheduling.PlanTypeAllowed(s.planTypeCodec().code(row.PlanType), allowedPlans) {
		return false
	}
	if tierQuota(row, modelTier) > 0 {
		return false
	}
	if row.CreditsAmount <= 0 || len(row.CreditTypes) == 0 {
		return false
	}
	s.mu.Lock()
	until := s.throttle[row.ID]
	s.mu.Unlock()
	return until.IsZero() || !now.Before(until)
}

func (s *Scheduler) applyQuotaToAvailable(id string, q *antigravityapi.Quota) bool {
	if q == nil {
		return false
	}
	ws := s.settingsSnapshot().QuotaWindowCodeAssistSeconds()
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.available {
		if s.available[i].ID != id {
			continue
		}
		s.available[i].QuotaClaude = q.QuotaClaude
		s.available[i].QuotaPro = q.QuotaPro
		s.available[i].QuotaFlash = q.QuotaFlash
		s.available[i].QuotaFlashlite = q.QuotaFlashlite
		s.available[i].QuotaTab = q.QuotaTab
		s.available[i].QuotaImage = q.QuotaImage
		s.available[i].ResetClaude = q.ResetClaude
		s.available[i].ResetPro = q.ResetPro
		s.available[i].ResetFlash = q.ResetFlash
		s.available[i].ResetFlashlite = q.ResetFlashlite
		s.available[i].ResetTab = q.ResetTab
		s.available[i].ResetImage = q.ResetImage
		s.available[i].ScoreClaude = CalcScore(q.QuotaClaude, q.ResetClaude, ws)
		s.available[i].ScorePro = CalcScore(q.QuotaPro, q.ResetPro, ws)
		s.available[i].ScoreFlash = CalcScore(q.QuotaFlash, q.ResetFlash, ws)
		s.available[i].ScoreFlashlite = CalcScore(q.QuotaFlashlite, q.ResetFlashlite, ws)
		s.available[i].ScoreTab = CalcScore(q.QuotaTab, q.ResetTab, ws)
		s.available[i].ScoreImage = CalcScore(q.QuotaImage, q.ResetImage, ws)
		if q.CreditsSynced {
			s.available[i].CreditsAmount = q.CreditsAmount
			s.available[i].CreditTypes = append([]string(nil), q.CreditTypes...)
		}
		return true
	}
	return false
}

func (s *Scheduler) refreshAvailableScores() {
	ws := s.settingsSnapshot().QuotaWindowCodeAssistSeconds()
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.available {
		if s.available[i].ScoreClaude >= 0 {
			s.available[i].ScoreClaude = CalcScore(s.available[i].QuotaClaude, s.available[i].ResetClaude, ws)
		}
		if s.available[i].ScorePro >= 0 {
			s.available[i].ScorePro = CalcScore(s.available[i].QuotaPro, s.available[i].ResetPro, ws)
		}
		if s.available[i].ScoreFlash >= 0 {
			s.available[i].ScoreFlash = CalcScore(s.available[i].QuotaFlash, s.available[i].ResetFlash, ws)
		}
		if s.available[i].ScoreFlashlite >= 0 {
			s.available[i].ScoreFlashlite = CalcScore(s.available[i].QuotaFlashlite, s.available[i].ResetFlashlite, ws)
		}
		if s.available[i].ScoreTab >= 0 {
			s.available[i].ScoreTab = CalcScore(s.available[i].QuotaTab, s.available[i].ResetTab, ws)
		}
		if s.available[i].ScoreImage >= 0 {
			s.available[i].ScoreImage = CalcScore(s.available[i].QuotaImage, s.available[i].ResetImage, ws)
		}
	}
}

func (s *Scheduler) evictCredential(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := s.available[:0]
	for _, row := range s.available {
		if row.ID != id {
			filtered = append(filtered, row)
		}
	}
	s.available = filtered
}

func (s *Scheduler) suspendCredentialTier(id string, modelTier string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.available {
		if s.available[i].ID != id {
			continue
		}
		setTierScore(&s.available[i], modelTier, -1)
		return
	}
}

func (s *Scheduler) startQuotaRefresh(id string, modelTier string) bool {
	if id == "" {
		return false
	}
	key := quotaRefreshKey(id, modelTier)
	s.mu.Lock()
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
	if id == "" {
		return
	}
	s.mu.Lock()
	delete(s.quotaRefreshing, quotaRefreshKey(id, modelTier))
	s.mu.Unlock()
}

func quotaRefreshKey(id string, modelTier string) string {
	return id + "\x00" + modelTier
}

func (s *Scheduler) clearExpiredThrottlesLocked(now time.Time) {
	for id, until := range s.throttle {
		if until.IsZero() || now.Before(until) {
			continue
		}
		delete(s.throttle, id)
	}
}

func (s *Scheduler) hasExpiredThrottleWindow(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, until := range s.throttle {
		if !until.IsZero() && !now.Before(until) {
			return true
		}
	}
	return false
}

func (s *Scheduler) rememberThrottleUntil(credentialID string, throttledUntil time.Time) {
	s.mu.Lock()
	s.throttle[credentialID] = throttledUntil
	s.mu.Unlock()

	go s.refreshAvailableAfterThrottle(credentialID, throttledUntil)
}

func (s *Scheduler) refreshAvailableAfterThrottle(credentialID string, throttledUntil time.Time) {
	scheduling.RefreshAfterDeadline(scheduling.RefreshAfterDeadlineConfig{
		Deadline: throttledUntil,
		Superseded: func() bool {
			s.mu.Lock()
			defer s.mu.Unlock()
			until, ok := s.throttle[credentialID]
			return ok && until.After(throttledUntil)
		},
		Refresh: func(ctx context.Context) error {
			_, err := s.RefreshAvailable(ctx)
			return err
		},
		ReportError: func(err error) {
			log.Error().Err(err).Str("credential", credentialID).Msg("antigravity scheduler: refresh available after throttle deadline")
		},
	})
}

func (s *Scheduler) validateCredentialAfterUnauthorized(credentialID string, statusCode int32) {
	ctx, cancel := context.WithTimeout(context.Background(), s.settingsSnapshot().UnauthorizedCheckTimeout())
	defer cancel()
	if err := s.manager.RefreshCredential(ctx, credentialID); err != nil {
		s.disableCredential(context.Background(), credentialID, fmt.Sprintf("refresh verification failed after auth rejection (%d)", statusCode))
		log.Warn().Err(err).Str("credential", credentialID).Msg("antigravity credential refresh verification failed")
		return
	}
	if _, err := s.RefreshAvailable(context.Background()); err != nil {
		log.Error().Err(err).Msg("antigravity scheduler: refresh available after unauthorized verification")
	}
}

func (s *Scheduler) disableCredential(ctx context.Context, id string, reason string) {
	if _, err := s.store.UpdateAntigravityStatus(ctx, id, string(utils.StatusDisabled), reason); err != nil {
		log.Error().Err(err).Str("credential", id).Msg("antigravity disable credential in DB failed")
		return
	}
	s.InvalidateCredential(id)
	s.evictCredential(id)
}

func (s *Scheduler) syncImportedCredential(ctx context.Context, credentialID string) {
	token, err := s.manager.AccessToken(ctx, credentialID, scheduling.UseCached)
	if err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("antigravity sync-credentials: get token")
		return
	}
	projectID, err := s.manager.ProjectID(ctx, credentialID)
	if err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("antigravity sync-credentials: get project")
		return
	}
	q, err := s.fetcher.FetchQuota(ctx, credentialID, token, projectID)
	if err != nil {
		if statusCode, body, ok := antigravityapi.ParseAPIError(err); ok && isCredentialRejectedStatus(statusCode) {
			if logErr := s.insertLog(ctx, db.InsertLogParams{
				Handler:      string(utils.HandlerAntigravity),
				CredentialID: credentialID,
				StatusCode:   int32(statusCode),
				Error:        body,
			}); logErr != nil {
				log.Error().Err(logErr).Str("credential", credentialID).Msg("antigravity scheduler: insert import validation failure log")
			}
			s.disableCredential(ctx, credentialID, fmt.Sprintf("initial quota validation rejected (%d)", statusCode))
			return
		}
		log.Error().Err(err).Str("credential", credentialID).Msg("antigravity sync-credentials: fetch quota")
		return
	}
	s.UpdateQuota(ctx, credentialID, q)
}

func CalcScore(quota float64, reset time.Time, windowSeconds int64) float64 {
	if quota == 0 {
		return -1
	}
	return scheduling.QuotaPressureScore(quota, reset, time.Now(), windowSeconds)
}

func ErrorRateSince(resetAt time.Time, windowSeconds int64) time.Time {
	return scheduling.WindowStart(resetAt, windowSeconds)
}

func weightedTierScore(row availableRow, modelTier string) float64 {
	switch normalizeModelTier(modelTier) {
	case ModelTierClaude:
		return scheduling.AdjustedScore(row.ScoreClaude, row.WeightClaude)
	case ModelTierPro:
		return scheduling.AdjustedScore(row.ScorePro, row.WeightPro)
	case ModelTierFlash:
		return scheduling.AdjustedScore(row.ScoreFlash, row.WeightFlash)
	case ModelTierFlashLite:
		return scheduling.AdjustedScore(row.ScoreFlashlite, row.WeightFlashlite)
	case ModelTierTab:
		return scheduling.AdjustedScore(row.ScoreTab, row.WeightTab)
	case ModelTierImage:
		return scheduling.AdjustedScore(row.ScoreImage, row.WeightImage)
	default:
		return -1
	}
}

func creditsFallbackScore(row availableRow, modelTier string) float64 {
	if row.CreditsAmount <= 0 {
		return -1
	}
	weight := tierWeight(row, modelTier)
	if weight < creditsFallbackBaseScore {
		weight = creditsFallbackBaseScore
	}
	return row.CreditsAmount * weight
}

func tierQuota(row availableRow, modelTier string) float64 {
	switch normalizeModelTier(modelTier) {
	case ModelTierClaude:
		return row.QuotaClaude
	case ModelTierPro:
		return row.QuotaPro
	case ModelTierFlash:
		return row.QuotaFlash
	case ModelTierFlashLite:
		return row.QuotaFlashlite
	case ModelTierTab:
		return row.QuotaTab
	case ModelTierImage:
		return row.QuotaImage
	default:
		return 0
	}
}

func tierWeight(row availableRow, modelTier string) float64 {
	switch normalizeModelTier(modelTier) {
	case ModelTierClaude:
		return row.WeightClaude
	case ModelTierPro:
		return row.WeightPro
	case ModelTierFlash:
		return row.WeightFlash
	case ModelTierFlashLite:
		return row.WeightFlashlite
	case ModelTierTab:
		return row.WeightTab
	case ModelTierImage:
		return row.WeightImage
	default:
		return 0
	}
}

func tierThrottledUntil(row availableRow, modelTier string) time.Time {
	switch normalizeModelTier(modelTier) {
	case ModelTierClaude:
		return row.ThrottledClaude
	case ModelTierPro:
		return row.ThrottledPro
	case ModelTierFlash:
		return row.ThrottledFlash
	case ModelTierFlashLite:
		return row.ThrottledLite
	case ModelTierTab:
		return row.ThrottledTab
	case ModelTierImage:
		return row.ThrottledImage
	default:
		return time.Time{}
	}
}

func setTierScore(row *availableRow, modelTier string, score float64) {
	switch normalizeModelTier(modelTier) {
	case ModelTierClaude:
		row.ScoreClaude = score
	case ModelTierPro:
		row.ScorePro = score
	case ModelTierFlash:
		row.ScoreFlash = score
	case ModelTierFlashLite:
		row.ScoreFlashlite = score
	case ModelTierTab:
		row.ScoreTab = score
	case ModelTierImage:
		row.ScoreImage = score
	}
}

func normalizeModelTier(modelTier string) string {
	switch strings.ToLower(strings.TrimSpace(modelTier)) {
	case ModelTierClaude, ModelTierPro, ModelTierFlash, ModelTierFlashLite, ModelTierTab, ModelTierImage:
		return strings.ToLower(strings.TrimSpace(modelTier))
	default:
		return ModelTierClaude
	}
}

func retryDecision(delay time.Duration) scheduling.RetryDecision {
	if delay <= 0 {
		return scheduling.RetryDecision{}
	}
	if delay > 5*time.Second {
		return scheduling.RetryDecision{Delay: delay}
	}
	return scheduling.RetryDecision{Delay: delay + 100*time.Millisecond, SameCredential: true}
}

const (
	planTypeCodeFree = iota
	planTypeCodePro
	planTypeCodeUltra
)

const planTypeCodeUnknown = 999

var planTypeCodes = map[string]int{
	utils.CodeAssistPlanTypeFree:    planTypeCodeFree,
	utils.CodeAssistPlanTypePro:     planTypeCodePro,
	utils.CodeAssistPlanTypeUltra:   planTypeCodeUltra,
	utils.CodeAssistPlanTypeUnknown: planTypeCodeUnknown,
}

type planTypeCodec struct{}

func newPlanTypeCodec() *planTypeCodec { return &planTypeCodec{} }

func (c *planTypeCodec) code(planType string) int {
	if code, ok := planTypeCodes[utils.NormalizeCodeAssistPlanType(planType)]; ok {
		return code
	}
	return planTypeCodeUnknown
}

func (c *planTypeCodec) codesFor(planTypes []string) []int {
	if len(planTypes) == 0 {
		return nil
	}

	codes := make([]int, 0, len(planTypes))
	seen := make(map[int]struct{}, len(planTypes))
	for _, planType := range planTypes {
		code := c.code(planType)
		if code == planTypeCodeUnknown {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
	}
	return codes
}

func isCredentialRejectedStatus(statusCode int) bool {
	return statusCode == http.StatusUnauthorized || statusCode == http.StatusPaymentRequired || statusCode == http.StatusForbidden
}

func joinCreditTypes(values []string) string {
	return strings.Join(normalizedCreditTypes(values), ",")
}

func parseCreditTypes(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return normalizedCreditTypes(strings.Split(value, ","))
}

func normalizedCreditTypes(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}

func (s *Scheduler) insertLog(ctx context.Context, arg db.InsertLogParams) error {
	if s.logStore == nil {
		return nil
	}
	return s.logStore.InsertLog(ctx, arg)
}

func (s *Scheduler) settingsSnapshot() settings.Snapshot {
	if s.settings == nil {
		return settings.DefaultSnapshot()
	}
	return s.settings.Snapshot()
}

func (s *Scheduler) creditsFallbackEnabled() bool {
	return s.settingsSnapshot().AntigravityUseCredits
}
