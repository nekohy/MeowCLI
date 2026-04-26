package codex

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	codexclient "github.com/nekohy/MeowCLI/api/codex"
	codexAPI "github.com/nekohy/MeowCLI/api/codex/utils"
	"github.com/nekohy/MeowCLI/core/scheduling"
	"github.com/nekohy/MeowCLI/internal/settings"
	db "github.com/nekohy/MeowCLI/internal/store"
	"github.com/nekohy/MeowCLI/utils"
	"golang.org/x/sync/singleflight"

	"github.com/rs/zerolog/log"
)

var (
	ErrNoAvailableCredential = errors.New("no available codex credential")
)

const quotaRefreshFailureBackoff = time.Minute

// SchedulerStore 描述调度器所依赖的 SQL 操作
type SchedulerStore interface {
	ListAvailableCodex(ctx context.Context) ([]db.ListAvailableCodexRow, error)
	UpsertQuota(ctx context.Context, arg db.UpsertQuotaParams) error
	SetQuotaThrottled(ctx context.Context, credentialID string, modelTier string, throttledUntil time.Time) error
}

// QuotaFetcher 由 API 适配器实现，用于从上游服务获取指定凭证的配额
type QuotaFetcher interface {
	FetchQuota(ctx context.Context, credentialID string, accessToken string) (*codexAPI.Quota, error)
}

// throttleState 在内存中跟踪每个凭证的指数退避状态
type throttleState struct {
	consecutive int
	lastFail    time.Time
	until       time.Time
}

// availableSnapshot 缓存 ListAvailableCodex 的查询结果
type availableSnapshot struct {
	rows           []availableRow
	bestByPlanType map[int]int
}

// availableRow 缓存中的单条凭证，Score 由 CalcScore 计算用于排序选择
type availableRow struct {
	ID           string
	PlanTypeCode int
	Quota5h      float64
	Quota7d      float64
	QuotaSpark5h float64
	QuotaSpark7d float64
	Reset5h      time.Time
	Reset7d      time.Time
	ResetSpark5h time.Time
	ResetSpark7d time.Time
	Score        float64
	ScoreSpark   float64
	Weight       float64
	WeightSpark  float64
}

// Scheduler 根据配额比率和重置时间优先级选择最佳可用凭证，
// 并管理失败请求的节流/退避
type Scheduler struct {
	store    SchedulerStore
	logStore db.LogStore
	manager  *Manager
	fetcher  QuotaFetcher
	settings settings.Provider

	mu               sync.Mutex
	throttle         map[string]*throttleState
	checking         map[string]struct{}
	quotaRefreshing  map[string]struct{}
	verifyCredential func(string)
	importSyncMu     sync.Mutex
	available        atomic.Pointer[availableSnapshot]
	planTypes        *planTypeCodec
	quotaRefreshSem  chan struct{}
	refreshGroup     singleflight.Group
}

