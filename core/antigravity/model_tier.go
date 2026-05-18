package antigravity

import (
	"strings"

	antigravityapi "github.com/nekohy/MeowCLI/api/antigravity"
)

const (
	ModelTierClaude    = antigravityapi.ModelTierClaude
	ModelTierPro       = antigravityapi.ModelTierPro
	ModelTierFlash     = antigravityapi.ModelTierFlash
	ModelTierFlashLite = antigravityapi.ModelTierFlashLite
	ModelTierTab       = antigravityapi.ModelTierTab
	ModelTierImage     = antigravityapi.ModelTierImage
)

func ResolveModelTier(modelName string) string {
	m := strings.ToLower(strings.TrimSpace(modelName))
	if strings.HasPrefix(m, "tab_") || strings.Contains(m, "tab_jump") {
		return ModelTierTab
	}
	if strings.Contains(m, "image") || strings.Contains(m, "imagen") {
		return ModelTierImage
	}
	if strings.Contains(m, "claude") || strings.Contains(m, "anthropic") || strings.Contains(m, "gpt") || strings.Contains(m, "openai") {
		return ModelTierClaude
	}
	if strings.Contains(m, "flash-lite") || strings.Contains(m, "flash_lite") || strings.Contains(m, "flashlite") {
		return ModelTierFlashLite
	}
	if strings.Contains(m, "flash") {
		return ModelTierFlash
	}
	if strings.Contains(m, "pro") {
		return ModelTierPro
	}
	return ModelTierClaude
}
