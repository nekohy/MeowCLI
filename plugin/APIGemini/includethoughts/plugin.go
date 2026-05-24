package includethoughts

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
		Name:        "gemini-include-thoughts",
		Label:       "启用Gemini思维链",
		Description: "设置includeThoughts为true",
		Handlers:    []utils.HandlerType{utils.HandlerGemini, utils.HandlerAntigravity},
		APITypes:    []utils.APIType{utils.APIGemini},
	}
}

func (Plugin) Apply(_ context.Context, req *plugin.Context) error {
	return forceGeminiIncludeThoughts(req)
}

func forceGeminiIncludeThoughts(req *plugin.Context) error {
	root, err := req.JSON()
	if err != nil {
		return err
	}
	if root.TypeSafe() != ast.V_OBJECT {
		return fmt.Errorf("request body must be a JSON object")
	}

	generationConfig, err := ensureObject(root, "generationConfig")
	if err != nil {
		return err
	}
	thinkingConfig, err := ensureObject(generationConfig, "thinkingConfig")
	if err != nil {
		return err
	}
	if _, err := thinkingConfig.Set("includeThoughts", ast.NewBool(true)); err != nil {
		return err
	}
	return nil
}

func ensureObject(parent *ast.Node, key string) (*ast.Node, error) {
	child := parent.Get(key)
	if child.Exists() {
		if err := child.Load(); err != nil {
			return nil, err
		}
		if child.TypeSafe() == ast.V_OBJECT {
			return child, nil
		}
	}
	if _, err := parent.Set(key, ast.NewObject(nil)); err != nil {
		return nil, err
	}
	return parent.Get(key), nil
}
