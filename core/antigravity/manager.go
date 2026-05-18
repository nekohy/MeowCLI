package antigravity

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	antigravityapi "github.com/nekohy/MeowCLI/api/antigravity"
	"github.com/nekohy/MeowCLI/internal/settings"
	db "github.com/nekohy/MeowCLI/internal/store"
	"github.com/nekohy/MeowCLI/utils"

	"github.com/maypok86/otter/v2"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/singleflight"
)

const (
	defaultCacheExpiration = 24 * time.Hour
	defaultLoadTimeout     = 20 * time.Second
	pruneEntryIdleAfter    = 5 * time.Minute
)

type ManagerStore interface {
	GetAntigravity(ctx context.Context, id string) (db.AntigravityCredential, error)
	UpdateAntigravityTokens(ctx context.Context, arg db.UpdateAntigravityTokensParams) (db.AntigravityCredential, error)
}

type Client interface {
	RefreshAccessToken(ctx context.Context, refreshToken string) (*antigravityapi.TokenData, error)
}

type Manager struct {
	store      ManagerStore
	client     Client
	settings   settings.Provider
	cache      *otter.Cache[string, CredentialCache]
	entries    sync.Map
	loadGroup  singleflight.Group
	refreshSem chan struct{}
}

type ManagerConfig struct {
	Store    ManagerStore
	Client   Client
	Settings settings.Provider
	Cache    *otter.Cache[string, CredentialCache]
}

type CredentialCache struct {
	ID          string
	AccessToken string
	ProjectID   string
	Expired     time.Time
}

type credentialEntry struct {
	refreshing atomic.Bool
	mu         sync.Mutex
	done       chan struct{}
	lastErr    error
	lastUsed   atomic.Int64
}

func NewManager(cfg ManagerConfig) (*Manager, error) {
	if cfg.Store == nil {
		return nil, fmt.Errorf("antigravity manager store is required")
	}
	if cfg.Client == nil {
		return nil, fmt.Errorf("antigravity manager client is required")
	}
	m := &Manager{
		store:      cfg.Store,
		client:     cfg.Client,
		settings:   cfg.Settings,
		refreshSem: make(chan struct{}, 16),
	}

	cache := cfg.Cache
	if cache == nil {
		var err error
		cache, err = otter.New[string, CredentialCache](&otter.Options[string, CredentialCache]{
			ExpiryCalculator: otter.ExpiryWriting[string, CredentialCache](defaultCacheExpiration),
			OnDeletion: func(e otter.DeletionEvent[string, CredentialCache]) {
				if e.Cause != otter.CauseExpiration {
					return
				}
				m.enqueueProactiveRefresh(e.Key)
			},
		})
		if err != nil {
			return nil, fmt.Errorf("create antigravity otter cache: %w", err)
		}
	}
	m.cache = cache
	return m, nil
}

func (m *Manager) GetAccessToken(ctx context.Context, credentialID string) (string, error) {
	return m.AccessToken(ctx, credentialID)
}

func (m *Manager) AccessToken(ctx context.Context, credentialID string) (string, error) {
	row, err := m.cachedCredential(ctx, credentialID)
	if err != nil {
		return "", err
	}
	return row.AccessToken, nil
}

func (m *Manager) AuthHeaders(ctx context.Context, credentialID string) (http.Header, error) {
	row, err := m.cachedCredential(ctx, credentialID)
	if err != nil {
		return nil, err
	}

	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+row.AccessToken)
	return headers, nil
}

func (m *Manager) RefreshCredential(ctx context.Context, credentialID string) error {
	_, err := m.forceRefreshCredential(ctx, credentialID)
	return err
}

func (m *Manager) InvalidateCredential(credentialID string) {
	m.invalidateCache(credentialID)
	m.entries.Delete(credentialID)
}

func (m *Manager) ProjectID(ctx context.Context, credentialID string) (string, error) {
	row, err := m.cachedCredential(ctx, credentialID)
	if err != nil {
		return "", err
	}
	return row.ProjectID, nil
}

func (m *Manager) cachedCredential(ctx context.Context, credentialID string) (db.AntigravityCredential, error) {
	entry := m.entry(credentialID)
	if snapshot, ok := m.readCache(credentialID); ok {
		return m.credentialFromCache(snapshot), nil
	}
	if entry.refreshing.Load() {
		return m.waitForCachedCredential(ctx, entry, credentialID)
	}

	row, err := m.loadAntigravity(ctx, credentialID)
	if err != nil {
		return db.AntigravityCredential{}, err
	}
	if strings.TrimSpace(row.ProjectID) == "" {
		row.ProjectID = utils.ProjectIDFromCredentialID(row.ID)
	}
	needsRefresh := strings.TrimSpace(row.AccessToken) == "" || m.shouldRefresh(row.Expired)
	if !needsRefresh && strings.TrimSpace(row.ProjectID) != "" {
		m.writeCache(row)
		if snapshot, ok := m.readCache(credentialID); ok {
			return m.credentialFromCache(snapshot), nil
		}
		return row, nil
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
		}
		return refreshed, err
	}

	return m.waitForCredential(ctx, entry, credentialID)
}

