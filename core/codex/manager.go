package codex

import (
	"context"
	"errors"
	"fmt"
	codexutils "github.com/nekohy/MeowCLI/api/codex/utils"
	"github.com/nekohy/MeowCLI/core/scheduling"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nekohy/MeowCLI/internal/settings"
	db "github.com/nekohy/MeowCLI/internal/store"
	"github.com/nekohy/MeowCLI/utils"

	"github.com/maypok86/otter/v2"
	"github.com/rs/zerolog/log"
)

var _ scheduling.CredentialManager = (*Manager)(nil)

var (
	ErrCodexAPIRequired    = errors.New("codex api is required")
	ErrCredentialDisabled  = errors.New("credential is not active")
	ErrRefreshTokenMissing = errors.New("refresh_token is empty")
)

const defaultManagerRefreshTimeout = 20 * time.Second

// CodexStore 描述令牌管理器所依赖的 SQL 操作
type CodexStore interface {
	GetCodex(ctx context.Context, id string) (db.Codex, error)
	UpdateCodexTokens(ctx context.Context, arg db.UpdateCodexTokensParams) (db.Codex, error)
	DeleteCodex(ctx context.Context, id string) error
	UpdateCodexStatus(ctx context.Context, id string, status string, reason string) (db.Codex, error)
	DeleteQuota(ctx context.Context, credentialID string) error
}

type Client interface {
	RefreshAccessToken(ctx context.Context, refreshToken string) (*codexutils.CodexTokenData, bool, error)
	FetchQuota(ctx context.Context, credentialID string, accessToken string) (*codexutils.Quota, error)
}

// ManagerConfig 配置 codex 令牌管理器
type ManagerConfig struct {
	Store    CodexStore
	Cache    *otter.Cache[string, CodexCache]
	CodexAPI Client
	Settings settings.Provider
}

// Manager 协调两个层级
// 1. 进程本地 otter 缓存，
// 2. 持久化令牌存储
type Manager struct {
	store    CodexStore
	cache    *otter.Cache[string, CodexCache]
	codexAPI Client
	settings settings.Provider
	entries  sync.Map
}

type CodexEntry struct {
	refreshing atomic.Bool
	mu         sync.Mutex
	done       chan struct{}
	lastErr    error
}

// CodexCache 缓存结构
type CodexCache struct {
	ID          string
	PlanType    string
	AccessToken string
}

// NewManager 使用合理的默认值构建令牌管理器
func NewManager(cfg ManagerConfig) (*Manager, error) {
	if cfg.Store == nil {
		return nil, errors.New("codex store is required")
	}
	if cfg.CodexAPI == nil {
		return nil, ErrCodexAPIRequired
	}

	m := &Manager{
		store:    cfg.Store,
		codexAPI: cfg.CodexAPI,
		settings: cfg.Settings,
	}

	cache := cfg.Cache
	if cache == nil {
		var err error
		cache, err = otter.New[string, CodexCache](&otter.Options[string, CodexCache]{
			OnDeletion: func(e otter.DeletionEvent[string, CodexCache]) {
				if e.Cause != otter.CauseExpiration {
					return
				}
				go m.proactiveRefresh(e.Key)
			},
		})
		if err != nil {
			return nil, fmt.Errorf("create codex otter cache: %w", err)
		}
	}

	m.cache = cache
	return m, nil
}

// GetAccessToken 是请求链路的热路径
// 优先使用 otter 缓存，仅在缓存未命中时才走 SQL/刷新流程
func (m *Manager) GetAccessToken(ctx context.Context, id string) (string, error) {
	return m.AccessToken(ctx, id, scheduling.UseCached)
}

func (m *Manager) AccessToken(ctx context.Context, id string, mode scheduling.RefreshMode) (string, error) {
	if mode == scheduling.ForceRefresh {
		return m.forceRefreshCredential(ctx, id)
	}
	return m.cachedAccessToken(ctx, id)
}

func (m *Manager) AuthHeaders(ctx context.Context, id string, mode scheduling.RefreshMode) (http.Header, error) {
	token, err := m.AccessToken(ctx, id, mode)
	if err != nil {
		return nil, err
	}

	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+token)
	headers.Set("Chatgpt-Account-Id", utils.AccountIDFromCredentialID(id))
	return headers, nil
}

