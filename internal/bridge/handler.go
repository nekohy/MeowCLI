package bridge

import (
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/nekohy/MeowCLI/api"
	corecodex "github.com/nekohy/MeowCLI/core/codex"
	coreGemini "github.com/nekohy/MeowCLI/core/gemini"
	"github.com/nekohy/MeowCLI/core/scheduling"
	"github.com/nekohy/MeowCLI/internal/settings"
	storedb "github.com/nekohy/MeowCLI/internal/store"
	"github.com/nekohy/MeowCLI/utils"

	"github.com/gin-gonic/gin"
	"github.com/maypok86/otter/v2"
)

type ResolvedModel struct {
	Origin           string
	Handler          utils.HandlerType
	AllowedPlanTypes []string
}

// ModelStore provides model alias resolution.
type ModelStore interface {
	ResolveModel(ctx context.Context, alias string) (*ResolvedModel, error)
}

type ModelListItem struct {
	Alias   string
	Origin  string
	Handler utils.HandlerType
}

type ModelLister interface {
	ListModels(ctx context.Context) ([]ModelListItem, error)
}

type CredentialScheduler interface {
	SelectCredential(ctx context.Context, selection scheduling.CredentialSelection) (credentialID string, err error)
	AuthHeaders(ctx context.Context, credentialID string) (http.Header, error)
	RecordSuccess(ctx context.Context, credentialID string, statusCode int32, modelTier string, metrics storedb.LogRequestMetrics)
	RecordFailure(ctx context.Context, credentialID string, statusCode int32, modelTier string, retryAfter time.Duration, metrics storedb.LogRequestMetrics)
	HandleUnauthorized(ctx context.Context, credentialID string, statusCode int32, modelTier string, metrics storedb.LogRequestMetrics) bool
	QueueQuotaRefresh(ctx context.Context, credentialID string, modelTier string)
	RetryDecision(statusCode int32, text string, headers http.Header) scheduling.RetryDecision
}

type relayTarget struct {
	info    *ResolvedModel
	backend api.Backend
	sched   CredentialScheduler
}

type Handler struct {
	backends   map[utils.HandlerType]api.Backend
	models     ModelStore
	schedulers map[utils.HandlerType]CredentialScheduler
	settings   settings.Provider
	sessions   *otter.Cache[string, string]
}

func NewHandler(models ModelStore, schedulers map[utils.HandlerType]CredentialScheduler, backends ...api.Backend) *Handler {
	m := make(map[utils.HandlerType]api.Backend, len(backends))
	for _, b := range backends {
		m[b.HandlerType()] = b
	}
	return &Handler{
		backends:   m,
		models:     models,
		schedulers: schedulers,
		sessions:   newSessionAffinityCache(),
	}
}

func (h *Handler) SetSettingsProvider(provider settings.Provider) {
	if h == nil {
		return
	}
	h.settings = provider
}

func (h *Handler) Route(apiType utils.APIType) gin.HandlerFunc {
	return func(c *gin.Context) {
		h.handleResponses(c, apiType)
	}
}

func (h *Handler) resolveModel(ctx context.Context, alias string) (*ResolvedModel, error) {
	return h.models.ResolveModel(ctx, alias)
}

func (h *Handler) maxRetries() int {
	return h.settingsSnapshot().RelayMaxRetries
}

func (h *Handler) maxAttempts() int {
	retries := h.maxRetries()
	if retries < 0 {
		retries = 0
	}
	return retries + 1
}

func (h *Handler) settingsSnapshot() settings.Snapshot {
	if h == nil || h.settings == nil {
		return settings.DefaultSnapshot()
	}
	return h.settings.Snapshot()
}

func readBridgeBody(c *gin.Context) ([]byte, relayError, bool) {
	bodyReader := http.MaxBytesReader(c.Writer, c.Request.Body, maxBridgeRequestBodyBytes)
	body, err := io.ReadAll(bodyReader)
	if err == nil {
		return body, relayError{}, true
	}
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return nil, errRequestBodyTooLarge, false
	}
	return nil, errReadRequestBody, false
}

func (h *Handler) resolveRelayTarget(ctx context.Context, alias string, apiType utils.APIType) (relayTarget, relayError, bool) {
	info, err := h.resolveModel(ctx, alias)
	if err != nil {
		if errors.Is(err, storedb.ErrNotFound) {
			return relayTarget{}, errModelNotConfigured, false
		}
		return relayTarget{}, errModelResolutionFailed, false
	}

	backend, ok := h.backends[info.Handler]
	if !ok {
		return relayTarget{}, errBackendUnavailable, false
	}
	if !slices.Contains(backend.APIType(), apiType) {
		return relayTarget{}, errUnsupportedAPIType, false
	}

	sched, ok := h.schedulers[info.Handler]
	if !ok {
		return relayTarget{}, errSchedulerUnavailable, false
	}

	return relayTarget{
		info:    info,
		backend: backend,
		sched:   sched,
	}, relayError{}, true
}

func sessionAffinityKey(providerType utils.HandlerType, sessionID string) string {
	provider := strings.TrimSpace(string(providerType))
	sessionID = strings.TrimSpace(sessionID)
	if provider != "" && sessionID != "" {
		return provider + "-" + sessionID
	}
	return ""
}

func modelTier(info *ResolvedModel) string {
	if info == nil {
		return ""
	}
	switch info.Handler {
	case utils.HandlerCodex:
		return corecodex.ResolveModelTier(info.Origin)
	case utils.HandlerGemini:
		return coreGemini.ResolveModelTier(info.Origin)
	default:
		return ""
	}
}
