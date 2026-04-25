package handler

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/nekohy/MeowCLI/internal/auth"
	db "github.com/nekohy/MeowCLI/internal/store"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// SetAuthCache injects the KeyCache after construction (called from app init).
func (a *AdminHandler) SetAuthCache(cache *auth.KeyCache) {
	a.authCache = cache
}

// Setup 首次初始化 — 创建第一个 admin key（仅在没有 admin key 时可用）
func (a *AdminHandler) Setup(c *gin.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.ensureAuthCache(c) {
		return
	}
	if !a.authCache.NeedsSetup() {
		c.JSON(http.StatusForbidden, gin.H{"error": "already initialized"})
		return
	}

	var body struct {
		Key  string `json:"key"`
		Note string `json:"note"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	key := strings.TrimSpace(body.Key)
	if key == "" {
		generated, err := generateKey()
		if err != nil {
			writeInternalError(c, err)
			return
		}
		key = generated
	}

	note := strings.TrimSpace(body.Note)
	if note == "" {
		note = "initial admin key"
	}

	created, err := a.store.CreateInitialAuthKey(c.Request.Context(), db.CreateAuthKeyParams{
		Key:  key,
		Role: "admin",
		Note: note,
	})
	if err != nil {
		if errors.Is(err, db.ErrAlreadyInitialized) {
			_ = a.authCache.Refresh(c.Request.Context())
		}
		if writeAdminKeyError(c, err, "", "key already exists", "already initialized", "") {
			return
		}
		return
	}

	if err := a.authCache.Refresh(c.Request.Context()); err != nil {
		writeInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"key":        created.Key,
		"role":       created.Role,
		"note":       created.Note,
		"created_at": created.CreatedAt.Format("2006-01-02 15:04:05"),
	})
}

// Status 返回系统状态（是否需要初始化），无需鉴权
func (a *AdminHandler) Status(c *gin.Context) {
	if !a.ensureAuthCache(c) {
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"need_setup": a.authCache.NeedsSetup(),
		"build_info": a.currentBuildInfo(),
	})
}

func generateKey() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "sk-" + hex.EncodeToString(b), nil
}
