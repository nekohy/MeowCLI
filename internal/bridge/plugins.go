package bridge

import (
	"context"
	"fmt"
	"strings"

	requestplugin "github.com/nekohy/MeowCLI/plugin"
	"github.com/nekohy/MeowCLI/utils"
)

func (h *Handler) SetPluginRegistry(registry *requestplugin.Registry) {
	if h == nil {
		return
	}
	h.plugins = registry
}

func (h *Handler) runModelPlugins(ctx context.Context, req pluginRequest) ([]byte, error) {
	if h == nil || h.plugins == nil || len(req.EnabledPlugins) == 0 {
		return req.Body, nil
	}
	pluginCtx := requestplugin.NewContext(req.Body)
	pluginCtx.Alias = req.Alias
	pluginCtx.Origin = req.Origin
	pluginCtx.Handler = req.Handler
	pluginCtx.APIType = req.APIType
	pluginCtx.Stream = req.Stream
	return h.plugins.Run(ctx, req.EnabledPlugins, pluginCtx)
}

type pluginRequest struct {
	Alias          string
	Origin         string
	Handler        utils.HandlerType
	APIType        utils.APIType
	Stream         bool
	EnabledPlugins []string
	Body           []byte
}

func parseEnabledPlugins(raw string) []string {
	return requestplugin.ParseList(raw)
}

func pluginFailure(err error) relayError {
	message := "request plugin failed"
	if err != nil {
		message = fmt.Sprintf("%s: %s", message, strings.TrimSpace(err.Error()))
	}
	return relayError{StatusCode: errBridgePluginFailed.StatusCode, Code: errBridgePluginFailed.Code, Message: message}
}
