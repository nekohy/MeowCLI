package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	db "github.com/nekohy/MeowCLI/internal/store"

	"github.com/gin-gonic/gin"
)

type createModelReq struct {
	Alias     string          `json:"alias" binding:"required"`
	Origin    string          `json:"origin" binding:"required"`
	Handler   string          `json:"handler" binding:"required"`
	PlanTypes string          `json:"plan_types"`
	Plugin    string          `json:"plugin"`
	Extra     json.RawMessage `json:"extra"`
	db.ModelScheduling
}

type updateModelReq struct {
	Origin    string          `json:"origin" binding:"required"`
	Handler   string          `json:"handler" binding:"required"`
	PlanTypes string          `json:"plan_types"`
	Plugin    string          `json:"plugin"`
	Extra     json.RawMessage `json:"extra"`
	db.ModelScheduling
}

type ModelSchedulingPatch struct {
	ContentAffinity *bool `json:"content_affinity"`
	FillFirst       *bool `json:"fill_first"`
}

type batchUpdateModelsReq struct {
	Aliases   []string        `json:"aliases" binding:"required"`
	Handler   string          `json:"handler" binding:"required"`
	PlanTypes string          `json:"plan_types"`
	Plugin    string          `json:"plugin"`
	Extra     json.RawMessage `json:"extra"`
	ModelSchedulingPatch
}

const conflictingModelSchedulingStrategies = "content_affinity and fill_first are mutually exclusive"

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
	alias, origin, handler, planTypes, plugins, extra, err := normalizeModelInput(req.Alias, req.Origin, req.Handler, req.PlanTypes, req.Plugin, req.Extra, a.pluginRegistry)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !req.ModelScheduling.Valid() {
		c.JSON(http.StatusBadRequest, gin.H{"error": conflictingModelSchedulingStrategies})
		return
	}
	row, err := a.store.CreateModel(c.Request.Context(), db.CreateModelParams{
		Alias:      alias,
		Origin:     origin,
		Handler:    handler,
		PlanTypes:  planTypes,
		Plugin:     plugins,
		Scheduling: req.ModelScheduling,
		Extra:      extra,
	})
	if writeStoreError(c, err, "", "model alias already exists") {
		return
	}
	a.invalidateModel(alias)
	c.JSON(http.StatusCreated, row)
}

func (a *AdminHandler) UpdateModel(c *gin.Context) {
	alias := strings.TrimSpace(c.Param("alias"))
	var req updateModelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	alias, origin, handler, planTypes, plugins, extra, err := normalizeModelInput(alias, req.Origin, req.Handler, req.PlanTypes, req.Plugin, req.Extra, a.pluginRegistry)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !req.ModelScheduling.Valid() {
		c.JSON(http.StatusBadRequest, gin.H{"error": conflictingModelSchedulingStrategies})
		return
	}
	row, err := a.store.UpdateModel(c.Request.Context(), db.UpdateModelParams{
		Alias:      alias,
		Origin:     origin,
		Handler:    handler,
		PlanTypes:  planTypes,
		Plugin:     plugins,
		Scheduling: req.ModelScheduling,
		Extra:      extra,
	})
	if writeStoreError(c, err, "model not found", "") {
		return
	}
	a.invalidateModel(alias)
	c.JSON(http.StatusOK, row)
}

func (a *AdminHandler) BatchUpdateModels(c *gin.Context) {
	var req batchUpdateModelsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	aliases := normalizeBatchModelAliases(req.Aliases)
	if len(aliases) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "aliases are required"})
		return
	}
	if req.ContentAffinity != nil && req.FillFirst != nil && *req.ContentAffinity && *req.FillFirst {
		c.JSON(http.StatusBadRequest, gin.H{"error": conflictingModelSchedulingStrategies})
		return
	}

	_, _, handler, planTypes, plugins, extra, err := normalizeModelInput("__batch__", "__batch__", req.Handler, req.PlanTypes, req.Plugin, req.Extra, a.pluginRegistry)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx := c.Request.Context()
	updated := make([]string, 0, len(aliases))
	errs := make([]batchError, 0)
	for _, alias := range aliases {
		row, err := a.store.ReverseInfoFromModel(ctx, alias)
		if err != nil {
			errs = append(errs, batchError{
				Input: alias,
				Error: storeErrorMessage(err, "model not found", ""),
			})
			continue
		}
		if row.Handler != handler {
			errs = append(errs, batchError{
				Input: alias,
				Error: "model handler mismatch: " + strconvQuote(row.Handler),
			})
			continue
		}
		scheduling := mergeModelScheduling(row.ModelScheduling, req.ModelSchedulingPatch)
		if _, err := a.store.UpdateModel(ctx, db.UpdateModelParams{
			Alias:      alias,
			Origin:     row.Origin,
			Handler:    row.Handler,
			PlanTypes:  planTypes,
			Plugin:     plugins,
			Scheduling: scheduling,
			Extra:      extra,
		}); err != nil {
			errs = append(errs, batchError{
				Input: alias,
				Error: storeErrorMessage(err, "model not found", ""),
			})
			continue
		}
		a.invalidateModel(alias)
		updated = append(updated, alias)
	}

	c.JSON(http.StatusOK, gin.H{
		"updated": updated,
		"errors":  errs,
	})
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
	a.invalidateModel(alias)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (a *AdminHandler) invalidateModel(alias string) {
	if a == nil || a.modelCache == nil {
		return
	}
	a.modelCache.InvalidateModel(alias)
}

func normalizeBatchModelAliases(raw []string) []string {
	seen := map[string]struct{}{}
	aliases := make([]string, 0, len(raw))
	for _, alias := range raw {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		if _, ok := seen[alias]; ok {
			continue
		}
		seen[alias] = struct{}{}
		aliases = append(aliases, alias)
	}
	return aliases
}

func strconvQuote(value string) string {
	bytes, _ := json.Marshal(value)
	return string(bytes)
}

func mergeModelScheduling(scheduling db.ModelScheduling, update ModelSchedulingPatch) db.ModelScheduling {
	if update.ContentAffinity != nil {
		scheduling.ContentAffinity = *update.ContentAffinity
		if *update.ContentAffinity {
			scheduling.FillFirst = false
		}
	}
	if update.FillFirst != nil {
		scheduling.FillFirst = *update.FillFirst
		if *update.FillFirst {
			scheduling.ContentAffinity = false
		}
	}
	return scheduling
}
