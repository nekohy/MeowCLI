package gemini

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	geminiapi "github.com/nekohy/MeowCLI/api/gemini"
	"github.com/nekohy/MeowCLI/internal/settings"
	db "github.com/nekohy/MeowCLI/internal/store"
)

type Store interface {
	GetGeminiCLI(ctx context.Context, id string) (db.GeminiCredential, error)
	UpdateGeminiTokens(ctx context.Context, arg db.UpdateGeminiTokensParams) (db.GeminiCredential, error)
}

type Manager struct {
	store    Store
	client   *geminiapi.Client
	settings settings.Provider
}

type ManagerConfig struct {
	Store    Store
	Client   *geminiapi.Client
	Settings settings.Provider
}

func NewManager(cfg ManagerConfig) (*Manager, error) {
	if cfg.Store == nil {
		return nil, fmt.Errorf("gemini manager store is required")
	}
	if cfg.Client == nil {
		return nil, fmt.Errorf("gemini manager client is required")
	}
	return &Manager{
		store:    cfg.Store,
		client:   cfg.Client,
		settings: cfg.Settings,
	}, nil
}

func (m *Manager) GetAccessToken(ctx context.Context, credentialID string) (string, error) {
	row, err := m.ensureCredential(ctx, credentialID)
	if err != nil {
		return "", err
	}
	return row.AccessToken, nil
}

func (m *Manager) GetAuthHeaders(ctx context.Context, credentialID string) (http.Header, error) {
	row, err := m.ensureCredential(ctx, credentialID)
	if err != nil {
		return nil, err
	}

	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+row.AccessToken)
	headers.Set("X-Meow-Gemini-Project", strings.TrimSpace(row.ProjectID))
	return headers, nil
}

func (m *Manager) ensureCredential(ctx context.Context, credentialID string) (db.GeminiCredential, error) {
	row, err := m.store.GetGeminiCLI(ctx, credentialID)
	if err != nil {
		return db.GeminiCredential{}, err
	}
	needsRefresh := strings.TrimSpace(row.AccessToken) == "" || m.shouldRefresh(row.Expired)
	if !needsRefresh && strings.TrimSpace(row.ProjectID) != "" {
		return row, nil
	}

	accessToken := strings.TrimSpace(row.AccessToken)
	refreshToken := strings.TrimSpace(row.RefreshToken)
	expiry := row.Expired
	projectID := strings.TrimSpace(row.ProjectID)
	if needsRefresh {
		if refreshToken == "" {
			return db.GeminiCredential{}, fmt.Errorf("gemini credential %s has no refresh token", credentialID)
		}

		tokenData, err := m.client.RefreshAccessToken(ctx, refreshToken)
		if err != nil {
			return db.GeminiCredential{}, err
		}
		accessToken = tokenData.AccessToken
		refreshToken = tokenData.RefreshToken
		expiry = tokenData.Expiry
	}

	if projectID == "" && accessToken != "" {
		resolvedProjectID, err := m.client.ResolveProjectID(ctx, accessToken)
		if err == nil {
			projectID = resolvedProjectID
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
	return updated, nil
}

func (m *Manager) shouldRefresh(expiry time.Time) bool {
	if expiry.IsZero() {
		return true
	}
	refreshBefore := 30 * time.Second
	if m != nil && m.settings != nil {
		refreshBefore = m.settings.Snapshot().RefreshBefore()
	}
	return time.Now().Add(refreshBefore).After(expiry)
}
