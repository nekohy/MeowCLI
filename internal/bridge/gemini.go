package bridge

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/nekohy/MeowCLI/api"
	"github.com/nekohy/MeowCLI/api/antigravity"
	"github.com/nekohy/MeowCLI/api/gemini"
	requestplugin "github.com/nekohy/MeowCLI/plugin"
	"github.com/nekohy/MeowCLI/utils"

	"github.com/bytedance/sonic"
	"github.com/gin-gonic/gin"
)

func (h *Handler) handleGemini(c *gin.Context) {
	ctx := c.Request.Context()

	body, relayErr, ok := readBridgeBody(c)
	if !ok {
		writeRelayError(c, relayErr)
		return
	}

	alias, action, err := parseGenerateContentTarget(c.Param("target"))
	if err != nil {
		writeRelayError(c, errModelRequired)
		return
	}
	stream := action == "streamGenerateContent"

	target, relayErr, ok := h.resolveRelayTarget(ctx, alias, utils.APIGemini)
	if !ok {
		writeRelayError(c, relayErr)
		return
	}
	if target.info.Handler != utils.HandlerGemini && target.info.Handler != utils.HandlerAntigravity {
		writeRelayError(c, errUnsupportedAPIType)
		return
	}
	request := preparePluginContext(requestplugin.NewContext(body), alias, target.info, utils.APIGemini, stream)

	finalPluginJSON, err := h.runModelPlugins(ctx, target.info.EnabledPlugins, request)
	if err != nil {
		writeRelayError(c, pluginFailure(err))
		return
	}
	backendOptions := generateContentBackendOptions(target.info.Handler, target.info.Origin, action, c.Request.URL.RawQuery)
	prepared, err := target.backend.PrepareRequest(finalPluginJSON, utils.APIGemini, backendOptions)
	if err != nil {
		writeRelayError(c, errReadRequestBody)
		return
	}
	h.relayUpstream(c, upstreamRelay{
		ctx:                   ctx,
		scheduler:             target.sched,
		requestHeaders:        c.Request.Header,
		allowedPlans:          target.info.AllowedPlanTypes,
		streamRequest:         stream,
		modelAlias:            alias,
		modelTier:             modelTier(target.info),
		apiType:               utils.APIGemini,
		backend:               target.backend,
		replaceResponseModel:  alias != target.info.Origin,
		responseModel:         alias,
		requestJSON:           prepared.Root,
		backendOptions:        backendOptions,
		prepareBackendOptions: prepareGenerateContentBackendOptions(target.sched),
		modelScheduling:       target.info.Scheduling,
		payloadAPIType:        prepared.PayloadAPIType,
	})
}

func generateContentBackendOptions(handler utils.HandlerType, modelName string, action string, rawQuery string) api.BackendOpts {
	if handler == utils.HandlerAntigravity {
		return &antigravity.Options{
			ModelName: modelName,
			Action:    action,
			RawQuery:  rawQuery,
		}
	}
	return &gemini.Options{
		ModelName: modelName,
		Action:    action,
		RawQuery:  rawQuery,
	}
}

func prepareGenerateContentBackendOptions(scheduler CredentialScheduler) func(context.Context, string, api.BackendOpts) (api.BackendOpts, error) {
	resolver, ok := scheduler.(ProjectIDProvider)
	if !ok {
		return nil
	}
	creditTypesProvider, _ := scheduler.(CreditTypesProvider)
	return func(ctx context.Context, credentialID string, opts api.BackendOpts) (api.BackendOpts, error) {
		switch typed := opts.(type) {
		case *gemini.Options:
			projectID, err := resolver.ProjectID(ctx, credentialID)
			if err != nil {
				return nil, fmt.Errorf("resolve gemini project id: %w", err)
			}
			cloned := *typed
			cloned.ProjectID = projectID
			return &cloned, nil
		case *antigravity.Options:
			projectID, err := resolver.ProjectID(ctx, credentialID)
			if err != nil {
				return nil, fmt.Errorf("resolve antigravity project id: %w", err)
			}
			cloned := *typed
			cloned.ProjectID = projectID
			if creditTypesProvider != nil {
				creditTypes, err := creditTypesProvider.CreditTypes(ctx, credentialID)
				if err != nil {
					return nil, fmt.Errorf("resolve antigravity credit types: %w", err)
				}
				cloned.CreditTypes = creditTypes
			}
			return &cloned, nil
		default:
			return opts, nil
		}
	}
}

func parseGenerateContentTarget(rawTarget string) (string, string, error) {
	target := strings.TrimSpace(rawTarget)
	target = strings.TrimPrefix(target, "/")
	if target == "" {
		return "", "", errors.New("empty generateContent target")
	}

	modelPart, action, ok := strings.Cut(target, ":")
	if !ok {
		return "", "", errors.New("invalid generateContent target")
	}
	modelPart = strings.TrimSpace(modelPart)
	action = strings.TrimSpace(action)
	if modelPart == "" || action == "" {
		return "", "", errors.New("invalid generateContent target")
	}
	if action != "generateContent" && action != "streamGenerateContent" {
		return "", "", errors.New("unsupported generateContent action")
	}
	modelName, err := url.PathUnescape(modelPart)
	if err != nil {
		return "", "", err
	}
	return modelName, action, nil
}

type geminiModel struct {
	Name                       string   `json:"name"`
	BaseModelID                string   `json:"baseModelId"`
	DisplayName                string   `json:"displayName"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
}

type geminiModelsResponse struct {
	Models []geminiModel `json:"models"`
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
	models := make([]geminiModel, 0, len(items))
	for _, item := range items {
		if item.Handler != utils.HandlerGemini && item.Handler != utils.HandlerAntigravity {
			continue
		}
		models = append(models, geminiModel{
			Name:                       "models/" + item.Alias,
			BaseModelID:                item.Origin,
			DisplayName:                item.Alias,
			SupportedGenerationMethods: []string{"generateContent", "streamGenerateContent"},
		})
	}
	body, err := sonic.Marshal(geminiModelsResponse{Models: models})
	if err != nil {
		writeRelayError(c, errRelayResponseFailed)
		return
	}
	c.Data(http.StatusOK, "application/json", body)
}
