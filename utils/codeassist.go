package utils

import (
	"fmt"
	"strings"

	"github.com/bytedance/sonic/ast"
)

const (
	CodeAssistPlanTypeUltra   = "ultra"
	CodeAssistPlanTypePro     = "pro"
	CodeAssistPlanTypeFree    = "free"
	CodeAssistPlanTypeUnknown = "unknown"
)

const projectCredentialIDSeparator = "__"

// UnwrapRequestEnvelope 返回 Code Assist 信封中的实际请求；标准请求则原样返回。
func UnwrapRequestEnvelope(root *ast.Node) (*ast.Node, error) {
	if root == nil {
		return nil, fmt.Errorf("request JSON is nil")
	}
	if err := root.Load(); err != nil {
		return nil, fmt.Errorf("load request JSON: %w", err)
	}
	if root.TypeSafe() != ast.V_OBJECT {
		return nil, fmt.Errorf("request must be a JSON object")
	}

	request := root.Get("request")
	if !request.Exists() {
		return root, nil
	}
	if request.TypeSafe() != ast.V_OBJECT {
		return nil, fmt.Errorf("request envelope field must be a JSON object")
	}
	return request, nil
}

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
