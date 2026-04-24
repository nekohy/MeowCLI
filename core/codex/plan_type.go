package codex

import (
	"net/http"
	"strings"

	"github.com/nekohy/MeowCLI/utils"
)

const (
	planTypeFree       = "free"
	planTypePlus       = "plus"
	planTypePro        = "pro"
	planTypeBusiness   = "business"
	planTypeEnterprise = "enterprise"
	planTypeUnknown    = "unknown"
)

func NormalizePlanType(planType string) string {
	normalized := strings.ToLower(strings.TrimSpace(planType))
	switch normalized {
	case planTypeFree, planTypePlus, planTypePro, planTypeBusiness, planTypeEnterprise, planTypeUnknown:
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
	return []string{
		planTypeFree,
		planTypePlus,
		planTypePro,
		planTypeBusiness,
		planTypeEnterprise,
		planTypeUnknown,
	}
}

const planTypeCodeAny = -1

const (
	planTypeCodeFree = iota
	planTypeCodePlus
	planTypeCodePro
	planTypeCodeBusiness
	planTypeCodeEnterprise
)

const planTypeCodeUnknown = 999

var planTypeCodes = map[string]int{
	planTypeFree:       planTypeCodeFree,
	planTypePlus:       planTypeCodePlus,
	planTypePro:        planTypeCodePro,
	planTypeBusiness:   planTypeCodeBusiness,
	planTypeEnterprise: planTypeCodeEnterprise,
	planTypeUnknown:    planTypeCodeUnknown,
}

type planTypeCodec struct{}

func newPlanTypeCodec() *planTypeCodec { return &planTypeCodec{} }

func (c *planTypeCodec) code(planType string) int {
	normalized := NormalizePlanType(planType)
	if normalized == "" {
		return planTypeCodeAny
	}
	if code, ok := planTypeCodes[normalized]; ok {
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
		if code == planTypeCodeAny {
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

func (s *Scheduler) preferredPlanTypeCodes(headers http.Header) []int {
	snapshot := s.settingsSnapshot()
	codec := s.planTypeCodec()

	return mergePlanTypeCodes(
		headerPlanTypeCodes(headers, snapshot.AllowUserPlanTypeHeader && snapshot.CodexAllowUserPlanTypeHeader, codec),
		codec.codesFor(ParsePlanTypeList(snapshot.CodexPreferredPlanTypes)),
	)
}

func headerPlanTypeCodes(headers http.Header, enabled bool, codec *planTypeCodec) []int {
	if !enabled {
		return nil
	}
	return codec.codesFor(ParsePlanTypeList(strings.Join(headers.Values(utils.HeaderPlanTypePreference), ",")))
}
