package opencodego

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

	opencodeapi "github.com/nekohy/MeowCLI/api/opencodego"
	"github.com/nekohy/MeowCLI/core/scheduling"
	"github.com/nekohy/MeowCLI/internal/settings"
	db "github.com/nekohy/MeowCLI/internal/store"
	"github.com/nekohy/MeowCLI/utils"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/singleflight"
)

const (
	ModelTierDefault    = "default"
	monthlyWindow       = 30 * 24 * time.Hour
	quotaRefreshTimeout = 30 * time.Second
)

var ErrNoAvailableCredential = errors.New("no available opencode go credential")

type SchedulerStore interface {
	GetOpenCodeGo(ctx context.Context, id string) (db.OpenCodeGoCredential, error)
	ListAvailableOpenCodeGo(ctx context.Context) ([]db.ListAvailableOpenCodeGoRow, error)
	UpsertOpenCodeGoQuota(ctx context.Context, arg db.UpsertOpenCodeGoQuotaParams) error
	SetOpenCodeGoQuotaThrottled(ctx context.Context, credentialID string, throttledUntil time.Time) error
	UpdateOpenCodeGoStatus(ctx context.Context, id string, status string, reason string) (db.OpenCodeGoCredential, error)
	RestoreExpiredThrottledOpenCodeGo(ctx context.Context) error
	NextOpenCodeGoThrottleDeadline(ctx context.Context) (time.Time, error)
}

type QuotaFetcher interface {
	FetchQuota(ctx context.Context, workspaceID, authCookie string) (*opencodeapi.Quota, error)
}

type throttleState struct {
	consecutive int
	lastFail    time.Time
	until       time.Time
}

type availableRow struct {
	ID             string
	Quota5h        float64
	Quota7d        float64
	Quota1mo       float64
	Reset5h        time.Time
	Reset7d        time.Time
	Reset1mo       time.Time
	ThrottledUntil time.Time
	Score          float64
	Weight         float64
}

type availableSnapshot struct {
	rows []availableRow
}

type Scheduler struct {
	store    SchedulerStore
	manager  *Manager
	fetcher  QuotaFetcher
	settings settings.Provider
	logStore db.LogStore

	mu              sync.Mutex
	throttle        map[string]*throttleState
	quotaRefreshing map[string]struct{}
	available       atomic.Pointer[availableSnapshot]
	refreshGroup    singleflight.Group
}

func NewScheduler(store SchedulerStore, manager *Manager, fetcher QuotaFetcher) *Scheduler {
	return &Scheduler{
		store:           store,
		manager:         manager,
		fetcher:         fetcher,
		throttle:        make(map[string]*throttleState),
		quotaRefreshing: make(map[string]struct{}),
	}
}

func (s *Scheduler) SetSettingsProvider(provider settings.Provider) {
	s.settings = provider
}

func (s *Scheduler) SetLogStore(store db.LogStore) {
	s.logStore = store
}

func (s *Scheduler) StartQuotaSyncer(ctx context.Context) {
	scheduling.ScoreRefreshLoop{
		Interval:        func() time.Duration { return s.settingsSnapshot().ScoreRefreshInterval() },
		DefaultInterval: settings.DefaultSnapshot().ScoreRefreshInterval(),
		Refresh:         s.refreshAvailableScores,
	}.Start(ctx)
	scheduling.StartThrottleDeadlineRefresh(ctx, scheduling.ThrottleDeadlineRefreshConfig{
		Component: "opencode go scheduler",
		Refresh: func(ctx context.Context) error {
			_, err := s.RefreshAvailable(ctx)
			return err
		},
		NextDeadline: s.store.NextOpenCodeGoThrottleDeadline,
		ReportError:  func(err error, message string) { log.Error().Err(err).Msg(message) },
	})
	s.quotaSyncer().Start(ctx)
}

