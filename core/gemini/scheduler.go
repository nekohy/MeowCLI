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
	"github.com/nekohy/MeowCLI/internal/settings"
	db "github.com/nekohy/MeowCLI/internal/store"
	"github.com/nekohy/MeowCLI/utils"
	"github.com/rs/zerolog/log"
)

var ErrNoAvailableCredential = fmt.Errorf("no available gemini credential")

type SchedulerStore interface {
	ListAvailableGeminiCLI(ctx context.Context) ([]db.ListAvailableGeminiCLIRow, error)
	SetGeminiCLIThrottled(ctx context.Context, credentialID string, throttledUntil time.Time) error
	UpdateGeminiCLIStatus(ctx context.Context, id string, status string, reason string) (db.GeminiCredential, error)
}

type Scheduler struct {
	store    SchedulerStore
	logStore db.LogStore
	manager  *Manager
	settings settings.Provider

	mu       sync.Mutex
	throttle map[string]*throttleState
	rr       atomic.Uint64
}

type throttleState struct {
	consecutive int
	lastFail    time.Time
	until       time.Time
}

func NewScheduler(store SchedulerStore, manager *Manager) *Scheduler {
	return &Scheduler{
		store:    store,
		manager:  manager,
		throttle: make(map[string]*throttleState),
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

func (s *Scheduler) RefreshAvailable(ctx context.Context) ([]db.ListAvailableGeminiCLIRow, error) {
	return s.store.ListAvailableGeminiCLI(ctx)
}

func (s *Scheduler) Pick(ctx context.Context, headers http.Header, preferredCredentialID string, allowedPlanTypes []string) (string, error) {
	rows, err := s.store.ListAvailableGeminiCLI(ctx)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", ErrNoAvailableCredential
	}

	preferredCredentialID = strings.TrimSpace(preferredCredentialID)
	if preferredCredentialID != "" {
		for _, row := range rows {
			if row.ID == preferredCredentialID && geminiPlanAllowed(row.PlanType, allowedPlanTypes) {
				return row.ID, nil
			}
		}
	}

	preferredPlanTypes := ResolvePreferredPlanTypes(headers, allowedPlanTypes)
	for _, planType := range preferredPlanTypes {
		if credentialID, ok := s.pickRoundRobin(rows, planType); ok {
			return credentialID, nil
		}
	}

	for _, row := range rows {
		if geminiPlanAllowed(row.PlanType, allowedPlanTypes) {
			return row.ID, nil
		}
	}
	return "", ErrNoAvailableCredential
}

func (s *Scheduler) pickRoundRobin(rows []db.ListAvailableGeminiCLIRow, planType string) (string, bool) {
	if len(rows) == 0 {
		return "", false
	}

	start := int(s.rr.Add(1)-1) % len(rows)
	for offset := range rows {
		idx := (start + offset) % len(rows)
		row := rows[idx]
		if row.ID == "" {
			continue
		}
		if planType != "" && NormalizePlanType(row.PlanType) != planType {
			continue
		}
		return row.ID, true
	}
	return "", false
}

func geminiPlanAllowed(planType string, allowedPlanTypes []string) bool {
	if len(allowedPlanTypes) == 0 {
		return true
	}
	normalized := NormalizePlanType(planType)
	for _, allowed := range allowedPlanTypes {
		if NormalizePlanType(allowed) == normalized {
			return true
		}
	}
	return false
}

func (s *Scheduler) AuthHeaders(ctx context.Context, credentialID string) (http.Header, error) {
	headers, err := s.manager.GetAuthHeaders(ctx, credentialID)
	if err != nil {
		return nil, err
	}
	return headers, nil
}

func (s *Scheduler) RecordSuccess(_ context.Context, credentialID string, statusCode int32) {
	s.mu.Lock()
	delete(s.throttle, credentialID)
	s.mu.Unlock()

	if err := s.insertLog(context.Background(), db.InsertLogParams{
		Handler:      string(utils.HandlerGemini),
		CredentialID: credentialID,
		StatusCode:   statusCode,
		Text:         "",
	}); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("gemini scheduler: insert success log")
	}
}

func (s *Scheduler) RecordFailure(_ context.Context, credentialID string, statusCode int32, text string, retryAfter time.Duration) {
	bgCtx := context.Background()

	if err := s.insertLog(bgCtx, db.InsertLogParams{
		Handler:      string(utils.HandlerGemini),
		CredentialID: credentialID,
		StatusCode:   statusCode,
		Text:         text,
	}); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("gemini scheduler: insert failure log")
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

	if retryAfter <= 0 {
		retryAfter = utils.CalcBackoff(consecutive, s.settingsSnapshot().ThrottleBase(), s.settingsSnapshot().ThrottleMax())
	}

	throttledUntil := now.Add(retryAfter)
	if err := s.store.SetGeminiCLIThrottled(bgCtx, credentialID, throttledUntil); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("gemini scheduler: set throttled")
	}

	s.mu.Lock()
	if state, ok := s.throttle[credentialID]; ok {
		state.until = throttledUntil
	}
	s.mu.Unlock()
}

func (s *Scheduler) HandleUnauthorized(ctx context.Context, credentialID string, statusCode int32, text string) bool {
	if statusCode != http.StatusUnauthorized && statusCode != http.StatusForbidden {
		return false
	}

	if err := s.insertLog(ctx, db.InsertLogParams{
		Handler:      string(utils.HandlerGemini),
		CredentialID: credentialID,
		StatusCode:   statusCode,
		Text:         text,
	}); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("gemini scheduler: insert invalidation log")
	}

	if _, err := s.store.UpdateGeminiCLIStatus(context.Background(), credentialID, "disabled", "unauthorized"); err != nil {
		log.Error().Err(err).Str("credential", credentialID).Msg("gemini scheduler: disable unauthorized credential")
	}

	s.mu.Lock()
	delete(s.throttle, credentialID)
	s.mu.Unlock()
	return true
}

func (s *Scheduler) RetryDelay(statusCode int32, text string, headers http.Header) time.Duration {
	if statusCode != http.StatusTooManyRequests {
		return 0
	}
	if retryAfter := geminiapi.ParseRetryAfterHeader(headers); retryAfter > 0 {
		return retryAfter
	}
	return geminiapi.ParseRetryDelayText(text)
}

func (s *Scheduler) GraceRetry(statusCode int32, _ string, retryAfter time.Duration) (time.Duration, bool) {
	if statusCode != http.StatusTooManyRequests || retryAfter <= 0 {
		return 0, false
	}
	if retryAfter > 2*time.Second {
		return 0, false
	}
	return retryAfter + 100*time.Millisecond, true
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