// NewScheduler 创建一个连接到指定存储和令牌管理器的调度器
func NewScheduler(store SchedulerStore, manager *Manager) *Scheduler {
	var fetcher QuotaFetcher
	if manager != nil {
		fetcher = manager.codexAPI
	}
	return &Scheduler{
		store:           store,
		manager:         manager,
		fetcher:         fetcher,
		throttle:        make(map[string]*throttleState),
		checking:        make(map[string]struct{}),
		quotaRefreshing: make(map[string]struct{}),
		planTypes:       newPlanTypeCodec(),
		quotaRefreshSem: make(chan struct{}, 8),
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

// StartQuotaSyncer 启动后台协程，定期从上游 API 获取配额并写入 quota 表
// 当 ctx 被取消时停止
func (s *Scheduler) StartQuotaSyncer(ctx context.Context) {
	s.quotaSyncer().Start(ctx)
}

func (s *Scheduler) quotaSyncer() scheduling.QuotaSyncer[db.ListAvailableCodexRow] {
	return scheduling.QuotaSyncer[db.ListAvailableCodexRow]{
		SyncInterval: func() time.Duration {
			return s.settingsSnapshot().QuotaSyncInterval()
		},
		List: s.store.ListAvailableCodex,
		CacheRows: func(ctx context.Context, rows []db.ListAvailableCodexRow) {
			s.refreshAvailableFromRows(ctx, rows)
		},
		Sync:     s.syncQuotaRow,
		RowID:    func(row db.ListAvailableCodexRow) string { return row.ID },
		SyncedAt: func(row db.ListAvailableCodexRow) time.Time { return row.SyncedAt },
		ResetAt: func(row db.ListAvailableCodexRow) time.Time {
			return scheduling.EarliestTime(row.Reset5h, row.Reset7d, row.ResetSpark5h, row.ResetSpark7d)
		},
		WithSyncedAt: func(row db.ListAvailableCodexRow, syncedAt time.Time) db.ListAvailableCodexRow {
			row.SyncedAt = syncedAt
			return row
		},
		ReportError: func(err error, message string) {
			log.Error().Err(err).Msg(message)
		},
		WarmErrorMessage:    "codex quota-sync: warm available cache",
		ListErrorMessage:    "codex quota-sync: list credentials",
		RefreshErrorMessage: "codex quota-sync: refresh available cache",
	}
}

func (s *Scheduler) syncQuotaRow(ctx context.Context, row db.ListAvailableCodexRow) {
	if s.fetcher == nil {
		return
	}

	token, err := s.manager.AccessToken(ctx, row.ID, scheduling.UseCached)
	if err != nil {
		log.Error().Err(err).Str("credential", row.ID).Msg("codex quota-sync: get token")
		return
	}

	quotaCtx, cancel := context.WithTimeout(ctx, s.settingsSnapshot().ImportedCheckTimeout())
	q, err := s.fetcher.FetchQuota(quotaCtx, row.ID, token)
	cancel()
	if err != nil {
		if statusCode, body, ok := codexclient.ParseAPIError(err); ok && isCredentialRejectedStatus(statusCode) {
			s.HandleUnauthorized(ctx, row.ID, int32(statusCode), "", db.LogRequestMetrics{Error: body})
			return
		}
		log.Error().Err(err).Str("credential", row.ID).Msg("codex quota-sync: fetch quota")
		return
	}

	log.Info().
		Str("credential", row.ID).
		Float64("quota_5h", q.Quota5h).
		Float64("quota_7d", q.Quota7d).
		Time("reset_5h", q.Reset5h).
		Time("reset_7d", q.Reset7d).
		Msg("codex quota-sync: fetched")

	s.StoreQuota(ctx, row.ID, q)
}

// SelectCredential selects the best available credential for a request.
// The caller can pass a preferred credential ID; if unavailable, no other
// credential is selected.
func (s *Scheduler) SelectCredential(ctx context.Context, selection scheduling.CredentialSelection) (string, error) {
	codec := s.planTypeCodec()
	return s.selectCredential(ctx, s.preferredPlanTypeCodes(selection.Headers), codec.codesFor(selection.AllowedPlanTypes), selection.PreferredCredentialID, selection.ModelTier)
}

func (s *Scheduler) selectCredential(ctx context.Context, preferredCodes []int, allowedCodes []int, preferredCredentialID string, modelTier string) (string, error) {
	snap, err := s.listAvailable(ctx)
	if err != nil {
		return "", err
	}

	allowed := scheduling.PlanTypeCodeSet(allowedCodes)
	if preferredCredentialID != "" {
		if row, ok := availableRowByCredentialID(snap, preferredCredentialID); ok && s.weightedTierScore(row, modelTier) >= 0 && scheduling.PlanTypeAllowed(row.PlanTypeCode, allowed) {
			return preferredCredentialID, nil
		}
		return "", ErrNoAvailableCredential
	}

	for _, code := range preferredCodes {
		if !scheduling.PlanTypeAllowed(code, allowed) {
			continue
		}
		if row, ok := s.selectWeightedCredential(snap.rows, modelTier, func(row availableRow) bool {
			return row.PlanTypeCode == code
		}); ok {
			return row.ID, nil
		}
	}

	if row, ok := s.selectWeightedCredential(snap.rows, modelTier, func(row availableRow) bool {
		return scheduling.PlanTypeAllowed(row.PlanTypeCode, allowed)
	}); ok {
		return row.ID, nil
	}

	return "", ErrNoAvailableCredential
}

func (s *Scheduler) selectWeightedCredential(rows []availableRow, modelTier string, match func(availableRow) bool) (availableRow, bool) {
	return scheduling.PickWeightedFromBest(rows, scheduling.DefaultWeightedBestCount, func(row availableRow) float64 {
		if !match(row) {
			return -1
		}
		return s.weightedTierScore(row, modelTier)
	})
}

func (s *Scheduler) weightedTierScore(row availableRow, modelTier string) float64 {
	if modelTier == ModelTierSpark {
		return scheduling.AdjustedScore(row.ScoreSpark, row.WeightSpark)
	}
	return scheduling.AdjustedScore(row.Score, row.Weight)
}

// listAvailable 返回内存缓存中的可用凭证列表
// 缓存由 RefreshAvailable 初始化，由 UpdateQuota / evictCredential 等事件驱动更新，
// 不再使用 TTL 过期机制
func (s *Scheduler) listAvailable(ctx context.Context) (*availableSnapshot, error) {
	if snap := s.available.Load(); snap != nil {
		if s.hasExpiredThrottleWindow(time.Now()) {
			if _, err := s.RefreshAvailable(ctx); err == nil {
				if refreshed := s.available.Load(); refreshed != nil {
					return refreshed, nil
				}
			} else {
				log.Error().Err(err).Msg("scheduler: refresh available after throttle expiry")
			}
		}
		return snap, nil
	}

	// 首次调用时缓存为空，从 DB 加载
	if _, err := s.RefreshAvailable(ctx); err != nil {
		return nil, err
	}
	if snap := s.available.Load(); snap != nil {
		return snap, nil
	}
	return buildAvailableSnapshot(nil), nil
}

// RefreshAvailable 从 DB 重新加载所有可用凭证并刷新内存缓存
// 在启动时由 StartQuotaSyncer 调用，也可在需要完整重建缓存时手动调用
func (s *Scheduler) RefreshAvailable(ctx context.Context) ([]availableRow, error) {
	value, err, _ := s.refreshGroup.Do("available", func() (any, error) {
		dbRows, err := s.store.ListAvailableCodex(ctx)
		if err != nil {
			return nil, fmt.Errorf("list available codex: %w", err)
		}

		return s.refreshAvailableFromRows(ctx, dbRows), nil
	})
	if err != nil {
		return nil, err
	}
	rows, _ := value.([]availableRow)
	return rows, nil
}

func (s *Scheduler) refreshAvailableFromRows(ctx context.Context, dbRows []db.ListAvailableCodexRow) []availableRow {
	config := s.settingsSnapshot()
	planTypes := s.planTypeCodec()
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
			ID:           r.ID,
			PlanTypeCode: planTypes.code(r.PlanType),
			Quota5h:      r.Quota5h,
			Quota7d:      r.Quota7d,
			QuotaSpark5h: r.QuotaSpark5h,
			QuotaSpark7d: r.QuotaSpark7d,
			Reset5h:      r.Reset5h,
			Reset7d:      r.Reset7d,
			ResetSpark5h: r.ResetSpark5h,
			ResetSpark7d: r.ResetSpark7d,
			Score:        CalcScore(r.Quota5h, r.Quota7d, r.Reset5h, r.Reset7d, config.QuotaWindow5hSeconds(), config.QuotaWindow7dSeconds()),
			ScoreSpark:   CalcScoreSpark(r.QuotaSpark5h, r.QuotaSpark7d, r.ResetSpark5h, r.ResetSpark7d, config.QuotaWindow5hSeconds(), config.QuotaWindow7dSeconds()),
			Weight:       1.0,
			WeightSpark:  1.0,
		}
		if !r.ThrottledUntil.IsZero() && now.Before(r.ThrottledUntil) {
			row.Score = -1
		}
		if !r.ThrottledUntilSpark.IsZero() && now.Before(r.ThrottledUntilSpark) {
			row.ScoreSpark = -1
		}
		rows = append(rows, row)
	}

	s.computeErrorRates(ctx, rows)

	sort.Slice(rows, func(i, j int) bool {
		return scheduling.AdjustedScore(rows[i].Score, rows[i].Weight) > scheduling.AdjustedScore(rows[j].Score, rows[j].Weight)
	})

	s.available.Store(buildAvailableSnapshot(rows))
	s.clearExpiredThrottles(time.Now())
	s.manager.PruneStaleEntries()
	return rows
}

