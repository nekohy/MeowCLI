package gemini

import (
	"net/http"
	"strings"

	"github.com/nekohy/MeowCLI/utils"
)

const (
	PlanTypeUltra = "ultra"
	PlanTypePro   = "pro"
	PlanTypeFree  = "free"
)

func NormalizePlanType(planType string) string {
	normalized := strings.ToLower(strings.TrimSpace(planType))
	switch normalized {
	case PlanTypeUltra, PlanTypePro, PlanTypeFree:
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
	return []string{PlanTypeUltra, PlanTypePro, PlanTypeFree}
}

func DefaultPreferredPlanTypes() []string {
	return append([]string(nil), PlanList()...)
}

func ResolvePreferredPlanTypes(headers http.Header, allowedPlanTypes []string) []string {
	preferred := append([]string(nil), allowedPlanTypes...)
	if len(preferred) == 0 {
		preferred = DefaultPreferredPlanTypes()
	}

	rawHeader := strings.Join(headers[utils.HeaderPlanTypePreference], ",")
	headerPlanTypes := ParsePlanTypeList(rawHeader)
	if len(headerPlanTypes) == 0 {
		return preferred
	}

	merged := make([]string, 0, len(headerPlanTypes)+len(preferred))
	seen := make(map[string]struct{}, len(headerPlanTypes)+len(preferred))
	for _, planType := range headerPlanTypes {
		if _, ok := seen[planType]; ok {
			continue
		}
		seen[planType] = struct{}{}
		merged = append(merged, planType)
	}
	for _, planType := range preferred {
		if _, ok := seen[planType]; ok {
			continue
		}
		seen[planType] = struct{}{}
		merged = append(merged, planType)
	}
	return merged
}
