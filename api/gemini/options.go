package gemini

import "github.com/nekohy/MeowCLI/utils"

// Options Gemini 后端专有请求参数
type Options struct {
	ModelName string // 解析后的 origin 模型名
	Action    string // generateContent / streamGenerateContent
	RawQuery  string // 原始 URL query string
	ProjectID string // Code Assist project ID
}

func (Options) HandlerType() utils.HandlerType { return utils.HandlerGemini }