func (s *Scheduler) applyQuotaToAvailable(id string, q *codexAPI.Quota) {
	config := s.settingsSnapshot()
	newScore := CalcScore(q.Quota5h, q.Quota7d, q.Reset5h, q.Reset7d, config.QuotaWindow5hSeconds(), config.QuotaWindow7dSeconds())
	newScoreSpark := CalcScoreSpark(q.QuotaSpark5h, q.QuotaSpark7d, q.ResetSpark5h, q.ResetSpark7d, config.QuotaWindow5hSeconds(), config.QuotaWindow7dSeconds())

	for {
		snap := s.available.Load()
		if snap == nil {
			return
		}

		updated := make([]availableRow, len(snap.rows))
		copy(updated, snap.rows)

		found := false
		for i := range updated {
			if updated[i].ID != id {
				continue
			}
			if updated[i].Score < 0 && newScore >= 0 {
				updated[i].Weight = 1.0
			}
			if updated[i].ScoreSpark < 0 && newScoreSpark >= 0 {
				updated[i].WeightSpark = 1.0
			}
			updated[i].Quota5h = q.Quota5h
			updated[i].Quota7d = q.Quota7d
			updated[i].QuotaSpark5h = q.QuotaSpark5h
			updated[i].QuotaSpark7d = q.QuotaSpark7d
			updated[i].Reset5h = q.Reset5h
			updated[i].Reset7d = q.Reset7d
			updated[i].ResetSpark5h = q.ResetSpark5h
			updated[i].ResetSpark7d = q.ResetSpark7d
			updated[i].Score = newScore
			updated[i].ScoreSpark = newScoreSpark
			found = true
			break
		}
		if !found {
			return
		}

		sort.Slice(updated, func(i, j int) bool {
			return scheduling.AdjustedScore(updated[i].Score, updated[i].Weight) > scheduling.AdjustedScore(updated[j].Score, updated[j].Weight)
		})

		if s.available.CompareAndSwap(snap, buildAvailableSnapshot(updated)) {
			return
		}
	}
}

