package handler

import (
	"context"

	db "github.com/nekohy/MeowCLI/internal/store"
)

type LogRow = db.LogRow
type LogFilterParams = db.LogFilterParams
type ListLogsParams = db.ListLogsParams

type LogStore interface {
	CountLogs(ctx context.Context, filter db.LogFilterParams) (db.LogStats, error)
	ListLogs(ctx context.Context, params db.ListLogsParams) ([]db.LogRow, error)
	ErrorRatesForCredentials(ctx context.Context, handler string, modelTier string, since []db.ErrorRateSince, minSamples int) (map[string]float64, error)
}

func (a *AdminHandler) countLogs(ctx context.Context, filter LogFilterParams) (db.LogStats, error) {
	if a == nil || a.logStore == nil {
		return db.LogStats{StatusCodes: []db.LogStatusCount{}}, nil
	}
	return a.logStore.CountLogs(ctx, filter)
}

func (a *AdminHandler) listLogs(ctx context.Context, params ListLogsParams) ([]LogRow, error) {
	if a == nil || a.logStore == nil {
		return []LogRow{}, nil
	}
	rows, err := a.logStore.ListLogs(ctx, params)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		return []LogRow{}, nil
	}
	return rows, nil
}
