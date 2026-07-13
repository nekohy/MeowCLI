package opencodego

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/nekohy/MeowCLI/core/scheduling"
	db "github.com/nekohy/MeowCLI/internal/store"
	"github.com/nekohy/MeowCLI/utils"
)

var (
	ErrCredentialDisabled = errors.New("opencode go credential is not active")
	ErrAPIKeyMissing      = errors.New("opencode go api key is empty")
)

type ManagerStore interface {
	GetOpenCodeGo(ctx context.Context, id string) (db.OpenCodeGoCredential, error)
}

type cachedCredential struct {
	APIKey string
}

type Manager struct {
	store ManagerStore
	cache sync.Map
}

func NewManager(store ManagerStore) (*Manager, error) {
	if store == nil {
		return nil, errors.New("opencode go store is required")
	}
	return &Manager{store: store}, nil
}

func (m *Manager) AccessToken(ctx context.Context, credentialID string, _ scheduling.RefreshMode) (string, error) {
	if cached, ok := m.cache.Load(credentialID); ok {
		return cached.(cachedCredential).APIKey, nil
	}
	row, err := m.store.GetOpenCodeGo(ctx, credentialID)
	if err != nil {
		return "", err
	}
	status, err := utils.ParseAccountStatus(row.Status)
	if err != nil || status != utils.StatusEnabled {
		return "", ErrCredentialDisabled
	}
	apiKey := strings.TrimSpace(row.APIKey)
	if apiKey == "" {
		return "", ErrAPIKeyMissing
	}
	m.cache.Store(credentialID, cachedCredential{APIKey: apiKey})
	return apiKey, nil
}

func (m *Manager) AuthHeaders(ctx context.Context, credentialID string, mode scheduling.RefreshMode) (http.Header, error) {
	apiKey, err := m.AccessToken(ctx, credentialID, mode)
	if err != nil {
		return nil, err
	}
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+apiKey)
	return headers, nil
}

func (m *Manager) RefreshCredential(ctx context.Context, credentialID string) error {
	m.InvalidateCredential(credentialID)
	_, err := m.AccessToken(ctx, credentialID, scheduling.UseCached)
	return err
}

func (m *Manager) InvalidateCredential(credentialID string) {
	if m == nil || credentialID == "" {
		return
	}
	m.cache.Delete(credentialID)
}