func (m *Manager) forceRefreshCredential(ctx context.Context, credentialID string) (db.AntigravityCredential, error) {
	entry := m.entry(credentialID)
	if !entry.refreshing.CompareAndSwap(false, true) {
		return m.waitForCachedCredential(ctx, entry, credentialID)
	}
	entry.beginRefresh()
	var refreshErr error
	defer func() {
		entry.finishRefresh(refreshErr)
		entry.refreshing.Store(false)
	}()
	m.invalidateCache(credentialID)

	row, err := m.loadAntigravity(ctx, credentialID)
	if err != nil {
		refreshErr = err
		return db.AntigravityCredential{}, err
	}

	row.AccessToken = ""
	row.Expired = time.Time{}
	refreshed, err := m.refreshAndWriteBack(ctx, row)
	if err != nil {
		refreshErr = err
	}
	return refreshed, err
}

func (m *Manager) refreshAndWriteBack(ctx context.Context, row db.AntigravityCredential) (db.AntigravityCredential, error) {
	accessToken := strings.TrimSpace(row.AccessToken)
	refreshToken := strings.TrimSpace(row.RefreshToken)
	expiry := row.Expired
	projectID := strings.TrimSpace(row.ProjectID)
	if projectID == "" {
		projectID = utils.ProjectIDFromCredentialID(row.ID)
	}
	if accessToken == "" || m.shouldRefresh(expiry) {
		if refreshToken == "" {
			return db.AntigravityCredential{}, fmt.Errorf("antigravity credential %s has no refresh token", row.ID)
		}

		tokenData, err := m.client.RefreshAccessToken(ctx, refreshToken)
		if err != nil {
			return db.AntigravityCredential{}, err
		}
		if tokenData == nil {
			return db.AntigravityCredential{}, fmt.Errorf("refresh antigravity tokens for %s: empty token response", row.ID)
		}
		accessToken = strings.TrimSpace(tokenData.AccessToken)
		refreshToken = strings.TrimSpace(tokenData.RefreshToken)
		expiry = tokenData.Expiry
		if accessToken == "" {
			return db.AntigravityCredential{}, fmt.Errorf("refresh antigravity tokens for %s: empty access token", row.ID)
		}
		if refreshToken == "" {
			return db.AntigravityCredential{}, fmt.Errorf("refresh antigravity tokens for %s: empty refresh token", row.ID)
		}
		if expiry.IsZero() {
			return db.AntigravityCredential{}, fmt.Errorf("refresh antigravity tokens for %s: zero expiry", row.ID)
		}
	}

	if projectID == "" {
		return db.AntigravityCredential{}, fmt.Errorf("antigravity credential %s has no project_id", row.ID)
	}

	updated, err := m.store.UpdateAntigravityTokens(ctx, db.UpdateAntigravityTokensParams{
		ID:           row.ID,
		Status:       "enabled",
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Expired:      expiry,
		Email:        row.Email,
		ProjectID:    projectID,
		PlanType:     row.PlanType,
	})
	if err != nil {
		return db.AntigravityCredential{}, err
	}
	m.writeCache(updated)
	return updated, nil
}

func (m *Manager) waitForCredential(ctx context.Context, entry *credentialEntry, credentialID string) (db.AntigravityCredential, error) {
	for {
		done, _ := entry.refreshState()
		if done == nil {
			if err := sleepUntilRefreshStarts(ctx); err != nil {
				return db.AntigravityCredential{}, fmt.Errorf("waiting for antigravity token refresh: %w", err)
			}
			continue
		}
		select {
		case <-ctx.Done():
			return db.AntigravityCredential{}, fmt.Errorf("waiting for antigravity token refresh: %w", ctx.Err())
		case <-done:
			if snapshot, ok := m.readCache(credentialID); ok {
				return m.credentialFromCache(snapshot), nil
			}
			_, refreshErr := entry.refreshState()
			if refreshErr != nil {
				return db.AntigravityCredential{}, fmt.Errorf("waiting for antigravity token refresh: %w", refreshErr)
			}
			return db.AntigravityCredential{}, fmt.Errorf("waiting for antigravity token refresh: refresh finished without cached credential")
		}
	}
}

func (m *Manager) waitForCachedCredential(ctx context.Context, entry *credentialEntry, credentialID string) (db.AntigravityCredential, error) {
	for {
		done, _ := entry.refreshState()
		if done == nil {
			if err := sleepUntilRefreshStarts(ctx); err != nil {
				return db.AntigravityCredential{}, fmt.Errorf("waiting for forced antigravity token refresh: %w", err)
			}
			continue
		}
		select {
		case <-ctx.Done():
			return db.AntigravityCredential{}, fmt.Errorf("waiting for forced antigravity token refresh: %w", ctx.Err())
		case <-done:
			if snapshot, ok := m.readCache(credentialID); ok {
				return m.credentialFromCache(snapshot), nil
			}
			_, refreshErr := entry.refreshState()
			if refreshErr != nil {
				return db.AntigravityCredential{}, fmt.Errorf("waiting for forced antigravity token refresh: %w", refreshErr)
			}
			return db.AntigravityCredential{}, fmt.Errorf("waiting for forced antigravity token refresh: refresh finished without cached credential")
		}
	}
}

