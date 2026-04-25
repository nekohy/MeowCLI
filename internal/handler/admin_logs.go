package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func (a *AdminHandler) ListLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}

	offset := int32((page - 1) * pageSize)
	filter := LogFilterParams{
		Search:  strings.TrimSpace(c.Query("search")),
		Handler: strings.TrimSpace(c.Query("handler")),
	}
	if rawStatusCode := strings.TrimSpace(c.Query("status_code")); rawStatusCode != "" {
		statusCode, err := strconv.Atoi(rawStatusCode)
		if err == nil {
			filter.StatusCode = int32(statusCode)
			filter.HasStatusCode = true
		}
	}

	totalAll, err := a.countLogs(c.Request.Context(), LogFilterParams{})
	if err != nil {
		writeInternalError(c, err)
		return
	}

	filteredStats, err := a.countLogs(c.Request.Context(), filter)
	if err != nil {
		writeInternalError(c, err)
		return
	}

	statusStats := filteredStats
	if filter.HasStatusCode {
		statusFilter := filter
		statusFilter.HasStatusCode = false
		statusStats, err = a.countLogs(c.Request.Context(), statusFilter)
		if err != nil {
			writeInternalError(c, err)
			return
		}
	}

	rows, err := a.listLogs(c.Request.Context(), ListLogsParams{
		Limit:           int32(pageSize),
		Offset:          offset,
		LogFilterParams: filter,
	})
	if err != nil {
		writeInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"total":     filteredStats.Total,
		"page":      page,
		"page_size": pageSize,
		"data":      rows,
		"summary": gin.H{
			"total":        totalAll.Total,
			"status_codes": statusStats.StatusCodes,
		},
	})
}