func (s *Scheduler) quotaSyncer() scheduling.QuotaSyncer[db.ListAvailableOpenCodeGoRow] {
	return scheduling.QuotaSyncer[db.ListAvailableOpenCodeGoRow]{
		SyncInterval: func() time.Duration { return s.settingsSnapshot().QuotaSyncInterval() },
		List:         s.store.ListAvailableOpenCodeGo,
		CacheRows: func(ctx context.Context, rows []db.ListAvailableOpenCodeGoRow) {
			s.refreshAvailableFromRows(ctx, rows)
		},
		Sync:     s.syncQuotaRow,
		RowID:    func(row db.ListAvailableOpenCodeGoRow) string { return row.ID },
		SyncedAt: func(row db.ListAvailableOpenCodeGoRow) time.Time { return row.SyncedAt },
		ResetAt: func(row db.ListAvailableOpenCodeGoRow) time.Time {
			resetAt := scheduling.EarliestTime(row.Reset5h, row.Reset7d, row.Reset1mo)
			if !resetAt.IsZero() && !resetAt.Add(scheduling.QuotaResetRefreshGrace).After(time.Now()) {
				return time.Time{}
			}
			return resetAt
		},
		WithSyncedAt: func(row db.ListAvailableOpenCodeGoRow, syncedAt time.Time) db.ListAvailableOpenCodeGoRow {
			row.SyncedAt = syncedAt
			return row
		},
		ReportError:         func(err error, message string) { log.Error().Err(err).Msg(message) },
		WarmErrorMessage:    "opencode go quota-sync: warm available cache",
		ListErrorMessage:    "opencode go quota-sync: list credentials",
		RefreshErrorMessage: "opencode go quota-sync: refresh available cache",
	}
}

func (s *Scheduler) SelectCredential(ctx context.Context, selection scheduling.CredentialSelection) (string, error) {
	snap, err := s.listAvailable(ctx)
	if err != nil {
		return "", err
	}
	if selection.PreferredCredentialID != "" {
		for _, row := range snap.rows {
			if row.ID == selection.PreferredCredentialID && adjustedScore(row) >= 0 {
				return row.ID, nil
			}
		}
		return "", ErrNoAvailableCredential
	}
	row, ok := scheduling.PickWeightedFromBest(snap.rows, s.settingsSnapshot().WeightedBestCount, func(row availableRow) float64 {
		return adjustedScore(row)
	})
	if !ok {
		return "", ErrNoAvailableCredential
	}
	return row.ID, nil
}

func (s *Scheduler) AuthHeaders(ctx context.Context, credentialID string) (http.Header, error) {
	if s.manager == nil {
		return nil, errors.New("opencode go manager is unavailable")
	}
	return s.manager.AuthHeaders(ctx, credentialID, scheduling.UseCached)
}

func (s *Scheduler) RecordSuccess(ctx context.Context, credentialID string, statusCode int32, modelTier string, metrics db.LogRequestMetrics) {
	s.mu.Lock()
	delete(s.throttle, credentialID)
	s.mu.Unlock()
	opCtx, cancel := scheduling.WithDefaultWriteTimeout(ctx)
	defer cancel()
	if err := s.recordResponse(opCtx, credentialID, statusCode, modelTier, metrics); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("opencode go scheduler: insert success log")
	}
}

func (s *Scheduler) RecordFailure(ctx context.Context, credentialID string, statusCode int32, modelTier string, retryAfter time.Duration, metrics db.LogRequestMetrics) {
	opCtx, cancel := scheduling.WithDefaultWriteTimeout(ctx)
	defer cancel()
	if err := s.recordResponse(opCtx, credentialID, statusCode, modelTier, metrics); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("opencode go scheduler: insert failure log")
	}

	now := time.Now()
	s.mu.Lock()
	state := s.throttle[credentialID]
	if state == nil {
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
	throttledUntil := now.Add(decision.Backoff)
	s.mu.Lock()
	state = s.throttle[credentialID]
	if state == nil {
		state = &throttleState{consecutive: consecutive, lastFail: now}
		s.throttle[credentialID] = state
	}
	state.until = throttledUntil
	s.mu.Unlock()
	s.suspendCredential(credentialID, throttledUntil)
	if err := s.store.SetOpenCodeGoQuotaThrottled(opCtx, credentialID, throttledUntil); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("opencode go scheduler: persist throttle")
	}
}

func (s *Scheduler) HandleUnauthorized(ctx context.Context, credentialID string, statusCode int32, modelTier string, metrics db.LogRequestMetrics) bool {
	if statusCode != http.StatusUnauthorized && statusCode != http.StatusForbidden {
		return false
	}
	opCtx, cancel := scheduling.WithDefaultWriteTimeout(ctx)
	defer cancel()
	_ = s.recordResponse(opCtx, credentialID, statusCode, modelTier, metrics)
	_, err := s.store.UpdateOpenCodeGoStatus(opCtx, credentialID, string(utils.StatusDisabled), fmt.Sprintf("upstream rejected API key (HTTP %d)", statusCode))
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		log.Error().Err(err).Str("credential", credentialID).Msg("opencode go scheduler: disable rejected credential")
	}
	s.InvalidateCredential(credentialID)
	s.evictCredential(credentialID)
	return true
}