func (s *Scheduler) mergeQuotaUpdate(credentialID string, q *codexAPI.Quota) codexAPI.Quota {
	merged := *q
	if q.HasDefaultQuota && q.HasSparkQuota {
		return merged
	}
	if !q.HasDefaultQuota && !q.HasSparkQuota {
		return merged
	}

	current, ok := s.availableQuota(credentialID)
	if !ok {
		return merged
	}

	if !q.HasDefaultQuota {
		merged.Quota5h = current.Quota5h
		merged.Quota7d = current.Quota7d
		merged.Reset5h = current.Reset5h
		merged.Reset7d = current.Reset7d
	}
	if !q.HasSparkQuota {
		merged.QuotaSpark5h = current.QuotaSpark5h
		merged.QuotaSpark7d = current.QuotaSpark7d
		merged.ResetSpark5h = current.ResetSpark5h
		merged.ResetSpark7d = current.ResetSpark7d
	}
	merged.HasDefaultQuota = true
	merged.HasSparkQuota = true
	return merged
}

func (s *Scheduler) availableQuota(credentialID string) (codexAPI.Quota, bool) {
	snap := s.available.Load()
	if snap == nil {
		return codexAPI.Quota{}, false
	}
	for _, row := range snap.rows {
		if row.ID != credentialID {
			continue
		}
		return codexAPI.Quota{
			Quota5h:      row.Quota5h,
			Quota7d:      row.Quota7d,
			QuotaSpark5h: row.QuotaSpark5h,
			QuotaSpark7d: row.QuotaSpark7d,
			Reset5h:      row.Reset5h,
			Reset7d:      row.Reset7d,
			ResetSpark5h: row.ResetSpark5h,
			ResetSpark7d: row.ResetSpark7d,
		}, true
	}
	return codexAPI.Quota{}, false
}

func (s *Scheduler) evictCredential(id string) {
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

func (s *Scheduler) suspendCredentialTier(id string, modelTier string) {
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
		case ModelTierSpark:
			updated[i].ScoreSpark = -1
		case ModelTierDefault:
			updated[i].Score = -1
		default:
			return
		}
		break
	}

	s.available.Store(buildAvailableSnapshot(updated))
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

