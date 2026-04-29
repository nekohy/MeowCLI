package bridge

import (
	"net/http"
	"slices"

	"github.com/nekohy/MeowCLI/utils"

	"github.com/bytedance/sonic"
	"github.com/gin-gonic/gin"
)

type openAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

func (h *Handler) RouteModels() gin.HandlerFunc {
	return func(c *gin.Context) {
		h.handleModels(c)
	}
}

func (h *Handler) handleModels(c *gin.Context) {
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

	models := make([]openAIModel, 0, len(items))
	for _, item := range items {
		backend, ok := h.backends[item.Handler]
		if !ok || !supportsOpenAIModelsAPI(backend.APIType()) {
			continue
		}
		models = append(models, openAIModel{
			ID:      item.Alias,
			Object:  "model",
			Created: 0,
			OwnedBy: string(item.Handler),
		})
	}

	body, err := sonic.Marshal(map[string]any{
		"object": "list",
		"data":   models,
	})
	if err != nil {
		writeRelayError(c, errRelayResponseFailed)
		return
	}
	c.Data(http.StatusOK, "application/json", body)
}

func supportsOpenAIModelsAPI(apiTypes []utils.APIType) bool {
	return slices.Contains(apiTypes, utils.APIResponses) || slices.Contains(apiTypes, utils.APICompletion)
}
