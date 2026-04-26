package handler

import (
	"context"

	db "github.com/nekohy/MeowCLI/internal/store"
)

type LogRow = db.LogRow
type LogFilterParams = db.LogFilterParams
type ListLogsParams = db.ListLogsParams

type LogStore interface {
	QueryLogs(ctx context.Context, params db.ListLogsParams) (db.LogQueryResult, error)
	ErrorRatesForCredentials(ctx context.Context, handler string, modelTier string, since []db.ErrorRateSince, minSamples int) (map[string]float64, error)
}

func (a *AdminHandler) queryLogs(ctx context.Context, params ListLogsParams) (db.LogQueryResult, error) {
	emptyStats := db.LogStats{StatusCodes: []db.LogStatusCount{}}
	if a == nil || a.logStore == nil {
		return db.LogQueryResult{
			Rows:          []LogRow{},
			FilteredStats: emptyStats,
			TotalStats:    emptyStats,
			StatusStats:   emptyStats,
		}, nil
	}
	result, err := a.logStore.QueryLogs(ctx, params)
	if err != nil {
		return db.LogQueryResult{}, err
	}
	if result.Rows == nil {
		result.Rows = []LogRow{}
	}
	if result.FilteredStats.StatusCodes == nil {
		result.FilteredStats.StatusCodes = []db.LogStatusCount{}
	}
	if result.TotalStats.StatusCodes == nil {
		result.TotalStats.StatusCodes = []db.LogStatusCount{}
	}
	if result.StatusStats.StatusCodes == nil {
		result.StatusStats.StatusCodes = []db.LogStatusCount{}
	}
	return result, nil
}
