package antigravity

import "github.com/nekohy/MeowCLI/utils"

// Options carries Code Assist routing details for the Antigravity backend.
type Options struct {
	ModelName   string
	Action      string
	RawQuery    string
	ProjectID   string
	CreditTypes []string
}

func (Options) HandlerType() utils.HandlerType { return utils.HandlerAntigravity }
