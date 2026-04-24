package codex

import "strings"

const (
	ModelTierDefault = "default"
	ModelTierSpark   = "spark"
)

// ResolveModelTier determines the quota tier from a Codex model name.
func ResolveModelTier(modelName string) string {
	m := strings.ToLower(modelName)
	if strings.Contains(m, "spark") {
		return ModelTierSpark
	}
	return ModelTierDefault
}
