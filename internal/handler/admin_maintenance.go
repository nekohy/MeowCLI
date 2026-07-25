package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/nekohy/MeowCLI/utils"
)

type clearLogsResponse struct {
	Deleted   int      `json:"deleted"`
	Refreshed bool     `json:"refreshed"`
	Errors    []string `json:"errors,omitempty"`
}

// RefreshQuota queues an immediate quota refresh for every credential of a provider.
func (a *AdminHandler) RefreshQuota(c *gin.Context) {
	provider, ok := parseProvider(c)
	if !ok {
		return
	}
	if a == nil || a.credRefresh == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "credential refresher is unavailable"})
		return
	}

	ids, err := a.credentialIDs(c.Request.Context(), provider)
	if err != nil {
		writeInternalError(c, err)
		return
	}

	a.credRefresh.SyncQuotas(context.Background(), provider, ids)
	c.JSON(http.StatusOK, gin.H{"queued": len(ids)})
}

// ClearLogs removes retained request logs for one provider or all providers.
func (a *AdminHandler) ClearLogs(c *gin.Context) {
	if a == nil || a.logStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "log store is unavailable"})
		return
	}

	handler := strings.TrimSpace(c.Param("provider"))
	var providers []utils.HandlerType
	if handler == "all" {
		providers = []utils.HandlerType{utils.HandlerCodex, utils.HandlerGemini, utils.HandlerAntigravity, utils.HandlerOpenCodeGo}
	} else {
		provider, ok := parseProvider(c)
		if !ok {
			return
		}
		handler = string(provider)
		providers = []utils.HandlerType{provider}
	}

	deleted, err := a.logStore.ClearLogs(c.Request.Context(), handler)
	if err != nil {
		writeInternalError(c, err)
		return
	}

	result := clearLogsResponse{Deleted: deleted}
	if a.credRefresh == nil {
		result.Errors = []string{"credential refresher is unavailable"}
		c.JSON(http.StatusOK, result)
		return
	}
	for _, provider := range providers {
		if err := a.credRefresh.RefreshAvailable(c.Request.Context(), provider); err != nil {
			result.Errors = append(result.Errors, string(provider)+": "+err.Error())
		}
	}
	result.Refreshed = len(result.Errors) == 0
	c.JSON(http.StatusOK, result)
}

func parseProvider(c *gin.Context) (utils.HandlerType, bool) {
	provider, ok := utils.ParseHandlerType(strings.TrimSpace(c.Param("provider")))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported provider"})
		return "", false
	}
	return provider, true
}

func (a *AdminHandler) credentialIDs(ctx context.Context, provider utils.HandlerType) ([]string, error) {
	ids := make([]string, 0)
	add := func(id string) {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}

	switch provider {
	case utils.HandlerCodex:
		rows, err := a.store.ListCodex(ctx)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			add(row.ID)
		}
	case utils.HandlerGemini:
		rows, err := a.store.ListGeminiCLI(ctx)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			add(row.ID)
		}
	case utils.HandlerAntigravity:
		rows, err := a.store.ListAntigravity(ctx)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			add(row.ID)
		}
	case utils.HandlerOpenCodeGo:
		rows, err := a.store.ListOpenCodeGo(ctx)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			add(row.ID)
		}
	}
	return ids, nil
}