func buildAvailableSnapshot(rows []availableRow) *availableSnapshot {
	bestByPlanType := make(map[int]int, len(rows))
	for i, row := range rows {
		if scheduling.AdjustedScore(row.Score, row.Weight) < 0 {
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

// CalcScore computes quota pressure with a 7d cap on the 5h window.
// Higher scores mean the credential has more quota at risk of expiring soon.
func CalcScore(quota5h, quota7d float64, reset5h, reset7d time.Time, window5hSeconds, window7dSeconds int64) float64 {
	if quota5h == 0 || quota7d == 0 {
		return -1
	}

	return scheduling.MultiWindowQuotaPressureScore(quota5h, quota7d, reset5h, reset7d, window5hSeconds, window7dSeconds)
}

// CalcScoreSpark computes a priority score for the spark model tier.
// Uses the same 5h/7d layered weighting as CalcScore.
// If either spark window quota is exhausted (0%), returns -1 (unusable for spark).
func CalcScoreSpark(quotaSpark5h, quotaSpark7d float64, resetSpark5h, resetSpark7d time.Time, window5hSeconds, window7dSeconds int64) float64 {
	if resetSpark5h.IsZero() && resetSpark7d.IsZero() {
		return -1
	}
	if quotaSpark5h == 0 || quotaSpark7d == 0 {
		return -1
	}
	return scheduling.MultiWindowQuotaPressureScore(quotaSpark5h, quotaSpark7d, resetSpark5h, resetSpark7d, window5hSeconds, window7dSeconds)
}

func ErrorRateSince(reset5h, reset7d time.Time, window5hSeconds, window7dSeconds int64) time.Time {
	return scheduling.LatestWindowStart(
		scheduling.WindowStart(reset5h, window5hSeconds),
		scheduling.WindowStart(reset7d, window7dSeconds),
	)
}

// RecordSuccess 记录成功请求并重置退避状态
func (s *Scheduler) RecordSuccess(_ context.Context, credentialID string, statusCode int32, modelTier string, metrics db.LogRequestMetrics) {
	s.mu.Lock()
	delete(s.throttle, credentialID)
	s.mu.Unlock()

	if err := s.recordResponse(context.Background(), credentialID, statusCode, modelTier, metrics); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("scheduler: insert success log")
	} else {
		s.refreshCredentialWeight(context.Background(), credentialID, modelTier)
	}
}

// RecordFailure records the response for error-rate weighting.
// Throttling is reserved for explicit 429 retry windows or repeated consecutive failures.
func (s *Scheduler) RecordFailure(_ context.Context, credentialID string, statusCode int32, modelTier string, retryAfter time.Duration, metrics db.LogRequestMetrics) {
	// 使用 background context：日志记录和节流是服务端内务操作，
	// 不应受客户端请求 context 取消的影响
	bgCtx := context.Background()

	if err := s.recordResponse(bgCtx, credentialID, statusCode, modelTier, metrics); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("scheduler: insert failure log")
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
	if err := s.store.SetQuotaThrottled(bgCtx, credentialID, throttleTier, throttledUntil); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("scheduler: set throttled")
	}
	s.rememberThrottleUntil(credentialID, throttledUntil)

	if decision.ExplicitRetryAfter && modelTier != "" {
		s.suspendCredentialTier(credentialID, modelTier)
		log.Warn().Str("credential", credentialID).Str("model_tier", modelTier).Dur("backoff", decision.Backoff).Str("reason", decision.Reason).Msg("credential tier throttled")
		return
	}

	// 凭证被节流，从缓存中移除
	s.evictCredential(credentialID)

	log.Warn().Str("credential", credentialID).Dur("backoff", decision.Backoff).Str("reason", decision.Reason).Int("consecutive_failures", consecutive).Msg("credential throttled")
}

// HandleUnauthorized handles auth/account terminal statuses outside the error-rate backoff path.
func (s *Scheduler) HandleUnauthorized(ctx context.Context, credentialID string, statusCode int32, modelTier string, metrics db.LogRequestMetrics) bool {
	if isCredentialDirectDisableStatus(int(statusCode)) {
		s.recordAuthRejection(context.Background(), credentialID, statusCode, modelTier, metrics)
		s.mu.Lock()
		delete(s.throttle, credentialID)
		delete(s.checking, credentialID)
		s.mu.Unlock()
		s.evictCredential(credentialID)
		if s.manager == nil {
			log.Warn().
				Str("credential", credentialID).
				Int32("status", statusCode).
				Msg("credential direct disable skipped because manager is unavailable")
			return true
		}
		s.manager.DisableCredential(context.Background(), credentialID, fmt.Sprintf("credential rejected (%d)", statusCode))
		return true
	}
	if !isCredentialRefreshStatus(int(statusCode)) {
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
			Msg("credential already under refresh verification after auth rejection response")
		return true
	}

	log.Warn().
		Str("credential", credentialID).
		Int32("status", statusCode).
		Msg("credential removed from available pool after auth rejection response")

	if s.verifyCredential != nil {
		go s.verifyCredential(credentialID)
		return true
	}
	if s.manager == nil {
		s.recordAuthRejection(context.Background(), credentialID, statusCode, modelTier, metrics)
		log.Warn().
			Str("credential", credentialID).
			Int32("status", statusCode).
			Msg("credential verification skipped because manager is unavailable")
		return true
	}
	go s.validateCredentialAfterAuthFailure(credentialID, statusCode, modelTier, metrics)
	return true
}

// GetAccessToken 委托给令牌管理器获取访问令牌
func (s *Scheduler) GetAccessToken(ctx context.Context, credentialID string) (string, error) {
	return s.manager.AccessToken(ctx, credentialID, scheduling.UseCached)
}

// UpdateQuota updates the in-memory scheduling cache only.
// Header-derived quota updates can be very frequent, so DB persistence is kept
// on explicit quota-sync paths via StoreQuota.
func (s *Scheduler) UpdateQuota(ctx context.Context, credentialID string, q *codexAPI.Quota) {
	if q == nil {
		return
	}
	if !q.HasDefaultQuota && !q.HasSparkQuota {
		return
	}
	merged := s.mergeQuotaUpdate(credentialID, q)
	q = &merged
	s.applyQuotaToAvailable(credentialID, q)
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
		log.Warn().Str("credential", credentialID).Str("model_tier", modelTier).Msg("scheduler: quota refresh skipped: concurrent limit reached")
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
			log.Warn().Err(err).Str("credential", credentialID).Str("model_tier", modelTier).Msg("scheduler: quota refresh get token")
			return
		}

		q, err := s.fetcher.FetchQuota(refreshCtx, credentialID, token)
		if err != nil {
			s.rememberThrottleUntil(credentialID, time.Now().Add(quotaRefreshFailureBackoff))
			log.Warn().Err(err).Str("credential", credentialID).Str("model_tier", modelTier).Msg("scheduler: quota refresh fetch")
			return
		}
		if q == nil {
			s.rememberThrottleUntil(credentialID, time.Now().Add(quotaRefreshFailureBackoff))
			log.Warn().Str("credential", credentialID).Str("model_tier", modelTier).Msg("scheduler: quota refresh returned empty quota")
			return
		}
		if !quotaRefreshHasTier(q, modelTier) {
			s.rememberThrottleUntil(credentialID, time.Now().Add(quotaRefreshFailureBackoff))
			log.Warn().Str("credential", credentialID).Str("model_tier", modelTier).Msg("scheduler: quota refresh missing requested tier")
			return
		}

		s.UpdateQuota(context.Background(), credentialID, q)
	}()
}

