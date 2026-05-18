package utils

import "strings"

const (
	CodeAssistPlanTypeUltra   = "ultra"
	CodeAssistPlanTypePro     = "pro"
	CodeAssistPlanTypeFree    = "free"
	CodeAssistPlanTypeUnknown = "unknown"
)

const projectCredentialIDSeparator = "__"

func DefaultProjectCredentialID(email, projectID string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	projectID = strings.TrimSpace(projectID)
	if email == "" || projectID == "" {
		return ""
	}
	return email + projectCredentialIDSeparator + projectID
}

func ProjectIDFromCredentialID(credentialID string) string {
	id := strings.TrimSpace(credentialID)
	separatorIndex := strings.LastIndex(id, projectCredentialIDSeparator)
	if separatorIndex >= 0 {
		projectID := strings.TrimSpace(id[separatorIndex+len(projectCredentialIDSeparator):])
		if projectID != "" {
			return projectID
		}
	}
	return ""
}

func NormalizeCodeAssistPlanType(planType string) string {
	normalized := strings.ToLower(strings.TrimSpace(planType))
	if normalized == "-" {
		return CodeAssistPlanTypeUnknown
	}
	switch normalized {
	case CodeAssistPlanTypeUltra, CodeAssistPlanTypePro, CodeAssistPlanTypeFree, CodeAssistPlanTypeUnknown:
		return normalized
	default:
		return ""
	}
}

func NormalizeCodeAssistPlanTypeList(raw string) string {
	return JoinNormalizedList(ParseDelimitedList(raw, NormalizeCodeAssistPlanType), NormalizeCodeAssistPlanType)
}

func ParseCodeAssistPlanTypeList(raw string) []string {
	return ParseDelimitedList(raw, NormalizeCodeAssistPlanType)
}

func CodeAssistPlanList() []string {
	return []string{CodeAssistPlanTypeUltra, CodeAssistPlanTypePro, CodeAssistPlanTypeFree, CodeAssistPlanTypeUnknown}
}
