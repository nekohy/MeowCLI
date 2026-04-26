package bridge

import (
	"context"
	"errors"
	"github.com/bytedance/sonic"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/nekohy/MeowCLI/api"
	"github.com/nekohy/MeowCLI/api/gemini"
	coreGemini "github.com/nekohy/MeowCLI/core/gemini"
	storedb "github.com/nekohy/MeowCLI/internal/store"
	"github.com/nekohy/MeowCLI/utils"

	"github.com/gin-gonic/gin"
)

func (h *Handler) RouteGemini() gin.HandlerFunc {
	return func(c *gin.Context) {
		h.handleGemini(c)
	}
}

func (h *Handler) RouteGeminiModels() gin.HandlerFunc {
	return func(c *gin.Context) {
		h.handleGeminiModels(c)
	}
}

func (h *Handler) handleGemini(c *gin.Context) {
	ctx := c.Request.Context()

	bodyReader := http.MaxBytesReader(c.Writer, c.Request.Body, maxBridgeRequestBodyBytes)
	body, err := io.ReadAll(bodyReader)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeRelayError(c, errRequestBodyTooLarge)
			return
		}
		writeRelayError(c, errReadRequestBody)
		return
	}

	alias, action, err := parseGeminiModelAction(c.Param("target"))
	if err != nil {
		writeRelayError(c, errModelRequired)
		return
	}

	info, err := h.resolveModel(ctx, alias)
	if err != nil {
		switch {
		case errors.Is(err, storedb.ErrNotFound):
			writeRelayError(c, errModelNotConfigured)
		default:
			writeRelayError(c, errModelResolutionFailed)
		}
		return
	}
	if info.Handler != utils.HandlerGemini {
		writeRelayError(c, errUnsupportedAPIType)
		return
	}

	backend, ok := h.backends[info.Handler]
	if !ok {
		writeRelayError(c, errBackendUnavailable)
		return
	}
	if !slices.Contains(backend.APIType(), utils.APIGemini) {
		writeRelayError(c, errUnsupportedAPIType)
		return
	}

	sched, ok := h.schedulers[info.Handler]
	if !ok {
		writeRelayError(c, errSchedulerUnavailable)
		return
	}

	streamRequest := action == "streamGenerateContent"
	needReplace := alias != info.Origin
	modelTier := coreGemini.ResolveModelTier(info.Origin)

	h.relayWithRetry(c, relayConfig{
		ctx:            ctx,
		sched:          sched,
		requestHeaders: c.Request.Header,
		allowedPlans:   info.AllowedPlanTypes,
		streamRequest:  streamRequest,
		modelAlias:     alias,
		modelTier:      modelTier,
		apiType:        utils.APIGemini,
		backend:        backend,
		needReplace:    needReplace,
		responseAlias:  alias,
		resolvePreferred: func(graceCredID string) string {
			return graceCredID
		},
		doRequest: func(attemptCtx context.Context, credID string, headers http.Header) (*http.Response, error) {
			return backend.Chat(&api.Request{
				Ctx:     attemptCtx,
				CredID:  credID,
				Body:    body,
				Headers: headers,
				Opts: &gemini.Options{
					ModelName: info.Origin,
					Action:    action,
					RawQuery:  c.Request.URL.RawQuery,
					ProjectID: headers.Get("X-Meow-Gemini-Project"),
				},
			})
		},
		onSuccess: nil,
	})
}

func parseGeminiModelAction(rawTarget string) (string, string, error) {
	target := strings.TrimSpace(rawTarget)
	target = strings.TrimPrefix(target, "/")
	if target == "" {
		return "", "", errors.New("empty gemini target")
	}

	modelPart, action, ok := strings.Cut(target, ":")
	if !ok {
		return "", "", errors.New("invalid gemini target")
	}
	modelPart = strings.TrimSpace(modelPart)
	action = strings.TrimSpace(action)
	if modelPart == "" || action == "" {
		return "", "", errors.New("invalid gemini target")
	}
	if action != "generateContent" && action != "streamGenerateContent" {
		return "", "", errors.New("unsupported gemini action")
	}
	modelName, err := url.PathUnescape(modelPart)
	if err != nil {
		return "", "", err
	}
	return modelName, action, nil
}

func (h *Handler) handleGeminiModels(c *gin.Context) {
	ctx := c.Request.Context()
	lister, ok := h.models.(ModelLister)
	if !ok {
		writeRelayError(c, errBackendUnavailable)
		return
	}
	items, err := lister.ListModels(ctx)
	if err != nil {
		writeRelayError(c, errModelResolutionFailed)
		return
	}
	models := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if item.Handler != utils.HandlerGemini {
			continue
		}
		models = append(models, map[string]any{
			"name":                       "models/" + item.Alias,
			"baseModelId":                item.Origin,
			"displayName":                item.Alias,
			"supportedGenerationMethods": []string{"generateContent", "streamGenerateContent"},
		})
	}
	body, err := sonic.Marshal(map[string]any{"models": models})
	if err != nil {
		writeRelayError(c, errRelayResponseFailed)
		return
	}
	c.Data(http.StatusOK, "application/json", body)
}