func (s *Scheduler) QueueQuotaRefresh(ctx context.Context, credentialID string, _ string) {
	if credentialID == "" || s.fetcher == nil {
		return
	}
	s.mu.Lock()
	if _, ok := s.quotaRefreshing[credentialID]; ok {
		s.mu.Unlock()
		return
	}
	s.quotaRefreshing[credentialID] = struct{}{}
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.quotaRefreshing, credentialID)
			s.mu.Unlock()
		}()
		refreshCtx, cancel := context.WithTimeout(context.Background(), quotaRefreshTimeout)
		defer cancel()
		row, err := s.store.GetOpenCodeGo(refreshCtx, credentialID)
		if err != nil || strings.TrimSpace(row.AuthCookie) == "" {
			return
		}
		s.syncCredential(refreshCtx, row)
	}()
}

func (s *Scheduler) RetryDecision(statusCode int32, _ string, headers http.Header) scheduling.RetryDecision {
	if statusCode != http.StatusTooManyRequests && statusCode != http.StatusServiceUnavailable {
		return scheduling.RetryDecision{}
	}
	return scheduling.RetryDecision{Delay: utils.ParseRetryAfterHeader(headers)}
}

func (s *Scheduler) RefreshAvailable(ctx context.Context) ([]availableRow, error) {
	value, err, _ := s.refreshGroup.Do("available", func() (any, error) {
		if err := s.store.RestoreExpiredThrottledOpenCodeGo(ctx); err != nil {
			return nil, fmt.Errorf("restore expired throttled opencode go: %w", err)
		}
		rows, err := s.store.ListAvailableOpenCodeGo(ctx)
		if err != nil {
			return nil, fmt.Errorf("list available opencode go: %w", err)
		}
		return s.refreshAvailableFromRows(ctx, rows), nil
	})
	if err != nil {
		return nil, err
	}
	rows, _ := value.([]availableRow)
	return rows, nil
}

func (s *Scheduler) SyncCredentials(ctx context.Context, ids []string) {
	if len(ids) == 0 || s.fetcher == nil {
		return
	}
	go func() {
		sem := make(chan struct{}, scheduling.DefaultQuotaSyncConcurrency)
		var wg sync.WaitGroup
		for _, id := range ids {
			id := strings.TrimSpace(id)
			if id == "" {
				continue
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					return
				}
				defer func() { <-sem }()
				row, err := s.store.GetOpenCodeGo(ctx, id)
				if err != nil || strings.TrimSpace(row.AuthCookie) == "" {
					return
				}
				s.syncCredential(ctx, row)
			}()
		}
		wg.Wait()
	}()
}

func (s *Scheduler) InvalidateCredential(credentialID string) {
	if s.manager != nil {
		s.manager.InvalidateCredential(credentialID)
	}
}

func (s *Scheduler) syncQuotaRow(ctx context.Context, row db.ListAvailableOpenCodeGoRow) {
	if strings.TrimSpace(row.AuthCookie) == "" {
		return
	}
	credential, err := s.store.GetOpenCodeGo(ctx, row.ID)
	if err != nil {
		log.Error().Err(err).Str("credential", row.ID).Msg("opencode go quota-sync: load credential")
		return
	}
	s.syncCredential(ctx, credential)
}

// RefreshQuota synchronously fetches and persists one credential's latest quota,
// then rebuilds the scheduler's available snapshot before returning.
func (s *Scheduler) RefreshQuota(ctx context.Context, credentialID string) error {
	if s == nil || s.store == nil || s.fetcher == nil {
		return errors.New("opencode go scheduler is unavailable")
	}
	credentialID = strings.TrimSpace(credentialID)
	if credentialID == "" {
		return errors.New("opencode go credential id is required")
	}
	row, err := s.store.GetOpenCodeGo(ctx, credentialID)
	if err != nil {
		return fmt.Errorf("load opencode go credential: %w", err)
	}
	return s.refreshCredentialQuota(ctx, row)
}

func (s *Scheduler) syncCredential(ctx context.Context, row db.OpenCodeGoCredential) {
	if err := s.refreshCredentialQuota(ctx, row); err != nil {
		log.Warn().Err(err).Str("credential", row.ID).Msg("opencode go quota-sync")
	}
}

