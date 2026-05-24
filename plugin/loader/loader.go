package loader

import (
	"github.com/nekohy/MeowCLI/plugin"

	_ "github.com/nekohy/MeowCLI/plugin/APIgemini/includethoughts"
)

func DefaultRegistry() *plugin.Registry {
	return plugin.DefaultRegistry()
}
