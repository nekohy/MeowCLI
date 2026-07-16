package bridge

import (
	"fmt"
	"strings"

	requestplugin "github.com/nekohy/MeowCLI/plugin"
	"github.com/nekohy/MeowCLI/utils"

	"github.com/bytedance/sonic/ast"
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

	request := requestplugin.NewContext(body)
	requestJSON, err := request.JSON()
	if err != nil {
		writeRelayError(c, errReadRequestBody)
		return
	}
	parsed, err := parseRelayRequest(requestJSON)
	if err != nil {
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

	request = preparePluginContext(request, alias, target.info, apiType, parsed.Stream)

	needReplace := alias != target.info.Origin
	if needReplace {
		if _, err := requestJSON.Set("model", ast.NewString(target.info.Origin)); err != nil {
			writeRelayError(c, errReadRequestBody)
			return
		}
	}

	finalPluginJSON, err := h.runModelPlugins(ctx, target.info.EnabledPlugins, request)
	if err != nil {
		writeRelayError(c, pluginFailure(err))
		return
	}
	prepared, err := target.backend.PrepareRequest(finalPluginJSON, apiType, nil)
	if err != nil {
		writeRelayError(c, errReadRequestBody)
		return
	}
	sessionKey := sessionAffinityKey(target.info.Handler, parsed.SessionID, h.settingsSnapshot())

	h.relayUpstream(c, upstreamRelay{
		ctx:                    ctx,
		scheduler:              target.sched,
		requestHeaders:         c.Request.Header,
		allowedPlans:           target.info.AllowedPlanTypes,
		streamRequest:          parsed.Stream,
		modelAlias:             alias,
		modelTier:              modelTier(target.info),
		apiType:                apiType,
		backend:                target.backend,
		replaceResponseModel:   needReplace,
		responseModel:          alias,
		requestJSON:            prepared.Root,
		sessionKey:             sessionKey,
		contentAffinityEnabled: target.info.ContentAffinity,
		payloadAPIType:         prepared.PayloadAPIType,
	})
}

func parseRelayRequest(root *ast.Node) (relayRequest, error) {
	if root == nil || root.TypeSafe() != ast.V_OBJECT {
		return relayRequest{}, fmt.Errorf("request body must be a JSON object")
	}
	model, err := requestString(root, "model")
	if err != nil {
		return relayRequest{}, err
	}
	sessionID, err := requestString(root, "prompt_cache_key")
	if err != nil {
		return relayRequest{}, err
	}
	stream, err := requestBool(root, "stream")
	if err != nil {
		return relayRequest{}, err
	}
	return relayRequest{Model: model, SessionID: sessionID, Stream: stream}, nil
}

func requestString(root *ast.Node, key string) (string, error) {
	node := root.Get(key)
	switch requestNodeType(node) {
	case ast.V_NONE, ast.V_NULL:
		return "", nil
	case ast.V_STRING:
		return node.StrictString()
	default:
		return "", fmt.Errorf("request field %q must be a string", key)
	}
}

func requestBool(root *ast.Node, key string) (bool, error) {
	node := root.Get(key)
	switch requestNodeType(node) {
	case ast.V_NONE, ast.V_NULL:
		return false, nil
	case ast.V_TRUE, ast.V_FALSE:
		return node.StrictBool()
	default:
		return false, fmt.Errorf("request field %q must be a boolean", key)
	}
}

func requestNodeType(node *ast.Node) int {
	if node == nil || !node.Exists() {
		return ast.V_NONE
	}
	if err := node.Load(); err != nil {
		return ast.V_ERROR
	}
	return node.TypeSafe()
}