func (m *Manager) cachedAccessToken(ctx context.Context, id string) (string, error) {
	entry := m.entry(id)

	if snapshot, ok := m.readCache(id); ok {
		return snapshot.AccessToken, nil
	}
	if entry.refreshing.Load() {
		snapshot, err := m.waitForCachedToken(ctx, entry, id)
		if err != nil {
			return "", err
		}
		return snapshot.AccessToken, nil
	}

	row, err := m.store.GetCodex(ctx, id)
	if err != nil {
		return "", err
	}
	if err := ensureCredentialEnabled(row.Status); err != nil {
		return "", err
	}

	m.writeCache(row)
	if snapshot, ok := m.readCache(id); ok {
		return snapshot.AccessToken, nil
	}

	if entry.refreshing.CompareAndSwap(false, true) {
		entry.beginRefresh()
		var refreshErr error
		defer func() {
			entry.finishRefresh(refreshErr)
			entry.refreshing.Store(false)
		}()

		refreshed, err := m.refreshAndWriteBack(ctx, row)
		if err != nil {
			refreshErr = err
			return "", err
		}

		return m.snapshotFromCodex(refreshed).AccessToken, nil
	}

	snapshot, err := m.waitForNewToken(ctx, entry, id)
	if err != nil {
		return "", err
	}

	return snapshot.AccessToken, nil
}

// DisableCredential 将凭证状态设为 disabled，从缓存和调度中移除，但保留数据库记录
func (m *Manager) DisableCredential(ctx context.Context, id string, reason string) {
	_, err := m.store.UpdateCodexStatus(ctx, id, string(utils.StatusDisabled), reason)
	switch {
	case err == nil:
	case errors.Is(err, db.ErrNotFound):
		log.Warn().Str("credential", id).Msg("credential already absent while disabling")
	default:
		log.Error().Err(err).Str("credential", id).Msg("disable credential in DB failed")
		return
	}

	if m.cache != nil {
		m.cache.Invalidate(id)
	}
	m.entries.Delete(id)

	event := log.Warn().Str("credential", id)
	if reason != "" {
		event = event.Str("reason", reason)
	}
	event.Msg("credential disabled")
}

// DeleteCredential 从 DB、缓存和刷新协调条目中删除凭证，同时清理关联的 quota 记录
func (m *Manager) DeleteCredential(ctx context.Context, id string, reason string) {
	err := m.store.DeleteCodex(ctx, id)
	switch {
	case err == nil:
	case errors.Is(err, db.ErrNotFound):
		log.Warn().Str("credential", id).Msg("credential already absent while deleting")
	default:
		log.Error().Err(err).Str("credential", id).Msg("delete credential from DB failed")
		return
	}

	// 清理关联的 quota 记录
	if qErr := m.store.DeleteQuota(ctx, id); qErr != nil && !errors.Is(qErr, db.ErrNotFound) {
		log.Error().Err(qErr).Str("credential", id).Msg("delete quota for credential failed")
	}

	if m.cache != nil {
		m.cache.Invalidate(id)
	}
	m.entries.Delete(id)

	event := log.Warn().Str("credential", id)
	if reason != "" {
		event = event.Str("reason", reason)
	}
	event.Msg("credential deleted")
}

// RefreshCredential 强制使用 refresh token 校验并刷新指定凭证
func (m *Manager) RefreshCredential(ctx context.Context, id string) error {
	_, err := m.forceRefreshCredential(ctx, id)
	return err
}

func (m *Manager) forceRefreshCredential(ctx context.Context, id string) (string, error) {
	entry := m.entry(id)
	if !entry.refreshing.CompareAndSwap(false, true) {
		snapshot, err := m.waitForCachedToken(ctx, entry, id)
		if err != nil {
			return "", err
		}
		return snapshot.AccessToken, nil
	}
	entry.beginRefresh()
	var refreshErr error
	defer func() {
		entry.finishRefresh(refreshErr)
		entry.refreshing.Store(false)
	}()
	m.invalidateCache(id)

	row, err := m.store.GetCodex(ctx, id)
	if err != nil {
		refreshErr = err
		return "", err
	}
	refreshed, err := m.refreshAndWriteBack(ctx, row)
	if err != nil {
		refreshErr = err
		return "", err
	}
	return refreshed.AccessToken, nil
}

