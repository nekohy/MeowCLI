package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/bytedance/sonic"

	codexapi "github.com/nekohy/MeowCLI/api/codex"
	geminiapi "github.com/nekohy/MeowCLI/api/gemini"
	corecodex "github.com/nekohy/MeowCLI/core/codex"
	coregemini "github.com/nekohy/MeowCLI/core/gemini"
	"github.com/nekohy/MeowCLI/internal/auth"
	"github.com/nekohy/MeowCLI/internal/settings"
	db "github.com/nekohy/MeowCLI/internal/store"
	"github.com/nekohy/MeowCLI/utils"

	"github.com/gin-gonic/gin"
)

// CredentialRefresher 凭证变更后刷新调度器缓存
type CredentialRefresher interface {
	RefreshAvailable(ctx context.Context, handler utils.HandlerType) error
	SyncQuotas(ctx context.Context, handler utils.HandlerType, ids []string)
	InvalidateCredentials(handler utils.HandlerType, ids []string)
}

type ModelCache interface {
	InvalidateModel(alias string)
}

// AdminHandler 管理后台 API
type AdminHandler struct {
	store       db.Store
	logStore    LogStore
	codexAPI    *codexapi.Client
	geminiAPI   *geminiapi.Client
	authCache   *auth.KeyCache
	credRefresh CredentialRefresher
	modelCache  ModelCache
	settingsSvc *settings.Service
	mu          sync.Mutex
}

func NewAdminHandler(store db.Store, codexAPI *codexapi.Client, geminiAPI *geminiapi.Client) *AdminHandler {
	return &AdminHandler{store: store, codexAPI: codexAPI, geminiAPI: geminiAPI}
}

func (a *AdminHandler) SetLogStore(store LogStore) {
	a.logStore = store
}

func (a *AdminHandler) SetCredentialRefresher(r CredentialRefresher) {
	a.credRefresh = r
}

func (a *AdminHandler) SetModelCache(cache ModelCache) {
	a.modelCache = cache
}

func (a *AdminHandler) SetSettingsService(svc *settings.Service) {
	a.settingsSvc = svc
}

func (a *AdminHandler) ensureAuthCache(c *gin.Context) bool {
	if a != nil && a.authCache != nil {
		return true
	}
	c.JSON(http.StatusServiceUnavailable, gin.H{"error": "auth cache is unavailable"})
	return false
}

func writeInternalError(c *gin.Context, err error) {
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

func writeStoreError(c *gin.Context, err error, notFoundMsg, conflictMsg string) bool {
	if err == nil {
		return false
	}
	message := storeErrorMessage(err, notFoundMsg, conflictMsg)
	switch {
	case errors.Is(err, db.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": message})
	case errors.Is(err, db.ErrConflict):
		c.JSON(http.StatusConflict, gin.H{"error": message})
	default:
		writeInternalError(c, err)
	}
	return true
}

func storeErrorMessage(err error, notFoundMsg, conflictMsg string) string {
	switch {
	case errors.Is(err, db.ErrNotFound):
		if notFoundMsg == "" {
			return "resource not found"
		}
		return notFoundMsg
	case errors.Is(err, db.ErrConflict):
		if conflictMsg == "" {
			return "resource already exists"
		}
		return conflictMsg
	case err == nil:
		return ""
	default:
		return err.Error()
	}
}

type batchDeleteReq struct {
	IDs []string `json:"ids" binding:"required,min=1"`
}

func writeAdminKeyError(c *gin.Context, err error, notFoundMsg, conflictMsg, initializedMsg, lastAdminMsg string) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, db.ErrAlreadyInitialized):
		if initializedMsg == "" {
			initializedMsg = "already initialized"
		}
		c.JSON(http.StatusForbidden, gin.H{"error": initializedMsg})
	case errors.Is(err, db.ErrLastAdmin):
		if lastAdminMsg == "" {
			lastAdminMsg = "cannot modify the last admin key"
		}
		c.JSON(http.StatusConflict, gin.H{"error": lastAdminMsg})
	default:
		return writeStoreError(c, err, notFoundMsg, conflictMsg)
	}
	return true
}

func normalizeModelInput(alias, origin, handler string, planTypes string, extra json.RawMessage) (string, string, string, string, json.RawMessage, error) {
	alias = strings.TrimSpace(alias)
	origin = strings.TrimSpace(origin)
	handler = strings.TrimSpace(handler)
	if alias == "" {
		return "", "", "", "", nil, fmt.Errorf("alias is required")
	}
	if origin == "" {
		return "", "", "", "", nil, fmt.Errorf("origin is required")
	}
	parsedHandler, ok := utils.ParseHandlerType(handler)
	if !ok {
		return "", "", "", "", nil, fmt.Errorf("unknown handler type: %q", handler)
	}
	switch parsedHandler {
	case utils.HandlerGemini:
		planTypes = coregemini.NormalizePlanTypeList(planTypes)
	case utils.HandlerCodex:
		planTypes = corecodex.NormalizePlanTypeList(planTypes)
	default:
		return "", "", "", "", nil, fmt.Errorf("unsupported handler type: %q", parsedHandler)
	}
	if len(extra) == 0 {
		extra = json.RawMessage("{}")
	} else if !sonic.Valid(extra) {
		return "", "", "", "", nil, fmt.Errorf("extra must be valid JSON")
	}
	return alias, origin, string(parsedHandler), planTypes, extra, nil
}

func (a *AdminHandler) currentSettings() settings.Snapshot {
	if a == nil || a.settingsSvc == nil {
		return settings.DefaultSnapshot()
	}
	return a.settingsSvc.Snapshot()
}
