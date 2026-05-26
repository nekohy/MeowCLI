package ignoreservicetier

import (
	"context"
	"fmt"

	"github.com/bytedance/sonic/ast"
	"github.com/nekohy/MeowCLI/plugin"
	"github.com/nekohy/MeowCLI/utils"
)

type Plugin struct{}

func init() {
	plugin.Register(Plugin{})
}

func (Plugin) Manifest() plugin.Manifest {
	return plugin.Manifest{
		Name:        "codex-ignore-service-tier",
		Label:       "忽略Service Tier",
		Description: "移除请求体的service_tier",
		Handlers:    []utils.HandlerType{utils.HandlerCodex},
		APITypes: []utils.APIType{
			utils.APIResponses,
			utils.APIResponsesCompact,
			utils.APICompletion,
		},
	}
}

func (Plugin) Apply(_ context.Context, req *plugin.Context) error {
	return removeServiceTier(req)
}

func removeServiceTier(req *plugin.Context) error {
	root, err := req.JSON()
	if err != nil {
		return err
	}
	if root.TypeSafe() != ast.V_OBJECT {
		return fmt.Errorf("request body must be a JSON object")
	}
	if _, err := root.Unset("service_tier"); err != nil {
		return err
	}
	return nil
}
