package logs

import (
	"context"
	"sync"
	"time"

	"github.com/nekohy/MeowCLI/internal/settings"
	db "github.com/nekohy/MeowCLI/internal/store"
)

type Store struct {
	settings settings.Provider

	mu   sync.RWMutex
	rows []db.LogRow
	head int
}

var _ db.LogStore = (*Store)(nil)

func NewStore(provider settings.Provider) *Store {
	return &Store{settings: provider}
}

func (s *Store) SetSettingsProvider(provider settings.Provider) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.settings = provider
	s.mu.Unlock()
}

func (s *Store) InsertLog(ctx context.Context, arg db.InsertLogParams) error {
	if err := contextErr(ctx); err != nil || s == nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.pruneLocked(now)
	s.rows = append(s.rows, db.LogRow{
		Handler:      arg.Handler,
		CredentialID: arg.CredentialID,
		StatusCode:   arg.StatusCode,
		Text:         arg.Text,
		ModelTier:    arg.ModelTier,
		CreatedAt:    now,
	})
	s.compactLocked()
	return nil
}

func (s *Store) ListLogs(ctx context.Context, arg db.ListLogsParams) ([]db.LogRow, error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	if s == nil || arg.Limit <= 0 {
		return []db.LogRow{}, nil
	}

	now := time.Now()
	if s.needsPrune(now) {
		s.mu.Lock()
		s.pruneLocked(now)
		rows := s.listLocked(arg)
		s.compactLocked()
		s.mu.Unlock()
		return rows, nil
	}

	s.mu.RLock()
	rows := s.listLocked(arg)
	s.mu.RUnlock()

	return rows, nil
}

func (s *Store) CountLogs(ctx context.Context) (int64, error) {
	if err := contextErr(ctx); err != nil {
		return 0, err
	}
	if s == nil {
		return 0, nil
	}

	now := time.Now()
	if s.needsPrune(now) {
		s.mu.Lock()
		s.pruneLocked(now)
		count := int64(len(s.rows) - s.head)
		s.compactLocked()
		s.mu.Unlock()
		return count, nil
	}

	s.mu.RLock()
	count := int64(len(s.rows) - s.head)
	s.mu.RUnlock()
	return count, nil
}

func (s *Store) ErrorRatesForCredentials(ctx context.Context, handler string, modelTier string, since []db.ErrorRateSince, minSamples int) (map[string]float64, error) {
	if err := contextErr(ctx); err != nil || s == nil {
		return nil, err
	}

	if minSamples <= 0 {
		minSamples = 1
	}

	sinceByID := make(map[string]time.Time, len(since))
	var oldestSince time.Time
	for _, item := range since {
		if item.CredentialID == "" {
			continue
		}
		sinceByID[item.CredentialID] = item.Since
		if item.Since.IsZero() {
			oldestSince = time.Time{}
			continue
		}
		if oldestSince.IsZero() || item.Since.Before(oldestSince) {
			oldestSince = item.Since
		}
	}
	if len(sinceByID) == 0 {
		return map[string]float64{}, nil
	}

	s.mu.RLock()
	type counters struct{ total, errors int }
	counts := make(map[string]*counters, len(sinceByID))
	for i := len(s.rows) - 1; i >= s.head; i-- {
		row := &s.rows[i]
		if !oldestSince.IsZero() && !row.CreatedAt.After(oldestSince) {
			break
		}
		if row.Handler != handler {
			continue
		}
		// When modelTier is specified, filter rows to that tier only.
		// Special case: "default" also matches rows where ModelTier is
		// empty — Codex records non-spark requests with ModelTier="".
		if modelTier != "" {
			if row.ModelTier != modelTier {
				if modelTier != "default" || row.ModelTier != "" {
					continue
				}
			}
		}
		sinceAt, ok := sinceByID[row.CredentialID]
		if !ok {
			continue
		}
		if !sinceAt.IsZero() && !row.CreatedAt.After(sinceAt) {
			continue
		}
		c := counts[row.CredentialID]
		if c == nil {
			c = &counters{}
			counts[row.CredentialID] = c
		}
		c.total++
		if isLogError(row.StatusCode) {
			c.errors++
		}
	}
	s.mu.RUnlock()

	result := make(map[string]float64, len(counts))
	for id, c := range counts {
		if c.total >= minSamples {
			result[id] = float64(c.errors) / float64(c.total)
		}
	}
	return result, nil
}

func isLogError(statusCode int32) bool {
	return statusCode >= 400 || statusCode == 0
}

func (s *Store) pruneLocked(now time.Time) {
	if s == nil || s.head >= len(s.rows) {
		s.rows = s.rows[:0]
		s.head = 0
		return
	}

	retention := s.retention()
	if retention <= 0 {
		s.rows = s.rows[:0]
		s.head = 0
		return
	}

	cutoff := now.Add(-retention)
	for s.head < len(s.rows) && !s.rows[s.head].CreatedAt.After(cutoff) {
		s.head++
	}
}

func (s *Store) compactLocked() {
	if s == nil || s.head == 0 {
		return
	}
	if s.head >= len(s.rows) {
		s.rows = s.rows[:0]
		s.head = 0
		return
	}
	if s.head < 1024 && s.head*2 < len(s.rows) {
		return
	}

	visible := len(s.rows) - s.head
	compacted := make([]db.LogRow, visible)
	copy(compacted, s.rows[s.head:])
	s.rows = compacted
	s.head = 0
}

func (s *Store) listLocked(arg db.ListLogsParams) []db.LogRow {
	visible := len(s.rows) - s.head
	if visible <= 0 || arg.Offset < 0 {
		return []db.LogRow{}
	}

	start := len(s.rows) - 1 - int(arg.Offset)
	if start < s.head {
		return []db.LogRow{}
	}

	limit := int(arg.Limit)
	result := make([]db.LogRow, 0, min(limit, start-s.head+1))
	for i := start; i >= s.head && len(result) < limit; i-- {
		result = append(result, s.rows[i])
	}
	return result
}

func (s *Store) needsPrune(now time.Time) bool {
	if s == nil {
		return false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.head >= len(s.rows) {
		return false
	}

	retention := s.retention()
	if retention <= 0 {
		return len(s.rows) > s.head
	}

	cutoff := now.Add(-retention)
	return !s.rows[s.head].CreatedAt.After(cutoff)
}

func (s *Store) retention() time.Duration {
	if s == nil || s.settings == nil {
		return settings.DefaultSnapshot().LogsRetention()
	}
	return s.settings.Snapshot().LogsRetention()
}

func contextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
