package handler

import (
	webui "github.com/nekohy/MeowCLI/web"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const adminWebPrefix = "/admin"

var adminDist = mustAdminDist()

func ShouldServeAdminWeb(method, requestPath string) bool {
	if method != http.MethodGet && method != http.MethodHead {
		return false
	}
	if requestPath == adminWebPrefix || requestPath == adminWebPrefix+"/" {
		return true
	}
	if !strings.HasPrefix(requestPath, adminWebPrefix+"/") {
		return false
	}

	trimmed := strings.TrimPrefix(requestPath, adminWebPrefix)
	return trimmed != "/api" && !strings.HasPrefix(trimmed, "/api/")
}

func ServeWeb() gin.HandlerFunc {
	fileServer := http.FileServer(http.FS(adminDist))
	return func(c *gin.Context) {
		filePath := resolveWebPath(c)
		if filePath == "" {
			c.Request.URL.Path = "/"
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}

		if info, err := fs.Stat(adminDist, filePath); err == nil {
			if info.IsDir() {
				c.Request.URL.Path = "/" + strings.TrimSuffix(filePath, "/") + "/"
			} else {
				c.Request.URL.Path = "/" + filePath
			}
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}

		if strings.Contains(filePath, ".") {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		c.Request.URL.Path = "/"
		fileServer.ServeHTTP(c.Writer, c.Request)
	}
}

func mustAdminDist() fs.FS {
	sub, err := fs.Sub(webui.Dist, "dist")
	if err != nil {
		panic("embedded admin dist is missing: " + err.Error())
	}
	return sub
}

func resolveWebPath(c *gin.Context) string {
	if filePath := strings.TrimPrefix(c.Param("filepath"), "/"); filePath != "" {
		return filePath
	}

	requestPath := strings.TrimPrefix(c.Request.URL.Path, adminWebPrefix)
	return strings.TrimPrefix(requestPath, "/")
}
