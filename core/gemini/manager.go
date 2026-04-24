package gemini

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	geminiapi "github.com/nekohy/MeowCLI/api/gemini"
	"github.com/nekohy/MeowCLI/core/scheduling"
	"github.com/nekohy/MeowCLI/internal/settings"
	db "github.com/nekohy/MeowCLI/internal/store"

	"github.com/maypok86/otter/v2"
	"github.com/rs/zerolog/log"
)

var _ scheduling.CredentialManager = (*Manager)(nil)

type Store interface {
	GetGeminiCLI(ctx context.Context, id string) (db.GeminiCredential, error)
	UpdateGeminiTokens(ctx context.Context, arg db.UpdateGeminiTokensParams) (db.GeminiCredential, error)
}

type Client interface {
	RefreshAccessToken(ctx context.Context, refreshToken string) (*geminiapi.TokenData, error)
	ResolveProjectID(ctx context.Context, accessToken string) (string, error)
}

type Manager struct {
	store    Store
	client   Client
	settings settings.Provider
	cache    *otter.Cache[string, GeminiCache]
	entries  sync.Map
}

type ManagerConfig struct {
	Store    Store
	Client   Client
	Settings settings.Provider
	Cache    *otter.Cache[string, GeminiCache]
}

type GeminiCache struct {
	ID          string
	AccessToken string
	ProjectID   string
	Expired     time.Time
}

type GeminiEntry struct {
	refreshing atomic.Bool
}

func NewManager(cfg ManagerConfig) (*Manager, error) {
	if cfg.Store == nil {
		return nil, fmt.Errorf("gemini manager store is required")
	}
	if cfg.Client == nil {
		return nil, fmt.Errorf("gemini manager client is required")
	}
	m := &Manager{
		store:    cfg.Store,
		client:   cfg.Client,
		settings: cfg.Settings,
	}

	cache := cfg.Cache
	if cache == nil {
		var err error
		cache, err = otter.New[string, GeminiCache](&otter.Options[string, GeminiCache]{
			OnDeletion: func(e otter.DeletionEvent[string, GeminiCache]) {
				if e.Cause != otter.CauseExpiration {
					return
				}
				go m.proactiveRefresh(e.Key)
			},
		})
		if err != nil {
			return nil, fmt.Errorf("create gemini otter cache: %w", err)
		}
	}
	m.cache = cache
	return m, nil
}

func (m *Manager) GetAccessToken(ctx context.Context, credentialID string) (string, error) {
	return m.AccessToken(ctx, credentialID, scheduling.UseCached)
}

func (m *Manager) GetProjectID(ctx context.Context, credentialID string) (string, error) {
	row, err := m.credential(ctx, credentialID, scheduling.UseCached)
	if err != nil {
		return "", err
	}
	return row.ProjectID, nil
}

func (m *Manager) GetAuthHeaders(ctx context.Context, credentialID string) (http.Header, error) {
	return m.AuthHeaders(ctx, credentialID, scheduling.UseCached)
}

func (m *Manager) AccessToken(ctx context.Context, credentialID string, mode scheduling.RefreshMode) (string, error) {
	row, err := m.credential(ctx, credentialID, mode)
	if err != nil {
		return "", err
	}
	return row.AccessToken, nil
}

func (m *Manager) AuthHeaders(ctx context.Context, credentialID string, mode scheduling.RefreshMode) (http.Header, error) {
	row, err := m.credential(ctx, credentialID, mode)
	if err != nil {
		return nil, err
	}

	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+row.AccessToken)
	headers.Set("X-Meow-Gemini-Project", strings.TrimSpace(row.ProjectID))
	return headers, nil
}

func (m *Manager) credential(ctx context.Context, credentialID string, mode scheduling.RefreshMode) (db.GeminiCredential, error) {
	if mode == scheduling.ForceRefresh {
		return m.forceRefreshCredential(ctx, credentialID)
	}
	return m.cachedCredential(ctx, credentialID)
}

