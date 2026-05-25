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

	antigravityapi "github.com/nekohy/MeowCLI/api/antigravity"
	codexapi "github.com/nekohy/MeowCLI/api/codex"
	geminiapi "github.com/nekohy/MeowCLI/api/gemini"
	oauthcore "github.com/nekohy/MeowCLI/core"
	coreantigravity "github.com/nekohy/MeowCLI/core/antigravity"
	corecodex "github.com/nekohy/MeowCLI/core/codex"
	coregemini "github.com/nekohy/MeowCLI/core/gemini"
	"github.com/nekohy/MeowCLI/internal/auth"
	"github.com/nekohy/MeowCLI/internal/settings"
	db "github.com/nekohy/MeowCLI/internal/store"
	requestplugin "github.com/nekohy/MeowCLI/plugin"
	pluginloader "github.com/nekohy/MeowCLI/plugin/loader"
	"github.com/nekohy/MeowCLI/utils"

	"github.com/gin-gonic/gin"
)

// CredentialRefresher 凭证变更后刷新调度器缓存
type CredentialRefresher interface {
	RefreshAvailable(ctx context.Context, handler utils.HandlerType) error
	SyncQuotas(ctx context.Context, handler utils.HandlerType, ids []string)
	InvalidateCredentials(handler utils.HandlerType, ids []string)
	CachedCodexQuota(id string) (corecodex.CachedQuotaSnapshot, bool)
	CachedGeminiQuota(id string) (coregemini.CachedQuotaSnapshot, bool)
	CachedAntigravityQuota(id string) (coreantigravity.CachedQuotaSnapshot, bool)
}

type ModelCache interface {
	InvalidateModel(alias string)
}

type BuildInfo struct {
	Version   string `json:"version"`
	BuildTime string `json:"build_time"`
}

type BuildInfoProvider func() BuildInfo

// AdminHandler 管理后台 API
type AdminHandler struct {
	store          db.Store
	logStore       LogStore
	codexAPI       *codexapi.Client
	geminiAPI      *geminiapi.Client
	antigravityAPI *antigravityapi.Client
	oauthFlows     map[utils.HandlerType]oauthcore.OAuthFlow
	authCache      *auth.KeyCache
	credRefresh    CredentialRefresher
	modelCache     ModelCache
	settingsSvc    *settings.Service
	importJobs     *importJobManager
	buildInfo      BuildInfoProvider
	pluginRegistry *requestplugin.Registry
	mu             sync.Mutex
}

func NewAdminHandler(store db.Store, codexAPI *codexapi.Client, geminiAPI *geminiapi.Client, antigravityAPI *antigravityapi.Client) *AdminHandler {
	return &AdminHandler{
		store:          store,
		codexAPI:       codexAPI,
		geminiAPI:      geminiAPI,
		antigravityAPI: antigravityAPI,
		importJobs:     newImportJobManager(defaultImportConcurrency),
	}
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
	if a.importJobs != nil {
		a.importJobs.SetSettingsProvider(svc)
	}
}

func (a *AdminHandler) SetBuildInfoProvider(provider BuildInfoProvider) {
	a.buildInfo = provider
}

func (a *AdminHandler) SetPluginRegistry(registry *requestplugin.Registry) {
	if a == nil {
		return
	}
	a.pluginRegistry = registry
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

func normalizeModelInput(alias, origin, handler string, planTypes string, plugins string, extra json.RawMessage, registry *requestplugin.Registry) (string, string, string, string, string, json.RawMessage, error) {
	alias = strings.TrimSpace(alias)
	origin = strings.TrimSpace(origin)
	handler = strings.TrimSpace(handler)
	if alias == "" {
		return "", "", "", "", "", nil, fmt.Errorf("alias is required")
	}
	if origin == "" {
		return "", "", "", "", "", nil, fmt.Errorf("origin is required")
	}
	parsedHandler, ok := utils.ParseHandlerType(handler)
	if !ok {
		return "", "", "", "", "", nil, fmt.Errorf("unknown handler type: %q", handler)
	}
	switch parsedHandler {
	case utils.HandlerGemini:
		planTypes = utils.NormalizeCodeAssistPlanTypeList(planTypes)
	case utils.HandlerAntigravity:
		planTypes = utils.NormalizeCodeAssistPlanTypeList(planTypes)
	case utils.HandlerCodex:
		planTypes = corecodex.NormalizePlanTypeList(planTypes)
	default:
		return "", "", "", "", "", nil, fmt.Errorf("unsupported handler type: %q", parsedHandler)
	}
	normalizedPlugins, err := normalizeModelPlugins(plugins, parsedHandler, registry)
	if err != nil {
		return "", "", "", "", "", nil, err
	}
	if len(extra) == 0 {
		extra = json.RawMessage("{}")
	} else if !sonic.Valid(extra) {
		return "", "", "", "", "", nil, fmt.Errorf("extra must be valid JSON")
	}
	return alias, origin, string(parsedHandler), planTypes, normalizedPlugins, extra, nil
}

func normalizeModelPlugins(raw string, handler utils.HandlerType, registry *requestplugin.Registry) (string, error) {
	names := requestplugin.ParseList(raw)
	if len(names) == 0 {
		return "", nil
	}
	if registry == nil {
		registry = pluginloader.DefaultRegistry()
	}
	normalized := names[:0]
	for _, name := range names {
		p, ok := registry.Get(name)
		if !ok {
			return "", fmt.Errorf("unknown plugin: %q", name)
		}
		if !p.Manifest().SupportsHandler(handler) {
			return "", fmt.Errorf("plugin %q is not available for handler %q", name, handler)
		}
		normalized = append(normalized, name)
	}
	return strings.Join(normalized, ","), nil
}

func (a *AdminHandler) currentSettings() settings.Snapshot {
	if a == nil || a.settingsSvc == nil {
		return settings.DefaultSnapshot()
	}
	return a.settingsSvc.Snapshot()
}

func (a *AdminHandler) currentBuildInfo() BuildInfo {
	if a == nil || a.buildInfo == nil {
		return BuildInfo{
			Version:   "dev",
			BuildTime: "unknown",
		}
	}
	return a.buildInfo()
}