// refreshAndWriteBack 刷新令牌、提取信息、持久化数据库，然后更新缓存
func (m *Manager) refreshAndWriteBack(ctx context.Context, row db.Codex) (db.Codex, error) {
	latest, err := m.store.GetCodex(ctx, row.ID)
	if err != nil {
		return db.Codex{}, err
	}

	if err := ensureCredentialEnabled(latest.Status); err != nil {
		return db.Codex{}, err
	}

	if latest.RefreshToken == "" {
		m.DisableCredential(ctx, latest.ID, "refresh token is empty")
		return db.Codex{}, ErrRefreshTokenMissing
	}

	// 调用 API 层刷新令牌
	refreshCtx, cancel := m.refreshContext(ctx)
	defer cancel()

	tokenData, retryable, err := m.codexAPI.RefreshAccessToken(refreshCtx, latest.RefreshToken)
	if err != nil {
		if !retryable {
			m.DisableCredential(ctx, latest.ID, "refresh token invalid")
		}
		return db.Codex{}, fmt.Errorf("refresh tokens for %s: %w", latest.ID, err)
	}

	if tokenData.RefreshToken == "" {
		m.DisableCredential(ctx, latest.ID, "no refresh token returned")
		return db.Codex{}, fmt.Errorf("no refresh token returned for %s", latest.ID)
	}

	// 解析令牌过期时间
	expire, err := time.Parse(time.RFC3339, tokenData.Expire)
	if err != nil {
		return db.Codex{}, fmt.Errorf("parse token expiry: %w", err)
	}

	// 提取 planType
	planType := latest.PlanType
	claims, err := utils.ParseJWT(tokenData.IDToken)
	if err != nil {
		log.Warn().Err(err).Str("credential", latest.ID).Msg("failed to parse ID token")
	} else {
		if pt := claims.GetPlanType(); pt != "" {
			planType = pt
		}
	}
	refreshed, err := m.store.UpdateCodexTokens(ctx, db.UpdateCodexTokensParams{
		ID:           latest.ID,
		Status:       string(utils.StatusEnabled),
		AccessToken:  tokenData.AccessToken,
		Expired:      expire,
		RefreshToken: tokenData.RefreshToken,
		PlanType:     planType,
	})
	if err != nil {
		return db.Codex{}, fmt.Errorf("update tokens for %s: %w", latest.ID, err)
	}

	if refreshed.AccessToken == "" {
		return db.Codex{}, errors.New("refresh returned empty access token")
	}
	if refreshed.Expired.IsZero() {
		return db.Codex{}, errors.New("refresh returned zero expiry")
	}

	m.writeCache(refreshed)
	return refreshed, nil
}

// waitForNewToken 轮询等待，直到另一个 goroutine 或另一个工作线程生成新Token
func (m *Manager) waitForNewToken(ctx context.Context, entry *CodexEntry, id string) (CodexCache, error) {
	ticker := time.NewTicker(m.settingsSnapshot().PollInterval())
	defer ticker.Stop()

	for {
		done, _ := entry.refreshState()
		select {
		case <-ctx.Done():
			return CodexCache{}, fmt.Errorf("waiting for codex token refresh: %w", ctx.Err())
		case <-done:
			if snapshot, ok := m.readCache(id); ok {
				return snapshot, nil
			}
			_, refreshErr := entry.refreshState()
			if refreshErr != nil {
				return CodexCache{}, fmt.Errorf("waiting for codex token refresh: %w", refreshErr)
			}
			return CodexCache{}, errors.New("waiting for codex token refresh: refresh finished without cached token")
		case <-ticker.C:
			if snapshot, ok := m.readCache(id); ok {
				return snapshot, nil
			}

			row, err := m.store.GetCodex(ctx, id)
			if err != nil {
				return CodexCache{}, err
			}

			if err := ensureCredentialEnabled(row.Status); err != nil {
				return CodexCache{}, err
			}

			m.writeCache(row)
			if snapshot, ok := m.readCache(id); ok {
				return snapshot, nil
			}

			if entry.refreshing.CompareAndSwap(false, true) {
				entry.beginRefresh()
				var refreshErr error
				defer func() {
					entry.finishRefresh(refreshErr)
					entry.refreshing.Store(false)
				}()
				refreshed, err := m.refreshAndWriteBack(ctx, row)
				if err != nil {
					refreshErr = err
					return CodexCache{}, err
				}

				return m.snapshotFromCodex(refreshed), nil
			}
		}
	}
}