func (m *Manager) cachedCredential(ctx context.Context, credentialID string) (db.GeminiCredential, error) {
	entry := m.entry(credentialID)
	if snapshot, ok := m.readCache(credentialID); ok {
		return m.credentialFromCache(snapshot), nil
	}
	if entry.refreshing.Load() {
		return m.waitForCachedCredential(ctx, credentialID)
	}

	row, err := m.store.GetGeminiCLI(ctx, credentialID)
	if err != nil {
		return db.GeminiCredential{}, err
	}
	if strings.TrimSpace(row.ProjectID) == "" {
		row.ProjectID = CredentialProjectID(row.ID)
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
		defer entry.refreshing.Store(false)
		return m.refreshAndWriteBack(ctx, row)
	}

	return m.waitForCredential(ctx, entry, credentialID)
}

func (m *Manager) RefreshCredential(ctx context.Context, credentialID string) error {
	_, err := m.forceRefreshCredential(ctx, credentialID)
	return err
}

func (m *Manager) forceRefreshCredential(ctx context.Context, credentialID string) (db.GeminiCredential, error) {
	entry := m.entry(credentialID)
	if !entry.refreshing.CompareAndSwap(false, true) {
		return m.waitForCachedCredential(ctx, credentialID)
	}
	defer entry.refreshing.Store(false)
	m.invalidateCache(credentialID)

	row, err := m.store.GetGeminiCLI(ctx, credentialID)
	if err != nil {
		return db.GeminiCredential{}, err
	}

	row.AccessToken = ""
	row.Expired = time.Time{}
	return m.refreshAndWriteBack(ctx, row)
}

func (m *Manager) refreshAndWriteBack(ctx context.Context, row db.GeminiCredential) (db.GeminiCredential, error) {
	accessToken := strings.TrimSpace(row.AccessToken)
	refreshToken := strings.TrimSpace(row.RefreshToken)
	expiry := row.Expired
	projectID := strings.TrimSpace(row.ProjectID)
	if projectID == "" {
		projectID = CredentialProjectID(row.ID)
	}
	if strings.TrimSpace(accessToken) == "" || m.shouldRefresh(expiry) {
		if refreshToken == "" {
			return db.GeminiCredential{}, fmt.Errorf("gemini credential %s has no refresh token", row.ID)
		}

		tokenData, err := m.client.RefreshAccessToken(ctx, refreshToken)
		if err != nil {
			return db.GeminiCredential{}, err
		}
		if tokenData == nil {
			return db.GeminiCredential{}, fmt.Errorf("refresh tokens for %s: empty token response", row.ID)
		}
		accessToken = strings.TrimSpace(tokenData.AccessToken)
		refreshToken = strings.TrimSpace(tokenData.RefreshToken)
		expiry = tokenData.Expiry
		if accessToken == "" {
			return db.GeminiCredential{}, fmt.Errorf("refresh tokens for %s: empty access token", row.ID)
		}
		if refreshToken == "" {
			return db.GeminiCredential{}, fmt.Errorf("refresh tokens for %s: empty refresh token", row.ID)
		}
		if expiry.IsZero() {
			return db.GeminiCredential{}, fmt.Errorf("refresh tokens for %s: zero expiry", row.ID)
		}
	}

	if projectID == "" && accessToken != "" {
		resolvedProjectID, err := m.client.ResolveProjectID(ctx, accessToken)
		if err == nil && strings.TrimSpace(resolvedProjectID) != "" {
			projectID = strings.TrimSpace(resolvedProjectID)
		}
	}

	updated, err := m.store.UpdateGeminiTokens(ctx, db.UpdateGeminiTokensParams{
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
		return db.GeminiCredential{}, err
	}
	m.writeCache(updated)
	return updated, nil
}

func (m *Manager) waitForCredential(ctx context.Context, entry *GeminiEntry, credentialID string) (db.GeminiCredential, error) {
	ticker := time.NewTicker(m.settingsSnapshot().PollInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return db.GeminiCredential{}, fmt.Errorf("waiting for gemini token refresh: %w", ctx.Err())
		case <-ticker.C:
			if snapshot, ok := m.readCache(credentialID); ok {
				return m.credentialFromCache(snapshot), nil
			}

			row, err := m.store.GetGeminiCLI(ctx, credentialID)
			if err != nil {
				return db.GeminiCredential{}, err
			}
			if strings.TrimSpace(row.ProjectID) == "" {
				row.ProjectID = CredentialProjectID(row.ID)
			}
			if strings.TrimSpace(row.AccessToken) != "" && !m.shouldRefresh(row.Expired) && strings.TrimSpace(row.ProjectID) != "" {
				m.writeCache(row)
				if snapshot, ok := m.readCache(credentialID); ok {
					return m.credentialFromCache(snapshot), nil
				}
				return row, nil
			}
			if entry.refreshing.CompareAndSwap(false, true) {
				refreshed, err := m.refreshAndWriteBack(ctx, row)
				entry.refreshing.Store(false)
				return refreshed, err
			}
		}
	}
}

