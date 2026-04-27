package bridge

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/nekohy/MeowCLI/api"
	"github.com/nekohy/MeowCLI/api/gemini"
	"github.com/nekohy/MeowCLI/utils"

	"github.com/bytedance/sonic"
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

	body, relayErr, ok := readBridgeBody(c)
	if !ok {
		writeRelayError(c, relayErr)
		return
	}

	alias, action, err := parseGeminiModelAction(c.Param("target"))
	if err != nil {
		writeRelayError(c, errModelRequired)
		return
	}

	target, relayErr, ok := h.resolveRelayTarget(ctx, alias, utils.APIGemini)
	if !ok {
		writeRelayError(c, relayErr)
		return
	}
	if target.info.Handler != utils.HandlerGemini {
		writeRelayError(c, errUnsupportedAPIType)
		return
	}

	h.relayUpstream(c, upstreamRelay{
		ctx:                  ctx,
		scheduler:            target.sched,
		requestHeaders:       c.Request.Header,
		allowedPlans:         target.info.AllowedPlanTypes,
		streamRequest:        action == "streamGenerateContent",
		modelAlias:           alias,
		modelTier:            modelTier(target.info),
		apiType:              utils.APIGemini,
		backend:              target.backend,
		replaceResponseModel: alias != target.info.Origin,
		responseModel:        alias,
		requestBody:          body,
		backendOptions: &gemini.Options{
			ModelName: target.info.Origin,
			Action:    action,
			RawQuery:  c.Request.URL.RawQuery,
		},
		prepareBackendOptions: prepareGeminiBackendOptions(target.sched),
	})
}

func prepareGeminiBackendOptions(scheduler CredentialScheduler) func(context.Context, string, api.BackendOpts) (api.BackendOpts, error) {
	resolver, ok := scheduler.(ProjectIDProvider)
	if !ok {
		return nil
	}
	return func(ctx context.Context, credentialID string, opts api.BackendOpts) (api.BackendOpts, error) {
		geminiOpts, ok := opts.(*gemini.Options)
		if !ok {
			return opts, nil
		}
		projectID, err := resolver.ProjectID(ctx, credentialID)
		if err != nil {
			return nil, fmt.Errorf("resolve gemini project id: %w", err)
		}
		cloned := *geminiOpts
		cloned.ProjectID = projectID
		return &cloned, nil
	}
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