func quotaRefreshHasTier(q *codexAPI.Quota, modelTier string) bool {
	if q == nil {
		return false
	}
	if modelTier == ModelTierSpark {
		return q.HasSparkQuota
	}
	return q.HasDefaultQuota
}

// StoreQuota updates the in-memory scheduling cache and persists the quota.
// Quota Reset fields are upstream absolute timestamps and are not recalculated.
func (s *Scheduler) StoreQuota(ctx context.Context, credentialID string, q *codexAPI.Quota) {
	if q == nil {
		return
	}
	if !q.HasDefaultQuota && !q.HasSparkQuota {
		return
	}
	s.applyQuotaToAvailable(credentialID, q)
	if err := s.store.UpsertQuota(ctx, db.UpsertQuotaParams{
		CredentialID: credentialID,
		Quota5h:      q.Quota5h,
		Quota7d:      q.Quota7d,
		QuotaSpark5h: q.QuotaSpark5h,
		QuotaSpark7d: q.QuotaSpark7d,
		Reset5h:      q.Reset5h,
		Reset7d:      q.Reset7d,
		ResetSpark5h: q.ResetSpark5h,
		ResetSpark7d: q.ResetSpark7d,
	}); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("scheduler: upsert quota")
	}
}

// SyncCredentials 为新导入凭证异步执行首轮 quota 校验
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
			log.Error().Err(err).Msg("sync-credentials: refresh available cache")
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
		log.Info().Str("credential", credentialID).Msg("credential refresh verification succeeded after auth rejection response")
	case errors.Is(err, ErrRefreshTokenMissing):
		if statusCode == http.StatusUnauthorized {
			s.recordAuthRejection(ctx, credentialID, statusCode, modelTier, metrics)
		}
		log.Warn().Str("credential", credentialID).Msg("credential removed after auth rejection response because refresh token is missing")
	default:
		if statusCode == http.StatusUnauthorized {
			s.recordAuthRejection(ctx, credentialID, statusCode, modelTier, metrics)
		}
		log.Warn().Err(err).Str("credential", credentialID).Msg("credential refresh verification finished with error after auth rejection response")
	}
	if err == nil {
		s.verifyCredentialUsable(ctx, credentialID)
	}

	if _, refreshErr := s.RefreshAvailable(context.Background()); refreshErr != nil {
		log.Error().Err(refreshErr).Msg("scheduler: refresh available after unauthorized verification")
	}
}