func (s *Scheduler) refreshCredentialQuota(ctx context.Context, row db.OpenCodeGoCredential) error {
	workspaceID := utils.OpenCodeGoWorkspaceIDFromCredentialID(row.ID)
	if workspaceID == "" {
		return errors.New("opencode go credential id is invalid")
	}
	if strings.TrimSpace(row.AuthCookie) == "" {
		return errors.New("opencode go credential has no auth cookie")
	}
	quotaCtx, cancel := context.WithTimeout(ctx, quotaRefreshTimeout)
	defer cancel()
	quota, err := s.fetcher.FetchQuota(quotaCtx, workspaceID, row.AuthCookie)
	if err != nil {
		return fmt.Errorf("fetch opencode go quota: %w", err)
	}
	if quota == nil {
		return errors.New("opencode go quota response is empty")
	}
	if err := s.store.UpsertOpenCodeGoQuota(ctx, quotaParams(row.ID, quota)); err != nil {
		return fmt.Errorf("persist opencode go quota: %w", err)
	}
	if _, err := s.RefreshAvailable(ctx); err != nil {
		return fmt.Errorf("refresh opencode go scheduler cache: %w", err)
	}
	return nil
}

func quotaParams(credentialID string, quota *opencodeapi.Quota) db.UpsertOpenCodeGoQuotaParams {
	return db.UpsertOpenCodeGoQuotaParams{
		CredentialID: credentialID, Quota5h: quota.Quota5h, Quota7d: quota.Quota7d, Quota1mo: quota.Quota1mo,
		Reset5h: quota.Reset5h, Reset7d: quota.Reset7d, Reset1mo: quota.Reset1mo,
		RewardsCount: len(quota.AvailableRewards),
	}
}

func (s *Scheduler) listAvailable(ctx context.Context) (*availableSnapshot, error) {
	if snap := s.available.Load(); snap != nil {
		return snap, nil
	}
	if _, err := s.RefreshAvailable(ctx); err != nil {
		return nil, err
	}
	if snap := s.available.Load(); snap != nil {
		return snap, nil
	}
	return &availableSnapshot{}, nil
}

func (s *Scheduler) refreshAvailableFromRows(ctx context.Context, source []db.ListAvailableOpenCodeGoRow) []availableRow {
	rows := make([]availableRow, 0, len(source))
	now := time.Now()
	for _, item := range source {
		score := CalcScore(item.Quota5h, item.Quota7d, item.Quota1mo, item.Reset5h, item.Reset7d, item.Reset1mo)
		if item.Reset5h.IsZero() && item.Reset7d.IsZero() && item.Reset1mo.IsZero() {
			score = 1
		}
		row := availableRow{
			ID:      item.ID,
			Quota5h: item.Quota5h, Quota7d: item.Quota7d, Quota1mo: item.Quota1mo,
			Reset5h: item.Reset5h, Reset7d: item.Reset7d, Reset1mo: item.Reset1mo,
			ThrottledUntil: item.ThrottledUntil, Score: score, Weight: 1,
		}
		if item.ThrottledUntil.After(now) {
			row.Score = -1
		}
		rows = append(rows, row)
	}
	s.applyMemoryThrottles(rows, now)
	s.applyErrorRates(ctx, rows)
	sort.Slice(rows, func(i, j int) bool { return adjustedScore(rows[i]) > adjustedScore(rows[j]) })
	s.available.Store(&availableSnapshot{rows: rows})
	return rows
}

func (s *Scheduler) applyErrorRates(ctx context.Context, rows []availableRow) {
	if s.logStore == nil || len(rows) == 0 {
		return
	}
	since := make([]db.ErrorRateSince, 0, len(rows))
	for _, row := range rows {
		start := ErrorRateSince(row.Reset5h, row.Reset7d, row.Reset1mo)
		if !start.IsZero() {
			since = append(since, db.ErrorRateSince{CredentialID: row.ID, Since: start})
		}
	}
	rates, err := s.logStore.ErrorRatesForCredentials(ctx, string(utils.HandlerOpenCodeGo), ModelTierDefault, since, scheduling.MinErrorRateSamples)
	if err != nil {
		log.Warn().Err(err).Msg("opencode go scheduler: compute error rates")
		return
	}
	for i := range rows {
		rows[i].Weight = scheduling.CalcWeight(rates[rows[i].ID])
	}
}

