package handler

import (
	"context"
	db "github.com/nekohy/MeowCLI/internal/store"
)

type LogRow = db.LogRow
type ListLogsParams = db.ListLogsParams

type LogStore interface {
	CountLogs(ctx context.Context) (int64, error)
	ListLogs(ctx context.Context, params db.ListLogsParams) ([]db.LogRow, error)
}

func (a *AdminHandler) countLogs(ctx context.Context) (int64, error) {
	if a == nil || a.logStore == nil {
		return 0, nil
	}
	return a.logStore.CountLogs(ctx)
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
