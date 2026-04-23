package handler

import (
	"encoding/json"
	db "github.com/nekohy/MeowCLI/internal/store"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type createModelReq struct {
	Alias     string          `json:"alias" binding:"required"`
	Origin    string          `json:"origin" binding:"required"`
	Handler   string          `json:"handler" binding:"required"`
	PlanTypes string          `json:"plan_types"`
	Extra     json.RawMessage `json:"extra"`
}

type updateModelReq struct {
	Origin    string          `json:"origin" binding:"required"`
	Handler   string          `json:"handler" binding:"required"`
	PlanTypes string          `json:"plan_types"`
	Extra     json.RawMessage `json:"extra"`
}

func (a *AdminHandler) ListModels(c *gin.Context) {
	rows, err := a.store.ListModels(c.Request.Context())
	if err != nil {
		writeInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, rows)
}

func (a *AdminHandler) CreateModel(c *gin.Context) {
	var req createModelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	alias, origin, handler, planTypes, extra, err := normalizeModelInput(req.Alias, req.Origin, req.Handler, req.PlanTypes, req.Extra)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	row, err := a.store.CreateModel(c.Request.Context(), db.CreateModelParams{
		Alias:     alias,
		Origin:    origin,
		Handler:   handler,
		PlanTypes: planTypes,
		Extra:     extra,
	})
	if writeStoreError(c, err, "", "model alias already exists") {
		return
	}
	c.JSON(http.StatusCreated, row)
}

func (a *AdminHandler) UpdateModel(c *gin.Context) {
	alias := strings.TrimSpace(c.Param("alias"))
	var req updateModelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	alias, origin, handler, planTypes, extra, err := normalizeModelInput(alias, req.Origin, req.Handler, req.PlanTypes, req.Extra)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	row, err := a.store.UpdateModel(c.Request.Context(), db.UpdateModelParams{
		Alias:     alias,
		Origin:    origin,
		Handler:   handler,
		PlanTypes: planTypes,
		Extra:     extra,
	})
	if writeStoreError(c, err, "model not found", "") {
		return
	}
	c.JSON(http.StatusOK, row)
}

func (a *AdminHandler) DeleteModel(c *gin.Context) {
	alias := strings.TrimSpace(c.Param("alias"))
	if alias == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "alias is required"})
		return
	}
	if err := a.store.DeleteModel(c.Request.Context(), alias); writeStoreError(c, err, "model not found", "") {
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
