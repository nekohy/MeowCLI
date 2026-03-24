package codex

import (
	"net/http"
	"strings"
	"unicode"

	"github.com/nekohy/MeowCLI/utils"
)

const (
	planTypeFree       = "free"
	planTypePlus       = "plus"
	planTypeTeam       = "team"
	planTypePro        = "pro"
	planTypeBusiness   = "business"
	planTypeEnterprise = "enterprise"
	planTypeUnknown    = "unknown"
)

var planTypeLabelsByStableCode = map[string]string{
	"0": planTypeFree,
	"1": planTypePlus,
	"2": planTypeTeam,
	"3": planTypePro,
	"4": planTypeBusiness,
	"5": planTypeEnterprise,
	"6": planTypeUnknown,
}

func NormalizePlanType(planType string) string {
	normalized := strings.ToLower(strings.TrimSpace(planType))
	if normalized == "" {
		return ""
	}
	if label, ok := planTypeLabelsByStableCode[normalized]; ok {
		return label
	}
	return normalized
}

func NormalizePlanTypeList(raw string) string {
	return joinPlanTypeList(parsePlanTypeList(raw))
}

func PlanList() []string {
	return []string{
		planTypeFree,
		planTypePlus,
		planTypeTeam,
		planTypePro,
		planTypeBusiness,
		planTypeEnterprise,
		planTypeUnknown,
	}
}

func parsePlanTypeList(raw string) []string {
	if raw == "" {
		return nil
	}

	parts := strings.FieldsFunc(raw, isPlanTypeSeparator)

	planTypes := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		planType := NormalizePlanType(part)
		if planType == "" {
			continue
		}
		if _, ok := seen[planType]; ok {
			continue
		}
		seen[planType] = struct{}{}
		planTypes = append(planTypes, planType)
	}
	return planTypes
}

func isPlanTypeSeparator(r rune) bool {
	return r == ',' || r == ';' || unicode.IsSpace(r)
}

func joinPlanTypeList(planTypes []string) string {
	if len(planTypes) == 0 {
		return ""
	}

	normalized := make([]string, 0, len(planTypes))
	seen := make(map[string]struct{}, len(planTypes))
	for _, planType := range planTypes {
		label := NormalizePlanType(planType)
		if label == "" {
			continue
		}
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		normalized = append(normalized, label)
	}
	return strings.Join(normalized, ",")
}

const planTypeCodeAny = -1

const (
	planTypeCodeFree = iota
	planTypeCodePlus
	planTypeCodeTeam
	planTypeCodePro
	planTypeCodeBusiness
	planTypeCodeEnterprise
)

const planTypeCodeUnknown = 999

var planTypeCodes = map[string]int{
	planTypeFree:       planTypeCodeFree,
	planTypePlus:       planTypeCodePlus,
	planTypeTeam:       planTypeCodeTeam,
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
		codec.codesFor(parsePlanTypeList(snapshot.CodexPreferredPlanTypes)),
	)
}

func headerPlanTypeCodes(headers http.Header, enabled bool, codec *planTypeCodec) []int {
	if !enabled {
		return nil
	}
	return codec.codesFor(parsePlanTypeList(strings.Join(headers.Values(utils.HeaderPlanTypePreference), ",")))
}