func (m *Manager) waitForCachedToken(ctx context.Context, entry *CodexEntry, id string) (CodexCache, error) {
	ticker := time.NewTicker(m.settingsSnapshot().PollInterval())
	defer ticker.Stop()

	for {
		done, _ := entry.refreshState()
		select {
		case <-ctx.Done():
			return CodexCache{}, fmt.Errorf("waiting for forced codex token refresh: %w", ctx.Err())
		case <-done:
			if snapshot, ok := m.readCache(id); ok {
				return snapshot, nil
			}
			_, refreshErr := entry.refreshState()
			if refreshErr != nil {
				return CodexCache{}, fmt.Errorf("waiting for forced codex token refresh: %w", refreshErr)
			}
			return CodexCache{}, errors.New("waiting for forced codex token refresh: refresh finished without cached token")
		case <-ticker.C:
			if snapshot, ok := m.readCache(id); ok {
				return snapshot, nil
			}
		}
	}
}

// snapshotFromCodex 构建缓存快照
func (m *Manager) snapshotFromCodex(row db.Codex) CodexCache {
	return CodexCache{
		ID:          row.ID,
		PlanType:    row.PlanType,
		AccessToken: row.AccessToken,
	}
}

// entry 获取或创建刷新协调条目
func (m *Manager) entry(id string) *CodexEntry {
	if actual, ok := m.entries.Load(id); ok {
		return actual.(*CodexEntry)
	}

	created := &CodexEntry{}
	actual, _ := m.entries.LoadOrStore(id, created)
	return actual.(*CodexEntry)
}

func (e *CodexEntry) beginRefresh() {
	e.mu.Lock()
	e.done = make(chan struct{})
	e.lastErr = nil
	e.mu.Unlock()
}

func (e *CodexEntry) finishRefresh(err error) {
	e.mu.Lock()
	if e.done != nil {
		e.lastErr = err
		close(e.done)
	}
	e.mu.Unlock()
}

func (e *CodexEntry) refreshState() (<-chan struct{}, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.done, e.lastErr
}

func (m *Manager) InvalidateCredential(id string) {
	if m == nil {
		return
	}
	m.invalidateCache(id)
	m.entries.Delete(id)
}

func (m *Manager) invalidateCache(id string) {
	if m == nil {
		return
	}
	if m.cache != nil {
		m.cache.Invalidate(id)
	}
}

// proactiveRefresh 缓存过期时主动刷新令牌，保持缓存常热
func (m *Manager) proactiveRefresh(id string) {
	entry := m.entry(id)
	if !entry.refreshing.CompareAndSwap(false, true) {
		return
	}
	entry.beginRefresh()
	var refreshErr error
	defer func() {
		entry.finishRefresh(refreshErr)
		entry.refreshing.Store(false)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), m.refreshTimeout())
	defer cancel()

	row, err := m.store.GetCodex(ctx, id)
	if err != nil {
		refreshErr = err
		log.Error().Err(err).Str("credential", id).Msg("proactive-refresh: get codex")
		return
	}

	if ensureCredentialEnabled(row.Status) != nil {
		return
	}

	if _, err := m.refreshAndWriteBack(ctx, row); err != nil {
		refreshErr = err
		log.Error().Err(err).Str("credential", id).Msg("proactive-refresh: failed")
	} else {
		log.Info().Str("credential", id).Msg("proactive-refresh: success")
	}
}

func ensureCredentialEnabled(status string) error {
	accountStatus, err := utils.ParseAccountStatus(status)
	if err != nil {
		return err
	}
	if accountStatus != utils.StatusEnabled {
		return ErrCredentialDisabled
	}

	return nil
}

func (m *Manager) availableUntil(row db.Codex) time.Time {
	return row.Expired.Add(-m.settingsSnapshot().RefreshBefore())
}

func (m *Manager) settingsSnapshot() settings.Snapshot {
	if m == nil || m.settings == nil {
		return settings.DefaultSnapshot()
	}
	return m.settings.Snapshot()
}

func (m *Manager) refreshTimeout() time.Duration {
	return defaultManagerRefreshTimeout
}

func (m *Manager) refreshContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, m.refreshTimeout())
}
