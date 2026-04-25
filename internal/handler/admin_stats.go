package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Stats 概览统计
func (a *AdminHandler) Stats(c *gin.Context) {
	ctx := c.Request.Context()

	credCount, err := a.store.CountEnabledCodex(ctx)
	if err != nil {
		writeInternalError(c, err)
		return
	}
	logStats, err := a.countLogs(ctx, LogFilterParams{})
	if err != nil {
		writeInternalError(c, err)
		return
	}
	modelsTotal, err := a.store.CountModels(ctx)
	if err != nil {
		writeInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"credentials_enabled": credCount,
		"logs_total":          logStats.Total,
		"models_total":        modelsTotal,
	})
}
