package codex

import (
	"context"
	"errors"
	"fmt"
	CodexAPI "github.com/nekohy/MeowCLI/api/codex"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nekohy/MeowCLI/internal/settings"
	db "github.com/nekohy/MeowCLI/internal/store"
	"github.com/nekohy/MeowCLI/utils"

	"github.com/maypok86/otter/v2"
	"github.com/rs/zerolog/log"
)

var (
	ErrCodexAPIRequired    = errors.New("codex api is required")
	ErrCredentialDisabled  = errors.New("credential is not active")
	ErrRefreshTokenMissing = errors.New("refresh_token is empty")
	ErrFreeCredential      = errors.New("credential deleted because plan type is free")
)

// CodexStore 描述令牌管理器所依赖的 SQL 操作
type CodexStore interface {
	GetCodex(ctx context.Context, id string) (db.Codex, error)
	UpdateCodexTokens(ctx context.Context, arg db.UpdateCodexTokensParams) (db.Codex, error)
	DeleteCodex(ctx context.Context, id string) error
}

// ManagerConfig 配置 codex 令牌管理器
type ManagerConfig struct {
	Store    CodexStore
	Cache    *otter.Cache[string, CodexCache]
	CodexAPI *CodexAPI.Client
	Settings settings.Provider
}

// Manager 协调两个层级
// 1. 进程本地 otter 缓存，
// 2. 持久化令牌存储
type Manager struct {
	store    CodexStore
	cache    *otter.Cache[string, CodexCache]
	codexAPI *CodexAPI.Client
	settings settings.Provider
	entries  sync.Map
}

type CodexEntry struct {
	refreshing atomic.Bool
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
	entry := m.BuildCodexCache(id)

	if snapshot, ok := m.readCache(id); ok {
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
		defer entry.refreshing.Store(false)

		refreshed, err := m.refreshAndWriteBack(ctx, row)
		if err != nil {
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

// DeleteCredential 从 DB、缓存和刷新协调条目中删除无效凭证
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
	row, err := m.store.GetCodex(ctx, id)
	if err != nil {
		return err
	}
	_, err = m.refreshAndWriteBack(ctx, row)
	return err
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
		m.DeleteCredential(ctx, latest.ID, "refresh token is empty")
		return db.Codex{}, ErrRefreshTokenMissing
	}

	// 调用 API 层刷新令牌
	tokenData, retryable, err := m.codexAPI.RefreshAccessToken(ctx, latest.RefreshToken)
	if err != nil {
		if !retryable {
			m.DeleteCredential(ctx, latest.ID, "refresh token invalid")
		}
		return db.Codex{}, fmt.Errorf("refresh tokens for %s: %w", latest.ID, err)
	}

	if tokenData.RefreshToken == "" {
		m.DeleteCredential(ctx, latest.ID, "no refresh token returned")
		return db.Codex{}, fmt.Errorf("no refresh token returned for %s", latest.ID)
	}

	// 解析令牌过期时间
	expire, err := time.Parse(time.RFC3339, tokenData.Expire)
	if err != nil {
		return db.Codex{}, fmt.Errorf("parse token expiry: %w", err)
	}

	// 提取 planType 和 planExpired
	planType := latest.PlanType
	planExpired := latest.PlanExpired
	claims, err := utils.ParseJWT(tokenData.IDToken)
	if err != nil {
		log.Warn().Err(err).Str("credential", latest.ID).Msg("failed to parse ID token")
	} else {
		if pt := claims.GetPlanType(); pt != "" {
			planType = pt
		}
		if until := claims.GetSubscriptionActiveUntil(); !until.IsZero() {
			planExpired = until
		}
	}
	if m.shouldDeleteFreeCredential(planType) {
		m.DeleteCredential(ctx, latest.ID, "plan type is free and auto-delete is enabled")
		return db.Codex{}, ErrFreeCredential
	}

	refreshed, err := m.store.UpdateCodexTokens(ctx, db.UpdateCodexTokensParams{
		ID:           latest.ID,
		Status:       string(utils.StatusEnabled),
		AccessToken:  tokenData.AccessToken,
		Expired:      expire,
		RefreshToken: tokenData.RefreshToken,
		PlanType:     planType,
		PlanExpired:  planExpired,
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
		select {
		case <-ctx.Done():
			return CodexCache{}, fmt.Errorf("waiting for codex token refresh: %w", ctx.Err())
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
				refreshed, err := m.refreshAndWriteBack(ctx, row)
				entry.refreshing.Store(false)
				if err != nil {
					return CodexCache{}, err
				}

				return m.snapshotFromCodex(refreshed), nil
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

// BuildCodexCache 获取或创建刷新协调条目
func (m *Manager) BuildCodexCache(id string) *CodexEntry {
	if actual, ok := m.entries.Load(id); ok {
		return actual.(*CodexEntry)
	}

	created := &CodexEntry{}
	actual, _ := m.entries.LoadOrStore(id, created)
	return actual.(*CodexEntry)
}

// proactiveRefresh 缓存过期时主动刷新令牌，保持缓存常热
func (m *Manager) proactiveRefresh(id string) {
	entry := m.BuildCodexCache(id)
	if !entry.refreshing.CompareAndSwap(false, true) {
		return
	}
	defer entry.refreshing.Store(false)

	ctx := context.Background()
	row, err := m.store.GetCodex(ctx, id)
	if err != nil {
		log.Error().Err(err).Str("credential", id).Msg("proactive-refresh: get codex")
		return
	}

	if ensureCredentialEnabled(row.Status) != nil {
		return
	}

	if _, err := m.refreshAndWriteBack(ctx, row); err != nil {
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

func (m *Manager) shouldDeleteFreeCredential(planType string) bool {
	if !m.settingsSnapshot().CodexDeleteFreeAccounts {
		return false
	}
	return IsFreePlanType(planType)
}
