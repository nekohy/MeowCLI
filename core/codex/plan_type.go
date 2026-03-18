package codex

import (
	"net/http"
	"strings"
	"sync"
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

func IsFreePlanType(planType string) bool {
	return NormalizePlanType(planType) == planTypeFree
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
	planTypeCodeUnknown
)

const planTypeCodeDynamicStart = planTypeCodeUnknown + 1

var defaultPlanTypeCodes = map[string]int{
	planTypeFree:       planTypeCodeFree,
	planTypePlus:       planTypeCodePlus,
	planTypeTeam:       planTypeCodeTeam,
	planTypePro:        planTypeCodePro,
	planTypeBusiness:   planTypeCodeBusiness,
	planTypeEnterprise: planTypeCodeEnterprise,
	planTypeUnknown:    planTypeCodeUnknown,
}

type planTypeCodec struct {
	mu    sync.RWMutex
	codes map[string]int
	next  int
}

func newPlanTypeCodec() *planTypeCodec {
	codec := &planTypeCodec{
		codes: make(map[string]int, len(defaultPlanTypeCodes)),
		next:  planTypeCodeDynamicStart,
	}
	for planType, code := range defaultPlanTypeCodes {
		codec.codes[planType] = code
	}
	return codec
}

func (c *planTypeCodec) code(planType string) int {
	normalized := NormalizePlanType(planType)
	if normalized == "" {
		return planTypeCodeAny
	}

	c.mu.RLock()
	code, ok := c.codes[normalized]
	c.mu.RUnlock()
	if ok {
		return code
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if code, ok = c.codes[normalized]; ok {
		return code
	}

	code = c.next
	c.codes[normalized] = code
	c.next++
	return code
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
