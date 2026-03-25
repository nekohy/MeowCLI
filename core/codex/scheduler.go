package codex

import (
	"context"
	"errors"
	"fmt"
	codexclient "github.com/nekohy/MeowCLI/api/codex"
	codexAPI "github.com/nekohy/MeowCLI/api/codex/utils"
	"math"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

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
	SetQuotaThrottled(ctx context.Context, credentialID string, throttledUntil time.Time) error
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

// availableRow 缓存中的单条凭证，Score 由 calcScore 计算用于排序选择
type availableRow struct {
	ID           string
	PlanTypeCode int
	Score        float64
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
	go func() {
		// 立即执行一次初始同步
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

// syncAllQuotas 遍历所有启用的凭证，获取其上游配额，
// 并将结果写入 quota 表
func (s *Scheduler) syncAllQuotas(ctx context.Context) {
	if s.fetcher == nil {
		return
	}

	rows, err := s.store.ListAvailableCodex(ctx)
	if err != nil {
		log.Error().Err(err).Msg("quota-sync: list credentials")
		return
	}

	for _, row := range rows {
		token, err := s.manager.GetAccessToken(ctx, row.ID)
		if err != nil {
			log.Error().Err(err).Str("credential", row.ID).Msg("quota-sync: get token")
			continue
		}

		quotaCtx, cancel := context.WithTimeout(ctx, s.settingsSnapshot().ImportedCheckTimeout())
		q, err := s.fetcher.FetchQuota(quotaCtx, row.ID, token)
		cancel()
		if err != nil {
			if statusCode, body, ok := codexclient.ParseAPIError(err); ok && isCredentialRejectedStatus(statusCode) {
				s.HandleUnauthorized(ctx, row.ID, int32(statusCode), body)
				continue
			}
			log.Error().Err(err).Str("credential", row.ID).Msg("quota-sync: fetch quota")
			continue
		}

		log.Info().
			Str("credential", row.ID).
			Float64("quota_5h", q.Quota5h).
			Float64("quota_7d", q.Quota7d).
			Time("reset_5h", q.Reset5h).
			Time("reset_7d", q.Reset7d).
			Msg("quota-sync: fetched")

		s.UpdateQuota(ctx, row.ID, q)
	}

	// 同步完成后刷新可用凭证缓存，确保新增/移除的凭证被感知
	if _, err := s.RefreshAvailable(ctx); err != nil {
		log.Error().Err(err).Msg("quota-sync: refresh available cache")
	}
}

// Pick 根据优先级评分选择最佳可用凭证。
// 请求头和设置里的 plan type 偏好会先被转换为内部 int code，再进入缓存调度。
func (s *Scheduler) Pick(ctx context.Context, headers http.Header) (string, error) {
	return s.pickPreferred(ctx, s.preferredPlanTypeCodes(headers))
}

func (s *Scheduler) pickPreferred(ctx context.Context, preferredCodes []int) (string, error) {
	snap, err := s.listAvailable(ctx)
	if err != nil {
		return "", err
	}

	for _, code := range preferredCodes {
		idx, ok := snap.bestByPlanType[code]
		if !ok {
			continue
		}
		if idx >= 0 && idx < len(snap.rows) {
			return snap.rows[idx].ID, nil
		}
	}

	for _, r := range snap.rows {
		if r.Score < 0 {
			continue
		}
		return r.ID, nil
	}

	return "", ErrNoAvailableCredential
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

	config := s.settingsSnapshot()
	planTypes := s.planTypeCodec()
	rows := make([]availableRow, 0, len(dbRows))
	for _, r := range dbRows {
		if r.SyncedAt.IsZero() {
			continue
		}
		if s.isChecking(r.ID) {
			continue
		}
		rows = append(rows, availableRow{
			ID:           r.ID,
			PlanTypeCode: planTypes.code(r.PlanType),
			Score:        calcScore(r.Quota5h, r.Quota7d, r.Reset5h, r.Reset7d, config.QuotaWindow5hSeconds(), config.QuotaWindow7dSeconds()),
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Score > rows[j].Score
	})

	s.available.Store(buildAvailableSnapshot(rows))
	s.pruneExpiredThrottles(time.Now())
	return rows, nil
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
			updated[i].Score = calcScore(q.Quota5h, q.Quota7d, q.Reset5h, q.Reset7d, config.QuotaWindow5hSeconds(), config.QuotaWindow7dSeconds())
			break
		}
	}

	sort.Slice(updated, func(i, j int) bool {
		return updated[i].Score > updated[j].Score
	})

	s.available.Store(buildAvailableSnapshot(updated))
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

func buildAvailableSnapshot(rows []availableRow) *availableSnapshot {
	bestByPlanType := make(map[int]int, len(rows))
	for i, row := range rows {
		if row.Score < 0 {
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

func (s *Scheduler) AuthHeaders(ctx context.Context, credentialID string) (http.Header, error) {
	token, err := s.GetAccessToken(ctx, credentialID)
	if err != nil {
		return nil, err
	}

	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+token)
	headers.Set("Chatgpt-Account-Id", utils.AccountIDFromCredentialID(credentialID))
	return headers, nil
}

func (s *Scheduler) planTypeCodec() *planTypeCodec {
	if s.planTypes == nil {
		s.planTypes = newPlanTypeCodec()
	}
	return s.planTypes
}

func mergePlanTypeCodes(groups ...[]int) []int {
	var merged []int
	seen := make(map[int]struct{})

	for _, group := range groups {
		for _, code := range group {
			if code == planTypeCodeAny {
				continue
			}
			if _, ok := seen[code]; ok {
				continue
			}
			seen[code] = struct{}{}
			merged = append(merged, code)
		}
	}

	return merged
}

// calcScore 根据配额比率和重置时间计算综合优先级评分
// 分数越高越优先使用采用分层加权：
//
//  1. 7d 重置紧迫度（权重 1000）— 优先使用即将重置的凭证
//  2. 7d 剩余配额  （权重 100） — 同等紧迫度下优先配额充沛的
//  3. 5h 重置紧迫度（权重 10）  — 优先使用即将重置的凭证
//  4. 5h 剩余配额  （权重 1）   — 最终决胜因子
func calcScore(quota5h, quota7d float64, reset5h, reset7d time.Time, window5hSeconds, window7dSeconds int64) float64 {
	// 任一窗口的配额完全耗尽（0%）则不可用，避免浪费请求触发 429
	// 注意：无此窗口时 quota 默认 1.0（满配额），不会触发此条件
	if quota5h == 0 || quota7d == 0 {
		return -1
	}

	now := time.Now().Unix()

	u7d := urgencyFactor(reset7d.Unix(), now, window7dSeconds)
	u5h := urgencyFactor(reset5h.Unix(), now, window5hSeconds)

	return u7d*1000 + quota7d*100 + u5h*10 + quota5h
}

// urgencyFactor 返回凭证窗口重置的紧迫程度（秒级精度）
// 0.0 = 刚刚重置（距离下次重置很远），1.0 = 即将重置或已过期
func urgencyFactor(resetAtUnix, nowUnix, windowSeconds int64) float64 {
	if resetAtUnix == 0 || windowSeconds == 0 {
		return 0.0
	}
	remaining := resetAtUnix - nowUnix
	if remaining <= 0 {
		return 1.0
	}
	ratio := float64(remaining) / float64(windowSeconds)
	if ratio >= 1.0 {
		return 0.0
	}
	return 1.0 - ratio
}

// RecordSuccess 记录成功请求并重置退避状态
func (s *Scheduler) RecordSuccess(_ context.Context, credentialID string, statusCode int32) {
	s.mu.Lock()
	delete(s.throttle, credentialID)
	s.mu.Unlock()

	if err := s.insertLog(context.Background(), db.InsertLogParams{
		Handler:      string(utils.HandlerCodex),
		CredentialID: credentialID,
		StatusCode:   statusCode,
		Text:         "",
	}); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("scheduler: insert success log")
	}
}

// RecordFailure 记录失败请求并临时禁用凭证
// 当 retryAfter > 0 时直接使用该时长；否则使用指数退避：
// 基础 1 分钟 × 2^(连续失败次数-1)，上限 30 分钟
func (s *Scheduler) RecordFailure(_ context.Context, credentialID string, statusCode int32, text string, retryAfter time.Duration) {
	// 使用 background context：日志记录和节流是服务端内务操作，
	// 不应受客户端请求 context 取消的影响
	bgCtx := context.Background()

	if err := s.insertLog(bgCtx, db.InsertLogParams{
		Handler:      string(utils.HandlerCodex),
		CredentialID: credentialID,
		StatusCode:   statusCode,
		Text:         text,
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
		backoff = calcBackoff(consecutive, config.ThrottleBase(), config.ThrottleMax())
		reason = fmt.Sprintf("attempt #%d", consecutive)
	}

	throttledUntil := now.Add(backoff)
	if err := s.store.SetQuotaThrottled(bgCtx, credentialID, throttledUntil); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("scheduler: set throttled")
	}
	s.setThrottleUntil(credentialID, throttledUntil)

	// 凭证被节流，从缓存中移除
	s.removeFromAvailable(credentialID)

	log.Warn().Str("credential", credentialID).Dur("backoff", backoff).Str("reason", reason).Msg("credential throttled")
}

// HandleUnauthorized 先将 401/402 凭证踢出可用池，再异步用 refresh token + usage 校验
func (s *Scheduler) HandleUnauthorized(ctx context.Context, credentialID string, statusCode int32, text string) bool {
	if !isCredentialRejectedStatus(int(statusCode)) {
		return false
	}

	if err := s.insertLog(ctx, db.InsertLogParams{
		Handler:      string(utils.HandlerCodex),
		CredentialID: credentialID,
		StatusCode:   statusCode,
		Text:         text,
	}); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("scheduler: insert invalidation log")
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
		log.Warn().
			Str("credential", credentialID).
			Int32("status", statusCode).
			Msg("credential verification skipped because manager is unavailable")
		return true
	}
	go s.verifyCredentialAfterUnauthorized(credentialID)
	return true
}

// GetAccessToken 委托给令牌管理器获取访问令牌
func (s *Scheduler) GetAccessToken(ctx context.Context, credentialID string) (string, error) {
	return s.manager.GetAccessToken(ctx, credentialID)
}

// UpdateQuota 从 API 响应更新凭证配额（写入 DB + 刷新缓存）
// Quota 中的 Reset5h/Reset7d 为 API 直接提供的绝对时间戳，无需二次计算
func (s *Scheduler) UpdateQuota(ctx context.Context, credentialID string, q *codexAPI.Quota) {
	s.updateAvailableQuota(credentialID, q)
	if err := s.store.UpsertQuota(ctx, db.UpsertQuotaParams{
		CredentialID: credentialID,
		Quota5h:      q.Quota5h,
		Quota7d:      q.Quota7d,
		Reset5h:      q.Reset5h,
		Reset7d:      q.Reset7d,
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

func (s *Scheduler) verifyCredentialAfterUnauthorized(credentialID string) {
	ctx, cancel := context.WithTimeout(context.Background(), s.settingsSnapshot().UnauthorizedCheckTimeout())
	defer cancel()
	defer s.finishChecking(credentialID)

	err := s.manager.RefreshCredential(ctx, credentialID)
	switch {
	case err == nil:
		log.Info().Str("credential", credentialID).Msg("credential refresh verification succeeded after auth rejection response")
	case errors.Is(err, ErrRefreshTokenMissing):
		log.Warn().Str("credential", credentialID).Msg("credential removed after auth rejection response because refresh token is missing")
	default:
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

	token, err := s.manager.GetAccessToken(ctx, credentialID)
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

	s.UpdateQuota(ctx, credentialID, q)
	log.Info().Str("credential", credentialID).Msg("credential usage verification succeeded after refresh")
}

func (s *Scheduler) validateImportedCredential(ctx context.Context, credentialID string) {
	token, err := s.manager.GetAccessToken(ctx, credentialID)
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
	s.UpdateQuota(ctx, credentialID, q)
}

// calcBackoff 返回 1分钟 × 2^(n-1)，上限 30 分钟
func calcBackoff(consecutive int, base, max time.Duration) time.Duration {
	if consecutive <= 0 {
		consecutive = 1
	}
	if base <= 0 {
		base = time.Minute
	}
	if max < base {
		max = 30 * time.Minute
	}
	backoff := time.Duration(math.Pow(2, float64(consecutive-1))) * base
	if backoff > max {
		return max
	}
	return backoff
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
