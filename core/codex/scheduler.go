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
	UpdateCodexPlanType(ctx context.Context, id string, planType string) (db.Codex, error)
	UpdateCodexStatus(ctx context.Context, id string, status string, reason string) (db.Codex, error)
	RestoreExpiredThrottledCodex(ctx context.Context) error
	NextCodexThrottleDeadline(ctx context.Context) (time.Time, error)
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
	ID                  string
	PlanTypeCode        int
	Quota5h             float64
	Quota7d             float64
	Quota1mo            float64
	QuotaSpark5h        float64
	QuotaSpark7d        float64
	QuotaSpark1mo       float64
	Reset5h             time.Time
	Reset7d             time.Time
	Reset1mo            time.Time
	ResetSpark5h        time.Time
	ResetSpark7d        time.Time
	ResetSpark1mo       time.Time
	ThrottledUntil      time.Time
	ThrottledUntilSpark time.Time
	Score               float64
	ScoreSpark          float64
	Weight              float64
	WeightSpark         float64
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
	s.settings = provider
}

func (s *Scheduler) SetLogStore(store db.LogStore) {
	s.logStore = store
}

// StartQuotaSyncer 启动后台协程，定期从上游 API 获取配额并写入 quota 表
// 当 ctx 被取消时停止
func (s *Scheduler) StartQuotaSyncer(ctx context.Context) {
	s.startScoreRefresh(ctx)
	s.startThrottleDeadlineRefresh(ctx)
	s.quotaSyncer().Start(ctx)
}