func (s *Scheduler) verifyCredentialUsable(ctx context.Context, credentialID string) {
	if s.manager == nil || s.fetcher == nil {
		return
	}

	token, err := s.manager.AccessToken(ctx, credentialID, scheduling.UseCached)
	if err != nil {
		log.Warn().Err(err).Str("credential", credentialID).Msg("credential usage verification skipped because access token could not be loaded after refresh")
		return
	}

	q, err := s.fetcher.FetchQuota(ctx, credentialID, token)
	if err != nil {
		if statusCode, body, ok := codexclient.ParseAPIError(err); ok && isCredentialRejectedStatus(statusCode) {
			if err := s.insertLog(ctx, db.InsertLogParams{
				Handler:      string(utils.HandlerCodex),
				CredentialID: credentialID,
				StatusCode:   int32(statusCode),
				Error:        body,
			}); err != nil {
				log.Error().Err(err).Str("credential", credentialID).Msg("scheduler: insert usage verification failure log")
			}
			s.manager.DisableCredential(ctx, credentialID, fmt.Sprintf("usage rejected after refresh (%d)", statusCode))
			log.Warn().
				Str("credential", credentialID).
				Int("status", statusCode).
				Msg("credential disabled because usage fetch still returned auth rejection after refresh")
			return
		}
		log.Warn().Err(err).Str("credential", credentialID).Msg("credential usage verification failed after refresh")
		return
	}

	s.StoreQuota(ctx, credentialID, q)
	log.Info().Str("credential", credentialID).Msg("credential usage verification succeeded after refresh")
}

func (s *Scheduler) syncImportedCredential(ctx context.Context, credentialID string) {
	token, err := s.manager.AccessToken(ctx, credentialID, scheduling.UseCached)
	if err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("sync-credentials: get token")
		return
	}

	q, err := s.fetcher.FetchQuota(ctx, credentialID, token)
	if err != nil {
		if statusCode, body, ok := codexclient.ParseAPIError(err); ok && isCredentialRejectedStatus(statusCode) {
			if logErr := s.insertLog(ctx, db.InsertLogParams{
				Handler:      string(utils.HandlerCodex),
				CredentialID: credentialID,
				StatusCode:   int32(statusCode),
				Error:        body,
			}); logErr != nil {
				log.Error().Err(logErr).Str("credential", credentialID).Msg("scheduler: insert import validation failure log")
			}
			s.evictCredential(credentialID)
			s.manager.DisableCredential(ctx, credentialID, fmt.Sprintf("initial quota validation rejected (%d)", statusCode))
			log.Warn().
				Str("credential", credentialID).
				Int("status", statusCode).
				Msg("credential disabled because initial quota validation returned auth rejection")
			return
		}
		log.Error().Err(err).Str("credential", credentialID).Msg("sync-credentials: fetch quota")
		return
	}

	log.Info().
		Str("credential", credentialID).
		Float64("quota_5h", q.Quota5h).
		Float64("quota_7d", q.Quota7d).
		Msg("sync-credentials: fetched")
	s.StoreQuota(ctx, credentialID, q)
}

func isCredentialRejectedStatus(statusCode int) bool {
	return isCredentialRefreshStatus(statusCode) || isCredentialDirectDisableStatus(statusCode)
}

func isCredentialRefreshStatus(statusCode int) bool {
	return statusCode == http.StatusUnauthorized
}

func isCredentialDirectDisableStatus(statusCode int) bool {
	return statusCode == http.StatusPaymentRequired
}

func (s *Scheduler) RetryDecision(statusCode int32, _ string, headers http.Header) scheduling.RetryDecision {
	if statusCode != http.StatusTooManyRequests {
		return scheduling.RetryDecision{}
	}
	return scheduling.RetryDecision{Delay: utils.ParseRetryAfterHeader(headers)}
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
	return s.insertLog(ctx, db.NewInsertLogParams(string(utils.HandlerCodex), credentialID, statusCode, modelTier, metrics))
}

