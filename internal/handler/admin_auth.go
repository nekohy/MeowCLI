package handler

import (
	"net/http"
	"strings"

	"github.com/nekohy/MeowCLI/internal/auth"
	db "github.com/nekohy/MeowCLI/internal/store"

	"github.com/gin-gonic/gin"
)

const contextKeyAuth = "authKey"

func extractBearerKey(c *gin.Context) string {
	header := strings.TrimSpace(c.GetHeader("Authorization"))
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

// APIAuthMiddleware 校验 /v1/* 请求的 Bearer key，admin 和 user 都可通过
func APIAuthMiddleware(cache *auth.KeyCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		if cache == nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "auth cache is unavailable"})
			return
		}
		key := extractBearerKey(c)
		if key == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing api key"})
			return
		}
		ak, ok := cache.Lookup(key)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
			return
		}
		c.Set(contextKeyAuth, ak)
		c.Next()
	}
}

// AdminAuthMiddleware 校验 /admin/api/* 请求的 Bearer key，仅 admin 可通过
func AdminAuthMiddleware(cache *auth.KeyCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		if cache == nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "auth cache is unavailable"})
			return
		}
		key := extractBearerKey(c)
		if key == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing api key"})
			return
		}
		ak, ok := cache.Lookup(key)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
			return
		}
		if ak.Role != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin access required"})
			return
		}
		c.Set(contextKeyAuth, ak)
		c.Next()
	}
}

// GetAuthKey 从 gin.Context 中获取已认证的 AuthKey
func GetAuthKey(c *gin.Context) (db.AuthKey, bool) {
	v, exists := c.Get(contextKeyAuth)
	if !exists {
		return db.AuthKey{}, false
	}
	ak, ok := v.(db.AuthKey)
	return ak, ok
}
