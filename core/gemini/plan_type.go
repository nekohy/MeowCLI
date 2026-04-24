package gemini

import (
	"strings"

	geminiapi "github.com/nekohy/MeowCLI/api/gemini"
	"github.com/nekohy/MeowCLI/utils"
)

const (
	PlanTypeUltra   = "ultra"
	PlanTypePro     = "pro"
	PlanTypeFree    = "free"
	PlanTypeUnknown = "unknown"
)

func NormalizePlanType(planType string) string {
	normalized := strings.ToLower(strings.TrimSpace(planType))
	if normalized == "-" {
		return PlanTypeUnknown
	}
	switch normalized {
	case PlanTypeUltra, PlanTypePro, PlanTypeFree, PlanTypeUnknown:
		return normalized
	default:
		return ""
	}
}

func NormalizePlanTypeList(raw string) string {
	return utils.JoinNormalizedList(utils.ParseDelimitedList(raw, NormalizePlanType), NormalizePlanType)
}

func ParsePlanTypeList(raw string) []string {
	return utils.ParseDelimitedList(raw, NormalizePlanType)
}

func PlanList() []string {
	return []string{PlanTypeUltra, PlanTypePro, PlanTypeFree, PlanTypeUnknown}
}

// Model tier constants — these represent different Gemini model families
// with separate quota limits, NOT subscription plan types.
// Re-exported from api/gemini to avoid duplication.
const (
	ModelTierPro       = geminiapi.ModelTierPro
	ModelTierFlash     = geminiapi.ModelTierFlash
	ModelTierFlashLite = geminiapi.ModelTierFlashLite
)

// ResolveModelTier determines the quota tier from a Gemini model name.
func ResolveModelTier(modelName string) string {
	m := strings.ToLower(modelName)
	if strings.Contains(m, "flash-lite") {
		return ModelTierFlashLite
	}
	if strings.Contains(m, "flash") {
		return ModelTierFlash
	}
	if strings.Contains(m, "pro") {
		return ModelTierPro
	}
	return ModelTierPro
}

// planTypeCodec maps Gemini plan types to integer codes for efficient
// scheduling lookups and sorted priority selection.
const (
	planTypeCodeFree = iota
	planTypeCodePro
	planTypeCodeUltra
)

const planTypeCodeUnknown = 999

var planTypeCodes = map[string]int{
	PlanTypeFree:    planTypeCodeFree,
	PlanTypePro:     planTypeCodePro,
	PlanTypeUltra:   planTypeCodeUltra,
	PlanTypeUnknown: planTypeCodeUnknown,
}

type planTypeCodec struct{}

func newPlanTypeCodec() *planTypeCodec { return &planTypeCodec{} }

func (c *planTypeCodec) code(planType string) int {
	if code, ok := planTypeCodes[NormalizePlanType(planType)]; ok {
		return code
	}
	return planTypeCodeUnknown
}

func (c *planTypeCodec) codesFor(planTypes []string) []int {
	if len(planTypes) == 0 {
		return nil
	}

	codes := make([]int, 0, len(planTypes))
	seen := make(map[int]struct{}, len(planTypes))
	for _, planType := range planTypes {
		code := c.code(planType)
		if code == planTypeCodeUnknown {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
	}
	return codes
}
