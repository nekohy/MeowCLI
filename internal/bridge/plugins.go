package bridge

import (
	"context"
	"fmt"
	"strings"

	"github.com/bytedance/sonic/ast"
	requestplugin "github.com/nekohy/MeowCLI/plugin"
	"github.com/nekohy/MeowCLI/utils"
)

func (h *Handler) SetPluginRegistry(registry *requestplugin.Registry) {
	if h == nil {
		return
	}
	h.plugins = registry
}

func preparePluginContext(request *requestplugin.Context, alias string, model *ResolvedModel, apiType utils.APIType, stream bool) *requestplugin.Context {
	request.Alias = alias
	request.Origin = model.Origin
	request.Handler = model.Handler
	request.APIType = apiType
	request.Stream = stream
	return request
}

func (h *Handler) runModelPlugins(ctx context.Context, enabled []string, req *requestplugin.Context) (*ast.Node, error) {
	if h.plugins != nil && len(enabled) > 0 {
		if err := h.plugins.Apply(ctx, enabled, req); err != nil {
			return nil, err
		}
	}
	return req.JSON()
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
