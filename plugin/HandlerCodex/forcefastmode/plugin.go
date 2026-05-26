package forcefastmode

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
		Name:        "codex-force-fast-mode",
		Label:       "强制Fast模式",
		Description: `设置service_tier为priority`,
		Handlers:    []utils.HandlerType{utils.HandlerCodex},
		APITypes: []utils.APIType{
			utils.APIResponses,
			utils.APIResponsesCompact,
			utils.APICompletion,
		},
	}
}

func (Plugin) Apply(_ context.Context, req *plugin.Context) error {
	return setServiceTierPriority(req)
}

func setServiceTierPriority(req *plugin.Context) error {
	root, err := req.JSON()
	if err != nil {
		return err
	}
	if root.TypeSafe() != ast.V_OBJECT {
		return fmt.Errorf("request body must be a JSON object")
	}
	if _, err := root.Set("service_tier", ast.NewString("priority")); err != nil {
		return err
	}
	return nil
}
