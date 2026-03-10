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

	s.pruneLocked(time.Now())
	s.rows = append(s.rows, db.LogRow{
		Handler:      arg.Handler,
		CredentialID: arg.CredentialID,
		StatusCode:   arg.StatusCode,
		Text:         arg.Text,
		CreatedAt:    time.Now(),
	})
	return nil
}

func (s *Store) ListLogs(ctx context.Context, arg db.ListLogsParams) ([]db.LogRow, error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	if s == nil || arg.Limit <= 0 {
		return []db.LogRow{}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.pruneLocked(time.Now())

	if len(s.rows) == 0 || arg.Offset < 0 {
		return []db.LogRow{}, nil
	}

	start := len(s.rows) - 1 - int(arg.Offset)
	if start < 0 {
		return []db.LogRow{}, nil
	}

	limit := int(arg.Limit)
	result := make([]db.LogRow, 0, min(limit, start+1))
	for i := start; i >= 0 && len(result) < limit; i-- {
		result = append(result, s.rows[i])
	}
	return result, nil
}

func (s *Store) CountLogs(ctx context.Context) (int64, error) {
	if err := contextErr(ctx); err != nil {
		return 0, err
	}
	if s == nil {
		return 0, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.pruneLocked(time.Now())
	return int64(len(s.rows)), nil
}

func (s *Store) pruneLocked(now time.Time) {
	if len(s.rows) == 0 {
		return
	}

	retention := s.retention()
	if retention <= 0 {
		s.rows = s.rows[:0]
		return
	}

	cutoff := now.Add(-retention)
	drop := 0
	for drop < len(s.rows) && !s.rows[drop].CreatedAt.After(cutoff) {
		drop++
	}
	if drop == 0 {
		return
	}

	copy(s.rows, s.rows[drop:])
	for i := len(s.rows) - drop; i < len(s.rows); i++ {
		s.rows[i] = db.LogRow{}
	}
	s.rows = s.rows[:len(s.rows)-drop]
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
