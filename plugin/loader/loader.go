package loader

import (
	"github.com/nekohy/MeowCLI/plugin"

	_ "github.com/nekohy/MeowCLI/plugin/APIGemini/includethoughts"
	_ "github.com/nekohy/MeowCLI/plugin/HandlerAntigravity/setclaudeThinking"
	_ "github.com/nekohy/MeowCLI/plugin/HandlerCodex/forcefastmode"
	_ "github.com/nekohy/MeowCLI/plugin/HandlerCodex/ignoreservicetier"
)

func DefaultRegistry() *plugin.Registry {
	return plugin.DefaultRegistry()
}
