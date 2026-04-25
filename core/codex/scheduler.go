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

	"github.com/rs/zerolog/log"
)

var (
	ErrNoAvailableCredential = errors.New("no available codex credential")
)

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
	verifyCredential func(string)
	importSyncMu     sync.Mutex
	available        atomic.Pointer[availableSnapshot]
	planTypes        *planTypeCodec
}

// NewScheduler 创建一个连接到指定存储和令牌管理器的调度器
func NewScheduler(store SchedulerStore, manager *Manager) *Scheduler {
	var fetcher QuotaFetcher
	if manager != nil {
		fetcher = manager.codexAPI
	}
	return &Scheduler{
		store:     store,
		manager:   manager,
		fetcher:   fetcher,
		throttle:  make(map[string]*throttleState),
		checking:  make(map[string]struct{}),
		planTypes: newPlanTypeCodec(),
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
		Interval: func() time.Duration {
			return s.settingsSnapshot().QuotaSyncInterval()
		},
		List: s.store.ListAvailableCodex,
		Refresh: func(ctx context.Context, rows []db.ListAvailableCodexRow) {
			s.refreshAvailableFromRows(ctx, rows)
		},
		Sync:     s.syncQuotaRow,
		RowID:    func(row db.ListAvailableCodexRow) string { return row.ID },
		SyncedAt: func(row db.ListAvailableCodexRow) time.Time { return row.SyncedAt },
		WithSyncedAt: func(row db.ListAvailableCodexRow, syncedAt time.Time) db.ListAvailableCodexRow {
			row.SyncedAt = syncedAt
			return row
		},
		LogError: func(err error, message string) {
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
			s.HandleUnauthorized(ctx, row.ID, int32(statusCode), body, "")
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

// Pick 根据优先级评分选择最佳可用凭证。
// 调用方可传入一个偏好的 credentialID；若不可用则返回无可用凭证，不再回退到其他凭证。
func (s *Scheduler) Pick(ctx context.Context, headers http.Header, preferredCredentialID string, allowedPlanTypes []string) (string, error) {
	codec := s.planTypeCodec()
	return s.pickPreferred(ctx, s.preferredPlanTypeCodes(headers), codec.codesFor(allowedPlanTypes), preferredCredentialID, "")
}

// PickWithTier selects the best available credential for a specific model tier.
// When modelTier is "spark", scoring only considers spark quota.
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
func (s *Scheduler) rowScoreForTier(row availableRow, modelTier string) float64 {
	if modelTier == ModelTierSpark {
		return scheduling.AdjustedScore(row.ScoreSpark, row.WeightSpark)
	}
	return scheduling.AdjustedScore(row.Score, row.Weight)
}

// listAvailable 返回内存缓存中的可用凭证列表
// 缓存由 RefreshAvailable 初始化，由 UpdateQuota / removeFromAvailable 等事件驱动更新，
// 不再使用 TTL 过期机制
func (s *Scheduler) listAvailable(ctx context.Context) (*availableSnapshot, error) {
	if snap := s.available.Load(); snap != nil {
		if s.hasExpiredThrottle(time.Now()) {
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
	dbRows, err := s.store.ListAvailableCodex(ctx)
	if err != nil {
		return nil, fmt.Errorf("list available codex: %w", err)
	}

	return s.refreshAvailableFromRows(ctx, dbRows), nil
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
		if s.isChecking(r.ID) {
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
	s.pruneExpiredThrottles(time.Now())
	return rows
}

// updateAvailableQuota 更新缓存中指定凭证的评分并重新排序
func (s *Scheduler) updateAvailableQuota(id string, q *codexAPI.Quota) {
	s.mu.Lock()
	defer s.mu.Unlock()

	snap := s.available.Load()
	if snap == nil {
		return
	}

	config := s.settingsSnapshot()
	updated := make([]availableRow, len(snap.rows))
	copy(updated, snap.rows)

	for i := range updated {
		if updated[i].ID == id {
			newScore := CalcScore(q.Quota5h, q.Quota7d, q.Reset5h, q.Reset7d, config.QuotaWindow5hSeconds(), config.QuotaWindow7dSeconds())
			newScoreSpark := CalcScoreSpark(q.QuotaSpark5h, q.QuotaSpark7d, q.ResetSpark5h, q.ResetSpark7d, config.QuotaWindow5hSeconds(), config.QuotaWindow7dSeconds())
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
			break
		}
	}

	sort.Slice(updated, func(i, j int) bool {
		return scheduling.AdjustedScore(updated[i].Score, updated[i].Weight) > scheduling.AdjustedScore(updated[j].Score, updated[j].Weight)
	})

	s.available.Store(buildAvailableSnapshot(updated))
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

// removeFromAvailable 从缓存中移除指定凭证（被节流时调用）
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

// CalcScore 根据配额比率和重置时间计算综合优先级评分
// 分数越高越优先使用采用分层加权：
//
//  1. 7d 重置紧迫度（权重 1000）— 优先使用即将重置的凭证
//  2. 7d 剩余配额  （权重 100） — 同等紧迫度下优先配额充沛的
//  3. 5h 重置紧迫度（权重 10）  — 优先使用即将重置的凭证
//  4. 5h 剩余配额  （权重 1）   — 最终决胜因子
func CalcScore(quota5h, quota7d float64, reset5h, reset7d time.Time, window5hSeconds, window7dSeconds int64) float64 {
	// 任一窗口的配额完全耗尽（0%）则不可用，避免浪费请求触发 429
	// 注意：无此窗口时 quota 默认 1.0（满配额），不会触发此条件
	if quota5h == 0 || quota7d == 0 {
		return -1
	}

	now := time.Now().Unix()

	u7d := scheduling.UrgencyFactor(reset7d.Unix(), now, window7dSeconds)
	u5h := scheduling.UrgencyFactor(reset5h.Unix(), now, window5hSeconds)

	return u7d*1000 + quota7d*100 + u5h*10 + quota5h
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
	now := time.Now().Unix()
	u7d := scheduling.UrgencyFactor(resetSpark7d.Unix(), now, window7dSeconds)
	u5h := scheduling.UrgencyFactor(resetSpark5h.Unix(), now, window5hSeconds)
	return u7d*1000 + quotaSpark7d*100 + u5h*10 + quotaSpark5h
}

func ErrorRateSince(reset5h, reset7d time.Time, window5hSeconds, window7dSeconds int64) time.Time {
	return scheduling.LatestWindowStart(
		scheduling.WindowStart(reset5h, window5hSeconds),
		scheduling.WindowStart(reset7d, window7dSeconds),
	)
}

// RecordSuccess 记录成功请求并重置退避状态
func (s *Scheduler) RecordSuccess(_ context.Context, credentialID string, statusCode int32, modelTier string) {
	s.mu.Lock()
	delete(s.throttle, credentialID)
	s.mu.Unlock()

	if err := s.insertLog(context.Background(), db.InsertLogParams{
		Handler:      string(utils.HandlerCodex),
		CredentialID: credentialID,
		StatusCode:   statusCode,
		Text:         "",
		ModelTier:    modelTier,
	}); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("scheduler: insert success log")
	}
}

// RecordFailure 记录失败请求并临时禁用凭证
// 当 retryAfter > 0 时直接使用该时长；否则使用指数退避：
// 基础 1 分钟 × 2^(连续失败次数-1)，上限 30 分钟
func (s *Scheduler) RecordFailure(_ context.Context, credentialID string, statusCode int32, text string, retryAfter time.Duration, modelTier string) {
	// 使用 background context：日志记录和节流是服务端内务操作，
	// 不应受客户端请求 context 取消的影响
	bgCtx := context.Background()

	if err := s.insertLog(bgCtx, db.InsertLogParams{
		Handler:      string(utils.HandlerCodex),
		CredentialID: credentialID,
		StatusCode:   statusCode,
		Text:         text,
		ModelTier:    modelTier,
	}); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("scheduler: insert failure log")
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

	var backoff time.Duration
	var reason string
	if retryAfter > 0 {
		backoff = retryAfter
		reason = "Retry-After"
	} else {
		config := s.settingsSnapshot()
		backoff = utils.CalcBackoff(consecutive, config.ThrottleBase(), config.ThrottleMax())
		reason = fmt.Sprintf("attempt #%d", consecutive)
	}

	throttledUntil := now.Add(backoff)
	if err := s.store.SetQuotaThrottled(bgCtx, credentialID, modelTier, throttledUntil); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("scheduler: set throttled")
	}
	s.setThrottleUntil(credentialID, throttledUntil)

	if statusCode == http.StatusTooManyRequests && modelTier != "" {
		s.markTierUnavailable(credentialID, modelTier)
		log.Warn().Str("credential", credentialID).Str("model_tier", modelTier).Dur("backoff", backoff).Str("reason", reason).Msg("credential tier throttled")
		return
	}

	// 凭证被节流，从缓存中移除
	s.removeFromAvailable(credentialID)

	log.Warn().Str("credential", credentialID).Dur("backoff", backoff).Str("reason", reason).Msg("credential throttled")
}

// HandleUnauthorized 先将 401/402 凭证踢出可用池，再异步用 refresh token + usage 校验
func (s *Scheduler) HandleUnauthorized(ctx context.Context, credentialID string, statusCode int32, text string, modelTier string) bool {
	if !isCredentialRejectedStatus(int(statusCode)) {
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
		s.recordUnauthorizedLog(ctx, credentialID, statusCode, text, modelTier, "scheduler: insert invalidation log")
		log.Warn().
			Str("credential", credentialID).
			Int32("status", statusCode).
			Msg("credential verification skipped because manager is unavailable")
		return true
	}
	go s.verifyCredentialAfterUnauthorized(credentialID, statusCode, text, modelTier)
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
	s.updateAvailableQuota(credentialID, q)
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
	s.updateAvailableQuota(credentialID, q)
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
			s.validateImportedCredential(validationCtx, id)
			cancel()
		}

		if _, err := s.RefreshAvailable(context.Background()); err != nil {
			log.Error().Err(err).Msg("sync-credentials: refresh available cache")
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
		log.Info().Str("credential", credentialID).Msg("credential refresh verification succeeded after auth rejection response")
	case errors.Is(err, ErrRefreshTokenMissing):
		s.recordUnauthorizedLog(ctx, credentialID, statusCode, text, modelTier, "scheduler: insert invalidation log")
		log.Warn().Str("credential", credentialID).Msg("credential removed after auth rejection response because refresh token is missing")
	default:
		s.recordUnauthorizedLog(ctx, credentialID, statusCode, text, modelTier, "scheduler: insert invalidation log")
		log.Warn().Err(err).Str("credential", credentialID).Msg("credential refresh verification finished with error after auth rejection response")
	}
	if err == nil {
		s.validateCredentialUsageAfterRefresh(ctx, credentialID)
	}

	if _, refreshErr := s.RefreshAvailable(context.Background()); refreshErr != nil {
		log.Error().Err(refreshErr).Msg("scheduler: refresh available after unauthorized verification")
	}
}

func (s *Scheduler) validateCredentialUsageAfterRefresh(ctx context.Context, credentialID string) {
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
				Text:         body,
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

func (s *Scheduler) validateImportedCredential(ctx context.Context, credentialID string) {
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
				Text:         body,
			}); logErr != nil {
				log.Error().Err(logErr).Str("credential", credentialID).Msg("scheduler: insert import validation failure log")
			}
			s.removeFromAvailable(credentialID)
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
	return statusCode == http.StatusUnauthorized || statusCode == http.StatusPaymentRequired
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

func (s *Scheduler) recordUnauthorizedLog(ctx context.Context, credentialID string, statusCode int32, text string, modelTier string, failureMsg string) {
	if err := s.insertLog(ctx, db.InsertLogParams{
		Handler:      string(utils.HandlerCodex),
		CredentialID: credentialID,
		StatusCode:   statusCode,
		Text:         text,
		ModelTier:    modelTier,
	}); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg(failureMsg)
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
