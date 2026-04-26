package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func (a *AdminHandler) QueryLogs(c *gin.Context) {
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

	params := ListLogsParams{
		Limit:           int32(pageSize),
		Offset:          offset,
		LogFilterParams: filter,
	}
	result, err := a.queryLogs(c.Request.Context(), params)
	if err != nil {
		writeInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"total":     result.FilteredStats.Total,
		"page":      page,
		"page_size": pageSize,
		"data":      result.Rows,
		"summary": gin.H{
			"total":        result.TotalStats.Total,
			"status_codes": result.StatusStats.StatusCodes,
		},
	})
}
