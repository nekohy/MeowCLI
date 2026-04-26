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
	SessionID string `json:"session_id"`
	Stream    bool   `json:"stream"`
}

func (h *Handler) handleResponses(c *gin.Context, apiType utils.APIType) {
	ctx := c.Request.Context()

	body, relayErr, ok := readBridgeBody(c)
	if !ok {
		writeRelayError(c, relayErr)
		return
	}

	var req relayRequest
	if err := sonic.Unmarshal(body, &req); err != nil {
		writeRelayError(c, errReadRequestBody)
		return
	}

	alias := strings.Clone(strings.TrimSpace(req.Model))
	if alias == "" {
		writeRelayError(c, errModelRequired)
		return
	}

	target, relayErr, ok := h.resolveRelayTarget(ctx, alias, apiType)
	if !ok {
		writeRelayError(c, relayErr)
		return
	}
	sessionKey := sessionAffinityKey(target.info.Handler, req.SessionID)
	needReplace := alias != target.info.Origin
	upstreamBody := body
	if needReplace {
		upstreamBody = target.backend.ReplaceModel(body, target.info.Origin)
	}

	h.relayUpstream(c, upstreamRelay{
		ctx:                  ctx,
		scheduler:            target.sched,
		requestHeaders:       c.Request.Header,
		allowedPlans:         target.info.AllowedPlanTypes,
		streamRequest:        req.Stream,
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