func (s *Scheduler) refreshAvailableScores() {
	snap := s.available.Load()
	if snap == nil {
		return
	}
	rows := append([]availableRow(nil), snap.rows...)
	now := time.Now()
	for i := range rows {
		if rows[i].ThrottledUntil.After(now) {
			rows[i].Score = -1
			continue
		}
		if rows[i].Reset5h.IsZero() && rows[i].Reset7d.IsZero() && rows[i].Reset1mo.IsZero() {
			rows[i].Score = 1
			continue
		}
		rows[i].Score = CalcScore(rows[i].Quota5h, rows[i].Quota7d, rows[i].Quota1mo, rows[i].Reset5h, rows[i].Reset7d, rows[i].Reset1mo)
	}
	sort.Slice(rows, func(i, j int) bool { return adjustedScore(rows[i]) > adjustedScore(rows[j]) })
	s.available.Store(&availableSnapshot{rows: rows})
}

func (s *Scheduler) applyMemoryThrottles(rows []availableRow, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, state := range s.throttle {
		if !state.until.After(now) {
			delete(s.throttle, id)
			continue
		}
		for i := range rows {
			if rows[i].ID == id {
				rows[i].ThrottledUntil = state.until
				rows[i].Score = -1
			}
		}
	}
}

func (s *Scheduler) suspendCredential(id string, until time.Time) {
	snap := s.available.Load()
	if snap == nil {
		return
	}
	rows := append([]availableRow(nil), snap.rows...)
	for i := range rows {
		if rows[i].ID == id {
			rows[i].Score = -1
			rows[i].ThrottledUntil = until
			break
		}
	}
	s.available.Store(&availableSnapshot{rows: rows})
}

func (s *Scheduler) evictCredential(id string) {
	snap := s.available.Load()
	if snap == nil {
		return
	}
	rows := make([]availableRow, 0, len(snap.rows))
	for _, row := range snap.rows {
		if row.ID != id {
			rows = append(rows, row)
		}
	}
	s.available.Store(&availableSnapshot{rows: rows})
}

func CalcScore(quota5h, quota7d, quota1mo float64, reset5h, reset7d, reset1mo time.Time) float64 {
	type window struct {
		quota float64
		reset time.Time
		size  time.Duration
	}
	windows := make([]window, 0, 3)
	if !reset5h.IsZero() {
		windows = append(windows, window{quota: quota5h, reset: reset5h, size: 5 * time.Hour})
	}
	if !reset7d.IsZero() {
		windows = append(windows, window{quota: quota7d, reset: reset7d, size: 7 * 24 * time.Hour})
	}
	if !reset1mo.IsZero() {
		windows = append(windows, window{quota: quota1mo, reset: reset1mo, size: monthlyWindow})
	}
	if len(windows) == 0 {
		return -1
	}
	effective := make([]float64, len(windows))
	cap := scheduling.MaxQuotaPressure
	for i := len(windows) - 1; i >= 0; i-- {
		if windows[i].quota <= 0 {
			return -1
		}
		pressure := scheduling.QuotaPressureScore(windows[i].quota, windows[i].reset, time.Now(), int64(windows[i].size/time.Second))
		if pressure < 0 {
			return -1
		}
		effective[i] = min(pressure, cap)
		cap = effective[i]
	}
	switch len(effective) {
	case 1:
		return effective[0]
	case 2:
		return 0.8*effective[0] + 0.2*effective[1]
	default:
		return 0.7*effective[0] + 0.2*effective[1] + 0.1*effective[2]
	}
}

func ErrorRateSince(reset5h, reset7d, reset1mo time.Time) time.Time {
	return scheduling.LatestWindowStart(
		scheduling.WindowStart(reset5h, int64((5*time.Hour)/time.Second)),
		scheduling.WindowStart(reset7d, int64((7*24*time.Hour)/time.Second)),
		scheduling.WindowStart(reset1mo, int64(monthlyWindow/time.Second)),
	)
}

func adjustedScore(row availableRow) float64 {
	return scheduling.AdjustedScore(row.Score, row.Weight)
}

func (s *Scheduler) recordResponse(ctx context.Context, credentialID string, statusCode int32, modelTier string, metrics db.LogRequestMetrics) error {
	if s.logStore == nil {
		return nil
	}
	if modelTier == "" {
		modelTier = ModelTierDefault
	}
	return s.logStore.InsertLog(ctx, db.NewInsertLogParams(string(utils.HandlerOpenCodeGo), credentialID, statusCode, modelTier, metrics))
}

func (s *Scheduler) settingsSnapshot() settings.Snapshot {
	if s == nil || s.settings == nil {
		return settings.DefaultSnapshot()
	}
	return s.settings.Snapshot()
}