func (s *Scheduler) startThrottleDeadlineRefresh(ctx context.Context) {
	scheduling.StartThrottleDeadlineRefresh(ctx, scheduling.ThrottleDeadlineRefreshConfig{
		Component: "scheduler",
		Refresh: func(ctx context.Context) error {
			_, err := s.RefreshAvailable(ctx)
			return err
		},
		NextDeadline: s.store.NextCodexThrottleDeadline,
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
			return scheduling.EarliestTime(row.Reset5h, row.Reset7d, row.Reset1mo, row.ResetSpark5h, row.ResetSpark7d, row.ResetSpark1mo)
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
		Float64("quota_1mo", q.Quota1mo).
		Time("reset_5h", q.Reset5h).
		Time("reset_7d", q.Reset7d).
		Time("reset_1mo", q.Reset1mo).
		Msg("codex quota-sync: fetched")

	s.StoreQuota(ctx, row.ID, q)
}

// SelectCredential selects the best available credential for a request.
// The caller can pass a preferred credential ID; if unavailable, no other
// credential is selected.
func (s *Scheduler) SelectCredential(ctx context.Context, selection scheduling.CredentialSelection) (string, error) {
	codec := s.planTypeCodec()
	return s.selectCredential(ctx, s.preferredPlanTypeCodes(), codec.codesFor(selection.AllowedPlanTypes), selection.PreferredCredentialID, selection.ModelTier)
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
	return scheduling.PickWeightedFromBest(rows, s.settingsSnapshot().WeightedBestCount, func(row availableRow) float64 {
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
		if err := s.store.RestoreExpiredThrottledCodex(ctx); err != nil {
			return nil, fmt.Errorf("restore expired throttled codex: %w", err)
		}
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
			ID:                  r.ID,
			PlanTypeCode:        planTypes.code(r.PlanType),
			Quota5h:             r.Quota5h,
			Quota7d:             r.Quota7d,
			Quota1mo:            r.Quota1mo,
			QuotaSpark5h:        r.QuotaSpark5h,
			QuotaSpark7d:        r.QuotaSpark7d,
			QuotaSpark1mo:       r.QuotaSpark1mo,
			Reset5h:             r.Reset5h,
			Reset7d:             r.Reset7d,
			Reset1mo:            r.Reset1mo,
			ResetSpark5h:        r.ResetSpark5h,
			ResetSpark7d:        r.ResetSpark7d,
			ResetSpark1mo:       r.ResetSpark1mo,
			ThrottledUntil:      r.ThrottledUntil,
			ThrottledUntilSpark: r.ThrottledUntilSpark,
			Score:               CalcScore(r.Quota5h, r.Quota7d, r.Quota1mo, r.Reset5h, r.Reset7d, r.Reset1mo, config.QuotaWindow5hSeconds(), config.QuotaWindow7dSeconds()),
			ScoreSpark:          CalcScoreSpark(r.QuotaSpark5h, r.QuotaSpark7d, r.QuotaSpark1mo, r.ResetSpark5h, r.ResetSpark7d, r.ResetSpark1mo, config.QuotaWindow5hSeconds(), config.QuotaWindow7dSeconds()),
			Weight:              1.0,
			WeightSpark:         1.0,
		}
		if !r.ThrottledUntil.IsZero() && now.Before(r.ThrottledUntil) {
			row.Score = -1
		}
		if !r.ThrottledUntilSpark.IsZero() && now.Before(r.ThrottledUntilSpark) {
			row.ScoreSpark = -1
		}
		rows = append(rows, row)
	}
	s.applyMemoryThrottles(rows, now)

	s.computeErrorRates(ctx, rows)

	sort.Slice(rows, func(i, j int) bool {
		return scheduling.AdjustedScore(rows[i].Score, rows[i].Weight) > scheduling.AdjustedScore(rows[j].Score, rows[j].Weight)
	})

	s.available.Store(buildAvailableSnapshot(rows))
	s.clearExpiredThrottles(time.Now())
	s.manager.PruneStaleEntries()
	return rows
}

func (s *Scheduler) applyQuotaToAvailable(id string, q *codexAPI.Quota) bool {
	config := s.settingsSnapshot()
	baseScore := CalcScore(q.Quota5h, q.Quota7d, q.Quota1mo, q.Reset5h, q.Reset7d, q.Reset1mo, config.QuotaWindow5hSeconds(), config.QuotaWindow7dSeconds())
	baseScoreSpark := CalcScoreSpark(q.QuotaSpark5h, q.QuotaSpark7d, q.QuotaSpark1mo, q.ResetSpark5h, q.ResetSpark7d, q.ResetSpark1mo, config.QuotaWindow5hSeconds(), config.QuotaWindow7dSeconds())
	now := time.Now()

	for {
		snap := s.available.Load()
		if snap == nil {
			return false
		}

		updated := make([]availableRow, len(snap.rows))
		copy(updated, snap.rows)

		found := false
		for i := range updated {
			if updated[i].ID != id {
				continue
			}
			newScore := baseScore
			newScoreSpark := baseScoreSpark
			if now.Before(updated[i].ThrottledUntil) {
				newScore = -1
			}
			if now.Before(updated[i].ThrottledUntilSpark) {
				newScoreSpark = -1
			}
			if updated[i].Score < 0 && newScore >= 0 {
				updated[i].Weight = 1.0
			}
			if updated[i].ScoreSpark < 0 && newScoreSpark >= 0 {
				updated[i].WeightSpark = 1.0
			}
			updated[i].Quota5h = q.Quota5h
			updated[i].Quota7d = q.Quota7d
			updated[i].Quota1mo = q.Quota1mo
			updated[i].QuotaSpark5h = q.QuotaSpark5h
			updated[i].QuotaSpark7d = q.QuotaSpark7d
			updated[i].QuotaSpark1mo = q.QuotaSpark1mo
			updated[i].Reset5h = q.Reset5h
			updated[i].Reset7d = q.Reset7d
			updated[i].Reset1mo = q.Reset1mo
			updated[i].ResetSpark5h = q.ResetSpark5h
			updated[i].ResetSpark7d = q.ResetSpark7d
			updated[i].ResetSpark1mo = q.ResetSpark1mo
			updated[i].Score = newScore
			updated[i].ScoreSpark = newScoreSpark
			found = true
			break
		}
		if !found {
			return false
		}

		sort.Slice(updated, func(i, j int) bool {
			return scheduling.AdjustedScore(updated[i].Score, updated[i].Weight) > scheduling.AdjustedScore(updated[j].Score, updated[j].Weight)
		})

		if s.available.CompareAndSwap(snap, buildAvailableSnapshot(updated)) {
			return true
		}
	}
}

func (s *Scheduler) refreshAvailableScores() {
	config := s.settingsSnapshot()
	scheduling.RefreshRows(
		s.available.Load,
		s.available.CompareAndSwap,
		func(snap *availableSnapshot) []availableRow {
			rows := make([]availableRow, len(snap.rows))
			copy(rows, snap.rows)
			return rows
		},
		func(rows []availableRow) *availableSnapshot {
			sort.Slice(rows, func(i, j int) bool {
				return scheduling.AdjustedScore(rows[i].Score, rows[i].Weight) > scheduling.AdjustedScore(rows[j].Score, rows[j].Weight)
			})
			return buildAvailableSnapshot(rows)
		},
		func(row *availableRow) {
			if row.Score >= 0 {
				row.Score = CalcScore(row.Quota5h, row.Quota7d, row.Quota1mo, row.Reset5h, row.Reset7d, row.Reset1mo, config.QuotaWindow5hSeconds(), config.QuotaWindow7dSeconds())
			}
			if row.ScoreSpark >= 0 {
				row.ScoreSpark = CalcScoreSpark(row.QuotaSpark5h, row.QuotaSpark7d, row.QuotaSpark1mo, row.ResetSpark5h, row.ResetSpark7d, row.ResetSpark1mo, config.QuotaWindow5hSeconds(), config.QuotaWindow7dSeconds())
			}
		},
	)
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
		merged.Quota1mo = current.Quota1mo
		merged.Reset5h = current.Reset5h
		merged.Reset7d = current.Reset7d
		merged.Reset1mo = current.Reset1mo
	}
	if !q.HasSparkQuota {
		merged.QuotaSpark5h = current.QuotaSpark5h
		merged.QuotaSpark7d = current.QuotaSpark7d
		merged.QuotaSpark1mo = current.QuotaSpark1mo
		merged.ResetSpark5h = current.ResetSpark5h
		merged.ResetSpark7d = current.ResetSpark7d
		merged.ResetSpark1mo = current.ResetSpark1mo
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
			Quota5h:       row.Quota5h,
			Quota7d:       row.Quota7d,
			Quota1mo:      row.Quota1mo,
			QuotaSpark5h:  row.QuotaSpark5h,
			QuotaSpark7d:  row.QuotaSpark7d,
			QuotaSpark1mo: row.QuotaSpark1mo,
			Reset5h:       row.Reset5h,
			Reset7d:       row.Reset7d,
			Reset1mo:      row.Reset1mo,
			ResetSpark5h:  row.ResetSpark5h,
			ResetSpark7d:  row.ResetSpark7d,
			ResetSpark1mo: row.ResetSpark1mo,
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

func (s *Scheduler) throttleCredentialTier(id string, modelTier string, throttledUntil time.Time) {
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
			updated[i].ThrottledUntilSpark = throttledUntil
		case ModelTierDefault:
			updated[i].Score = -1
			updated[i].ThrottledUntil = throttledUntil
		default:
			return
		}
		break
	}

	s.available.Store(buildAvailableSnapshot(updated))
}

func (s *Scheduler) applyMemoryThrottles(rows []availableRow, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 合并后台 quota 探测失败产生的短退避，避免完整刷新从 DB 重建时提前恢复
	for i := range rows {
		if until := activeThrottleUntil(s.throttle, rows[i].ID, ModelTierDefault, now); !until.IsZero() {
			if until.After(rows[i].ThrottledUntil) {
				rows[i].ThrottledUntil = until
			}
			rows[i].Score = -1
		}
		if until := activeThrottleUntil(s.throttle, rows[i].ID, ModelTierSpark, now); !until.IsZero() {
			if until.After(rows[i].ThrottledUntilSpark) {
				rows[i].ThrottledUntilSpark = until
			}
			rows[i].ScoreSpark = -1
		}
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

	s.suspendCredentialTier(id, modelTier)
	return true
}

func (s *Scheduler) completeQuotaRefresh(id string, modelTier string) {
	if id == "" {
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
	if s.manager == nil || credentialID == "" {
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

// CalcScore computes quota pressure across the quota windows returned by upstream.
// Higher scores mean the credential has more quota at risk of expiring soon.
func CalcScore(quota5h, quota7d, quota1mo float64, reset5h, reset7d, reset1mo time.Time, window5hSeconds, window7dSeconds int64) float64 {
	return calcCodexScore(quota5h, quota7d, quota1mo, reset5h, reset7d, reset1mo, window5hSeconds, window7dSeconds)
}

// CalcScoreSpark computes a priority score for the spark model tier.
// Uses the same layered weighting as CalcScore.
func CalcScoreSpark(quotaSpark5h, quotaSpark7d, quotaSpark1mo float64, resetSpark5h, resetSpark7d, resetSpark1mo time.Time, window5hSeconds, window7dSeconds int64) float64 {
	return calcCodexScore(quotaSpark5h, quotaSpark7d, quotaSpark1mo, resetSpark5h, resetSpark7d, resetSpark1mo, window5hSeconds, window7dSeconds)
}

type codexQuotaWindow struct {
	quota         float64
	reset         time.Time
	windowSeconds int64
}

func calcCodexScore(quota5h, quota7d, quota1mo float64, reset5h, reset7d, reset1mo time.Time, window5hSeconds, window7dSeconds int64) float64 {
	windows := make([]codexQuotaWindow, 0, 3)
	if !reset5h.IsZero() {
		windows = append(windows, codexQuotaWindow{quota: quota5h, reset: reset5h, windowSeconds: window5hSeconds})
	}
	if !reset7d.IsZero() {
		windows = append(windows, codexQuotaWindow{quota: quota7d, reset: reset7d, windowSeconds: window7dSeconds})
	}
	if !reset1mo.IsZero() {
		windows = append(windows, codexQuotaWindow{quota: quota1mo, reset: reset1mo, windowSeconds: codexAPI.MonthlyWindowSeconds})
	}
	if len(windows) == 0 {
		return -1
	}
	for _, window := range windows {
		if window.quota <= 0 {
			return -1
		}
	}
	return layeredQuotaPressureScore(windows)
}

func layeredQuotaPressureScore(windows []codexQuotaWindow) float64 {
	if len(windows) == 0 {
		return -1
	}
	effective := make([]float64, len(windows))
	longerCap := scheduling.MaxQuotaPressure
	for i := len(windows) - 1; i >= 0; i-- {
		pressure := scheduling.QuotaPressureScore(windows[i].quota, windows[i].reset, time.Now(), windows[i].windowSeconds)
		if pressure < 0 {
			return -1
		}
		effective[i] = min(pressure, longerCap)
		longerCap = effective[i]
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

func ErrorRateSince(reset5h, reset7d, reset1mo time.Time, window5hSeconds, window7dSeconds int64) time.Time {
	return scheduling.LatestWindowStart(
		scheduling.WindowStart(reset5h, window5hSeconds),
		scheduling.WindowStart(reset7d, window7dSeconds),
		scheduling.WindowStart(reset1mo, codexAPI.MonthlyWindowSeconds),
	)
}

// RecordSuccess 记录成功请求并重置退避状态
func (s *Scheduler) RecordSuccess(ctx context.Context, credentialID string, statusCode int32, modelTier string, metrics db.LogRequestMetrics) {
	throttleTier := throttleTierForModel(modelTier)
	s.mu.Lock()
	delete(s.throttle, throttleStateKey(credentialID, throttleTier))
	s.mu.Unlock()

	opCtx, cancel := scheduling.WithDefaultWriteTimeout(ctx)
	defer cancel()
	if err := s.recordResponse(opCtx, credentialID, statusCode, modelTier, metrics); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("scheduler: insert success log")
	} else {
		s.refreshCredentialWeight(opCtx, credentialID, modelTier)
	}
}

// RecordFailure records the response for error-rate weighting.
// Throttling is reserved for explicit 429 retry windows or repeated consecutive failures.
func (s *Scheduler) RecordFailure(ctx context.Context, credentialID string, statusCode int32, modelTier string, retryAfter time.Duration, metrics db.LogRequestMetrics) {
	opCtx, cancel := scheduling.WithDefaultWriteTimeout(ctx)
	defer cancel()

	if err := s.recordResponse(opCtx, credentialID, statusCode, modelTier, metrics); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("scheduler: insert failure log")
	} else {
		s.refreshCredentialWeight(opCtx, credentialID, modelTier)
	}

	now := time.Now()
	throttleTier := throttleTierForModel(modelTier)
	throttleKey := throttleStateKey(credentialID, throttleTier)

	s.mu.Lock()
	state, ok := s.throttle[throttleKey]
	if !ok {
		state = &throttleState{}
		s.throttle[throttleKey] = state
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
	if err := s.store.SetQuotaThrottled(opCtx, credentialID, throttleTier, throttledUntil); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("scheduler: set throttled")
	}
	s.rememberThrottleUntil(credentialID, throttleTier, throttledUntil)
	log.Warn().Str("credential", credentialID).Str("model_tier", throttleTier).Dur("backoff", decision.Backoff).Str("reason", decision.Reason).Int("consecutive_failures", consecutive).Msg("credential tier throttled")
}

func throttleTierForModel(modelTier string) string {
	switch modelTier {
	case ModelTierDefault, ModelTierSpark:
		return modelTier
	default:
		return ModelTierDefault
	}
}

// HandleUnauthorized handles auth/account terminal statuses outside the error-rate backoff path.
func (s *Scheduler) HandleUnauthorized(ctx context.Context, credentialID string, statusCode int32, modelTier string, metrics db.LogRequestMetrics) bool {
	if statusCode == http.StatusUnauthorized && isCodexNoMatchingAccessRuleError(metrics.Error) {
		return false
	}
	if isCredentialDirectDisableStatus(int(statusCode)) {
		opCtx, cancel := scheduling.WithDefaultWriteTimeout(ctx)
		defer cancel()
		s.recordAuthRejection(opCtx, credentialID, statusCode, modelTier, metrics)
		s.mu.Lock()
		deleteCredentialThrottleStates(s.throttle, credentialID)
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
		s.manager.DisableCredential(opCtx, credentialID, fmt.Sprintf("credential rejected (%d)", statusCode))
		return true
	}
	if !isCredentialRefreshStatus(int(statusCode)) {
		return false
	}

	s.mu.Lock()
	deleteCredentialThrottleStates(s.throttle, credentialID)
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
		opCtx, cancel := scheduling.WithDefaultWriteTimeout(ctx)
		defer cancel()
		s.recordAuthRejection(opCtx, credentialID, statusCode, modelTier, metrics)
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

// UpdateQuota 只更新内存调度缓存
// 请求头里的 quota 更新很频繁，持久化只留给显式同步路径 StoreQuota
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

// QueueQuotaRefresh 临时摘掉当前 tier 并后台探测 quota
// 探测失败只写内存短退避，不写数据库 throttle 状态
func (s *Scheduler) QueueQuotaRefresh(_ context.Context, credentialID string, modelTier string) {
	if credentialID == "" || s.manager == nil || s.fetcher == nil {
		return
	}
	tier := throttleTierForModel(modelTier)
	select {
	case s.quotaRefreshSem <- struct{}{}:
	default:
		log.Warn().Str("credential", credentialID).Str("model_tier", tier).Msg("scheduler: quota refresh skipped: concurrent limit reached")
		return
	}
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
			s.rememberThrottleUntil(credentialID, tier, time.Now().Add(quotaRefreshFailureBackoff))
			log.Warn().Err(err).Str("credential", credentialID).Str("model_tier", tier).Msg("scheduler: quota refresh get token")
			return
		}

		q, err := s.fetcher.FetchQuota(refreshCtx, credentialID, token)
		if err != nil {
			s.rememberThrottleUntil(credentialID, tier, time.Now().Add(quotaRefreshFailureBackoff))
			log.Warn().Err(err).Str("credential", credentialID).Str("model_tier", tier).Msg("scheduler: quota refresh fetch")
			return
		}
		if q == nil {
			s.rememberThrottleUntil(credentialID, tier, time.Now().Add(quotaRefreshFailureBackoff))
			log.Warn().Str("credential", credentialID).Str("model_tier", tier).Msg("scheduler: quota refresh returned empty quota")
			return
		}
		if !quotaRefreshHasTier(q, tier) {
			s.rememberThrottleUntil(credentialID, tier, time.Now().Add(quotaRefreshFailureBackoff))
			log.Warn().Str("credential", credentialID).Str("model_tier", tier).Msg("scheduler: quota refresh missing requested tier")
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
	planTypeUpdated := s.updatePlanType(ctx, credentialID, q.PlanType)
	cacheUpdated := s.applyQuotaToAvailable(credentialID, q)
	if planTypeUpdated {
		cacheUpdated = false
	}
	if err := s.store.UpsertQuota(ctx, db.UpsertQuotaParams{
		CredentialID:  credentialID,
		Quota5h:       q.Quota5h,
		Quota7d:       q.Quota7d,
		Quota1mo:      q.Quota1mo,
		QuotaSpark5h:  q.QuotaSpark5h,
		QuotaSpark7d:  q.QuotaSpark7d,
		QuotaSpark1mo: q.QuotaSpark1mo,
		Reset5h:       q.Reset5h,
		Reset7d:       q.Reset7d,
		Reset1mo:      q.Reset1mo,
		ResetSpark5h:  q.ResetSpark5h,
		ResetSpark7d:  q.ResetSpark7d,
		ResetSpark1mo: q.ResetSpark1mo,
	}); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("scheduler: upsert quota")
		return
	}
	if !cacheUpdated {
		if _, err := s.RefreshAvailable(ctx); err != nil {
			log.Error().Err(err).Str("credential", credentialID).Msg("scheduler: refresh available after quota insert")
		}
	}
}

func (s *Scheduler) updatePlanType(ctx context.Context, credentialID string, planType string) bool {
	normalized := NormalizePlanType(planType)
	if normalized == "" {
		return false
	}
	if snap := s.available.Load(); snap != nil {
		expectedCode := s.planTypeCodec().code(normalized)
		for _, row := range snap.rows {
			if row.ID == credentialID && row.PlanTypeCode == expectedCode {
				return false
			}
		}
	}
	if _, err := s.store.UpdateCodexPlanType(ctx, credentialID, normalized); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Str("plan_type", normalized).Msg("scheduler: update codex plan type")
		return false
	}
	return true
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

func (s *Scheduler) rememberThrottleUntil(credentialID string, modelTier string, throttledUntil time.Time) {
	throttleTier := throttleTierForModel(modelTier)
	key := throttleStateKey(credentialID, throttleTier)
	s.mu.Lock()
	state, ok := s.throttle[key]
	if !ok {
		state = &throttleState{}
		s.throttle[key] = state
	}
	state.until = throttledUntil
	s.mu.Unlock()

	s.throttleCredentialTier(credentialID, throttleTier, throttledUntil)
	go s.refreshAvailableAfterThrottle(credentialID, throttleTier, throttledUntil)
}

func (s *Scheduler) refreshAvailableAfterThrottle(credentialID string, modelTier string, throttledUntil time.Time) {
	key := throttleStateKey(credentialID, modelTier)
	scheduling.RefreshAfterDeadline(scheduling.RefreshAfterDeadlineConfig{
		Deadline: throttledUntil,
		Superseded: func() bool {
			s.mu.Lock()
			defer s.mu.Unlock()
			state, ok := s.throttle[key]
			return ok && state.until.After(throttledUntil)
		},
		Refresh: func(ctx context.Context) error {
			_, err := s.RefreshAvailable(ctx)
			return err
		},
		ReportError: func(err error) {
			log.Error().Err(err).Str("credential", credentialID).Str("model_tier", modelTier).Msg("scheduler: refresh available after throttle deadline")
		},
	})
}

func throttleStateKey(credentialID string, modelTier string) string {
	return credentialID + "\x00" + throttleTierForModel(modelTier)
}

func deleteCredentialThrottleStates(states map[string]*throttleState, credentialID string) {
	prefix := credentialID + "\x00"
	for key := range states {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(states, key)
		}
	}
}

func activeThrottleUntil(states map[string]*throttleState, credentialID string, modelTier string, now time.Time) time.Time {
	state := states[throttleStateKey(credentialID, modelTier)]
	if state == nil || state.until.IsZero() || !now.Before(state.until) {
		return time.Time{}
	}
	return state.until
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
	if statusCode != http.StatusTooManyRequests && statusCode != http.StatusServiceUnavailable {
		return scheduling.RetryDecision{}
	}
	return scheduling.RetryDecision{Delay: utils.ParseRetryAfterHeader(headers)}
}

func (s *Scheduler) settingsSnapshot() settings.Snapshot {
	if s.settings == nil {
		return settings.DefaultSnapshot()
	}
	return s.settings.Snapshot()
}

func (s *Scheduler) insertLog(ctx context.Context, arg db.InsertLogParams) error {
	if s.logStore == nil {
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
		if since := ErrorRateSince(row.Reset5h, row.Reset7d, row.Reset1mo, config.QuotaWindow5hSeconds(), config.QuotaWindow7dSeconds()); !since.IsZero() {
			defaultSince = append(defaultSince, db.ErrorRateSince{CredentialID: row.ID, Since: since})
		}
		if since := ErrorRateSince(row.ResetSpark5h, row.ResetSpark7d, row.ResetSpark1mo, config.QuotaWindow5hSeconds(), config.QuotaWindow7dSeconds()); !since.IsZero() {
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
	if s.logStore == nil || credentialID == "" {
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
		return ErrorRateSince(row.ResetSpark5h, row.ResetSpark7d, row.ResetSpark1mo, config.QuotaWindow5hSeconds(), config.QuotaWindow7dSeconds()), true
	case ModelTierDefault:
		return ErrorRateSince(row.Reset5h, row.Reset7d, row.Reset1mo, config.QuotaWindow5hSeconds(), config.QuotaWindow7dSeconds()), true
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