func (m *Manager) readCache(id string) (CredentialCache, bool) {
	if m.cache == nil {
		return CredentialCache{}, false
	}
	return m.cache.GetIfPresent(id)
}

func (m *Manager) writeCache(row db.AntigravityCredential) {
	projectID := strings.TrimSpace(row.ProjectID)
	if projectID == "" {
		projectID = utils.ProjectIDFromCredentialID(row.ID)
	}
	if m.cache == nil || strings.TrimSpace(row.AccessToken) == "" || projectID == "" {
		return
	}
	ttl := time.Until(row.Expired.Add(-m.settingsSnapshot().RefreshBefore()))
	if ttl <= 0 {
		return
	}
	snapshot := CredentialCache{
		ID:          row.ID,
		AccessToken: strings.TrimSpace(row.AccessToken),
		ProjectID:   projectID,
		Expired:     row.Expired,
	}
	m.cache.Set(row.ID, snapshot)
	m.cache.SetExpiresAfter(row.ID, ttl)
}

func (m *Manager) credentialFromCache(snapshot CredentialCache) db.AntigravityCredential {
	projectID := strings.TrimSpace(snapshot.ProjectID)
	if projectID == "" {
		projectID = utils.ProjectIDFromCredentialID(snapshot.ID)
	}
	return db.AntigravityCredential{
		ID:          snapshot.ID,
		AccessToken: snapshot.AccessToken,
		ProjectID:   projectID,
		Expired:     snapshot.Expired,
	}
}

func (m *Manager) entry(id string) *credentialEntry {
	if actual, ok := m.entries.Load(id); ok {
		entry := actual.(*credentialEntry)
		entry.touch()
		return entry
	}
	created := &credentialEntry{}
	created.touch()
	actual, _ := m.entries.LoadOrStore(id, created)
	entry := actual.(*credentialEntry)
	entry.touch()
	return entry
}

func (e *credentialEntry) beginRefresh() {
	e.touch()
	e.mu.Lock()
	e.done = make(chan struct{})
	e.lastErr = nil
	e.mu.Unlock()
}

func (e *credentialEntry) finishRefresh(err error) {
	e.touch()
	e.mu.Lock()
	if e.done != nil {
		e.lastErr = err
		close(e.done)
	}
	e.mu.Unlock()
}

func (e *credentialEntry) refreshState() (<-chan struct{}, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.done, e.lastErr
}

func (e *credentialEntry) touch() {
	e.lastUsed.Store(time.Now().UnixNano())
}

func (m *Manager) PruneStaleEntries() {
	if m.cache == nil {
		return
	}
	m.entries.Range(func(key, value any) bool {
		id := key.(string)
		if _, ok := m.cache.GetIfPresent(id); !ok {
			entry := value.(*credentialEntry)
			lastUsed := time.Unix(0, entry.lastUsed.Load())
			if !entry.refreshing.Load() && time.Since(lastUsed) > pruneEntryIdleAfter {
				m.entries.Delete(id)
			}
		}
		return true
	})
}

func (m *Manager) invalidateCache(id string) {
	if m.cache != nil {
		m.cache.Invalidate(id)
	}
}

func (m *Manager) loadAntigravity(ctx context.Context, id string) (db.AntigravityCredential, error) {
	result := m.loadGroup.DoChan(id, func() (any, error) {
		loadCtx, cancel := context.WithTimeout(context.Background(), defaultLoadTimeout)
		defer cancel()
		return m.store.GetAntigravity(loadCtx, id)
	})

	select {
	case <-ctx.Done():
		return db.AntigravityCredential{}, ctx.Err()
	case res := <-result:
		if res.Err != nil {
			return db.AntigravityCredential{}, res.Err
		}
		return res.Val.(db.AntigravityCredential), nil
	}
}

func sleepUntilRefreshStarts(ctx context.Context) error {
	timer := time.NewTimer(time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (m *Manager) enqueueProactiveRefresh(id string) {
	go func() {
		m.refreshSem <- struct{}{}
		defer func() { <-m.refreshSem }()
		m.proactiveRefresh(id)
	}()
}

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

	ctx, cancel := context.WithTimeout(context.Background(), defaultLoadTimeout)
	defer cancel()

	row, err := m.store.GetAntigravity(ctx, id)
	if err != nil {
		refreshErr = err
		log.Error().Err(err).Str("credential", id).Msg("antigravity proactive-refresh: get credential")
		return
	}
	if _, err := m.refreshAndWriteBack(ctx, row); err != nil {
		refreshErr = err
		log.Error().Err(err).Str("credential", id).Msg("antigravity proactive-refresh: failed")
	}
}

func (m *Manager) shouldRefresh(expiry time.Time) bool {
	if expiry.IsZero() {
		return true
	}
	return time.Now().Add(m.settingsSnapshot().RefreshBefore()).After(expiry)
}

func (m *Manager) settingsSnapshot() settings.Snapshot {
	if m.settings == nil {
		return settings.DefaultSnapshot()
	}
	return m.settings.Snapshot()
}
