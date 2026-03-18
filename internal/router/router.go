package router

import (
	"github.com/nekohy/MeowCLI/internal/auth"
	"github.com/nekohy/MeowCLI/internal/bridge"
	"github.com/nekohy/MeowCLI/internal/handler"
	"github.com/nekohy/MeowCLI/utils"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Deps holds all dependencies needed for route registration.
type Deps struct {
	Bridge    *bridge.Handler
	Admin     *handler.AdminHandler
	AuthCache *auth.KeyCache
}

// Setup registers all routes on the given Gin engine.
func Setup(r *gin.Engine, deps Deps) {
	// /v1 with API auth (admin + user)
	v1 := r.Group("/v1", handler.APIAuthMiddleware(deps.AuthCache))
	v1.POST("/responses", deps.Bridge.Route(utils.APIResponses))
	v1.POST("/responses/compact", deps.Bridge.Route(utils.APIResponsesCompact))

	// Admin
	admin := r.Group("/admin")
	{
		// Admin frontend entry (no auth — login handled client-side)
		admin.GET("", handler.ServeWeb())
		admin.GET("/", handler.ServeWeb())

		// Public API — no auth required
		admin.GET("/api/status", deps.Admin.Status)
		admin.POST("/api/setup", deps.Admin.Setup)

		// Admin API (admin-only auth)
		apiGroup := admin.Group("/api", handler.AdminAuthMiddleware(deps.AuthCache))
		{
			apiGroup.GET("/overview", deps.Admin.Overview)
			apiGroup.GET("/stats", deps.Admin.Stats)
			apiGroup.GET("/settings", deps.Admin.GetSettings)
			apiGroup.PUT("/settings", deps.Admin.UpdateSettings)

			apiGroup.GET("/codex", deps.Admin.ListCodex)
			apiGroup.POST("/codex", deps.Admin.BatchCreateCodex)
			apiGroup.PUT("/codex/status", deps.Admin.BatchUpdateStatus)
			apiGroup.DELETE("/codex", deps.Admin.BatchDeleteCodex)

			apiGroup.GET("/models", deps.Admin.ListModels)
			apiGroup.POST("/models", deps.Admin.CreateModel)
			apiGroup.PUT("/models/:alias", deps.Admin.UpdateModel)
			apiGroup.DELETE("/models/:alias", deps.Admin.DeleteModel)

			apiGroup.GET("/logs", deps.Admin.ListLogs)

			apiGroup.GET("/auth-keys", deps.Admin.ListAuthKeys)
			apiGroup.POST("/auth-keys", deps.Admin.CreateAuthKey)
			apiGroup.PUT("/auth-keys/:key", deps.Admin.UpdateAuthKey)
			apiGroup.DELETE("/auth-keys/:key", deps.Admin.DeleteAuthKey)
		}
	}

	webHandler := handler.ServeWeb()
	r.NoRoute(func(c *gin.Context) {
		if handler.ShouldServeAdminWeb(c.Request.Method, c.Request.URL.Path) {
			webHandler(c)
			return
		}
		c.AbortWithStatus(http.StatusNotFound)
	})
}
