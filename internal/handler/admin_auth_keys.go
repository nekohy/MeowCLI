package handler

import (
	db "github.com/nekohy/MeowCLI/internal/store"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func (a *AdminHandler) ListAuthKeys(c *gin.Context) {
	keys, err := a.store.ListAuthKeys(c.Request.Context())
	if err != nil {
		writeInternalError(c, err)
		return
	}
	type keyItem struct {
		Key       string `json:"key"`
		Role      string `json:"role"`
		Note      string `json:"note"`
		CreatedAt string `json:"created_at"`
	}
	out := make([]keyItem, len(keys))
	for i, k := range keys {
		out[i] = keyItem{
			Key:       k.Key,
			Role:      k.Role,
			Note:      k.Note,
			CreatedAt: k.CreatedAt.Format("2006-01-02 15:04:05"),
		}
	}
	c.JSON(http.StatusOK, out)
}

func (a *AdminHandler) CreateAuthKey(c *gin.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.ensureAuthCache(c) {
		return
	}
	var body struct {
		Key  string `json:"key"`
		Role string `json:"role"`
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

	role := strings.TrimSpace(body.Role)
	if role == "" {
		role = "user"
	}
	if role != "admin" && role != "user" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role must be admin or user"})
		return
	}

	note := strings.TrimSpace(body.Note)

	created, err := a.store.CreateAuthKey(c.Request.Context(), db.CreateAuthKeyParams{
		Key:  key,
		Role: role,
		Note: note,
	})
	if writeStoreError(c, err, "", "key already exists") {
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

func (a *AdminHandler) UpdateAuthKey(c *gin.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.ensureAuthCache(c) {
		return
	}
	key := strings.TrimSpace(c.Param("key"))
	var body struct {
		Role string `json:"role"`
		Note string `json:"note"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	role := strings.TrimSpace(body.Role)
	if role != "admin" && role != "user" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role must be admin or user"})
		return
	}
	note := strings.TrimSpace(body.Note)

	ctx := c.Request.Context()

	updated, err := a.store.UpdateAuthKeyChecked(ctx, key, role, note)
	if writeAdminKeyError(c, err, "key not found", "", "", "cannot downgrade the last admin key") {
		return
	}

	if err := a.authCache.Refresh(ctx); err != nil {
		writeInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"key":        updated.Key,
		"role":       updated.Role,
		"note":       updated.Note,
		"created_at": updated.CreatedAt.Format("2006-01-02 15:04:05"),
	})
}

func (a *AdminHandler) DeleteAuthKey(c *gin.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.ensureAuthCache(c) {
		return
	}
	key := strings.TrimSpace(c.Param("key"))
	ctx := c.Request.Context()

	if err := a.store.DeleteAuthKeyChecked(ctx, key); writeAdminKeyError(c, err, "key not found", "", "", "cannot delete the last admin key") {
		return
	}

	if err := a.authCache.Refresh(ctx); err != nil {
		writeInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}