func (m *Manager) waitForCachedCredential(ctx context.Context, credentialID string) (db.GeminiCredential, error) {
	ticker := time.NewTicker(m.settingsSnapshot().PollInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return db.GeminiCredential{}, fmt.Errorf("waiting for forced gemini token refresh: %w", ctx.Err())
		case <-ticker.C:
			if snapshot, ok := m.readCache(credentialID); ok {
				return m.credentialFromCache(snapshot), nil
			}
		}
	}
}

func (m *Manager) readCache(id string) (GeminiCache, bool) {
	if m == nil || m.cache == nil {
		return GeminiCache{}, false
	}
	return m.cache.GetIfPresent(id)
}

func (m *Manager) writeCache(row db.GeminiCredential) {
	projectID := strings.TrimSpace(row.ProjectID)
	if projectID == "" {
		projectID = CredentialProjectID(row.ID)
	}
	if m == nil || m.cache == nil || strings.TrimSpace(row.AccessToken) == "" || projectID == "" {
		return
	}
	ttl := time.Until(row.Expired.Add(-m.settingsSnapshot().RefreshBefore()))
	if ttl <= 0 {
		return
	}
	snapshot := GeminiCache{
		ID:          row.ID,
		AccessToken: strings.TrimSpace(row.AccessToken),
		ProjectID:   projectID,
		Expired:     row.Expired,
	}
	m.cache.Set(row.ID, snapshot)
	m.cache.SetExpiresAfter(row.ID, ttl)
}

func (m *Manager) credentialFromCache(snapshot GeminiCache) db.GeminiCredential {
	projectID := strings.TrimSpace(snapshot.ProjectID)
	if projectID == "" {
		projectID = CredentialProjectID(snapshot.ID)
	}
	return db.GeminiCredential{
		ID:          snapshot.ID,
		AccessToken: snapshot.AccessToken,
		ProjectID:   projectID,
		Expired:     snapshot.Expired,
	}
}

func (m *Manager) entry(id string) *GeminiEntry {
	if actual, ok := m.entries.Load(id); ok {
		return actual.(*GeminiEntry)
	}
	created := &GeminiEntry{}
	actual, _ := m.entries.LoadOrStore(id, created)
	return actual.(*GeminiEntry)
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

func (m *Manager) proactiveRefresh(id string) {
	entry := m.entry(id)
	if !entry.refreshing.CompareAndSwap(false, true) {
		return
	}
	defer entry.refreshing.Store(false)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	row, err := m.store.GetGeminiCLI(ctx, id)
	if err != nil {
		log.Error().Err(err).Str("credential", id).Msg("gemini proactive-refresh: get credential")
		return
	}
	if _, err := m.refreshAndWriteBack(ctx, row); err != nil {
		log.Error().Err(err).Str("credential", id).Msg("gemini proactive-refresh: failed")
	}
}

func (m *Manager) shouldRefresh(expiry time.Time) bool {
	if expiry.IsZero() {
		return true
	}
	return time.Now().Add(m.settingsSnapshot().RefreshBefore()).After(expiry)
}

func (m *Manager) settingsSnapshot() settings.Snapshot {
	if m == nil || m.settings == nil {
		return settings.DefaultSnapshot()
	}
	return m.settings.Snapshot()
}