func (s *Scheduler) recordAuthRejection(ctx context.Context, credentialID string, statusCode int32, modelTier string, metrics db.LogRequestMetrics) {
	if err := s.recordResponse(ctx, credentialID, statusCode, modelTier, metrics); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("scheduler: insert auth rejection log")
	}
}

func (s *Scheduler) computeErrorRates(ctx context.Context, rows []availableRow) {
	if s.logStore == nil || len(rows) == 0 {
		return
	}

	config := s.settingsSnapshot()
	defaultSince := make([]db.ErrorRateSince, 0, len(rows))
	sparkSince := make([]db.ErrorRateSince, 0, len(rows))
	for _, row := range rows {
		if since := ErrorRateSince(row.Reset5h, row.Reset7d, config.QuotaWindow5hSeconds(), config.QuotaWindow7dSeconds()); !since.IsZero() {
			defaultSince = append(defaultSince, db.ErrorRateSince{CredentialID: row.ID, Since: since})
		}
		if since := ErrorRateSince(row.ResetSpark5h, row.ResetSpark7d, config.QuotaWindow5hSeconds(), config.QuotaWindow7dSeconds()); !since.IsZero() {
			sparkSince = append(sparkSince, db.ErrorRateSince{CredentialID: row.ID, Since: since})
		}
	}

	ratesDefault, err := s.logStore.ErrorRatesForCredentials(ctx, string(utils.HandlerCodex), ModelTierDefault, defaultSince, scheduling.MinErrorRateSamples)
	if err != nil {
		log.Warn().Err(err).Msg("scheduler: compute default error rates")
		return
	}
	ratesSpark, err := s.logStore.ErrorRatesForCredentials(ctx, string(utils.HandlerCodex), ModelTierSpark, sparkSince, scheduling.MinErrorRateSamples)
	if err != nil {
		log.Warn().Err(err).Msg("scheduler: compute spark error rates")
		return
	}

	for i := range rows {
		if rate, ok := ratesDefault[rows[i].ID]; ok && rate > 0 {
			rows[i].Weight = scheduling.CalcWeight(rate)
		}
		if rate, ok := ratesSpark[rows[i].ID]; ok && rate > 0 {
			rows[i].WeightSpark = scheduling.CalcWeight(rate)
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
		rates, err := s.logStore.ErrorRatesForCredentials(ctx, string(utils.HandlerCodex), modelTier, []db.ErrorRateSince{{
			CredentialID: credentialID,
			Since:        since,
		}}, scheduling.MinErrorRateSamples)
		if err != nil {
			log.Warn().Err(err).Str("credential", credentialID).Str("model_tier", modelTier).Msg("scheduler: refresh credential weight")
			return
		}
		if rate, ok := rates[credentialID]; ok && rate > 0 {
			weight = scheduling.CalcWeight(rate)
		}
	}

	s.applyCredentialWeight(credentialID, modelTier, weight)
}

func (s *Scheduler) errorRateSinceForTier(row availableRow, modelTier string) (time.Time, bool) {
	config := s.settingsSnapshot()
	switch modelTier {
	case ModelTierSpark:
		return ErrorRateSince(row.ResetSpark5h, row.ResetSpark7d, config.QuotaWindow5hSeconds(), config.QuotaWindow7dSeconds()), true
	case ModelTierDefault:
		return ErrorRateSince(row.Reset5h, row.Reset7d, config.QuotaWindow5hSeconds(), config.QuotaWindow7dSeconds()), true
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

		updated := make([]availableRow, len(snap.rows))
		copy(updated, snap.rows)

		found := false
		for i := range updated {
			if updated[i].ID != credentialID {
				continue
			}
			switch modelTier {
			case ModelTierSpark:
				updated[i].WeightSpark = weight
			case ModelTierDefault:
				updated[i].Weight = weight
			default:
				return
			}
			found = true
			break
		}
		if !found {
			return
		}

		sort.Slice(updated, func(i, j int) bool {
			return scheduling.AdjustedScore(updated[i].Score, updated[i].Weight) > scheduling.AdjustedScore(updated[j].Score, updated[j].Weight)
		})

		if s.available.CompareAndSwap(snap, buildAvailableSnapshot(updated)) {
			return
		}
	}
}
