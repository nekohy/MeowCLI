package bridge

import (
	"strings"

	"github.com/nekohy/MeowCLI/utils"

	"github.com/bytedance/sonic"
	"github.com/gin-gonic/gin"
)

const maxBridgeRequestBodyBytes = 32 << 20

type relayRequest struct {
	Model     string `json:"model"`
	SessionID string `json:"prompt_cache_key"`
	Stream    bool   `json:"stream"`
}

func (h *Handler) handleResponses(c *gin.Context, apiType utils.APIType) {
	ctx := c.Request.Context()

	body, relayErr, ok := readBridgeBody(c)
	if !ok {
		writeRelayError(c, relayErr)
		return
	}

	var parsed relayRequest
	if err := sonic.Unmarshal(body, &parsed); err != nil {
		writeRelayError(c, errReadRequestBody)
		return
	}

	alias := strings.Clone(strings.TrimSpace(parsed.Model))
	if alias == "" {
		writeRelayError(c, errModelRequired)
		return
	}

	target, relayErr, ok := h.resolveRelayTarget(ctx, alias, apiType)
	if !ok {
		writeRelayError(c, relayErr)
		return
	}

	needReplace := alias != target.info.Origin
	upstreamBody := body
	if needReplace {
		upstreamBody = target.backend.ReplaceModel(body, target.info.Origin)
	}

	upstreamBody, err := h.runModelPlugins(ctx, pluginRequest{
		Alias:          alias,
		Origin:         target.info.Origin,
		Handler:        target.info.Handler,
		APIType:        apiType,
		Stream:         parsed.Stream,
		EnabledPlugins: target.info.EnabledPlugins,
		Body:           upstreamBody,
	})
	if err != nil {
		writeRelayError(c, pluginFailure(err))
		return
	}

	sessionKey := sessionAffinityKey(target.info.Handler, parsed.SessionID, h.settingsSnapshot())

	h.relayUpstream(c, upstreamRelay{
		ctx:                  ctx,
		scheduler:            target.sched,
		requestHeaders:       c.Request.Header,
		allowedPlans:         target.info.AllowedPlanTypes,
		streamRequest:        parsed.Stream,
		modelAlias:           alias,
		modelTier:            modelTier(target.info),
		apiType:              apiType,
		backend:              target.backend,
		replaceResponseModel: needReplace,
		responseModel:        alias,
		requestBody:          upstreamBody,
		sessionKey:           sessionKey,
	})
}
