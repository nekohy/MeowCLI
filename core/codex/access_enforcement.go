package codex

import (
	"net/http"
	"strings"

	"github.com/bytedance/sonic/ast"
)

const codexInactivePersonalAccessTokenOwnerCode = "biscuit_baker_service_auth_credential_error_status"

func normalizeCodexCredentialStatus(statusCode int, body string) int {
	if isCodexInactivePersonalAccessTokenOwnerError(body) {
		return http.StatusUnauthorized
	}
	return statusCode
}

func isCodexInactivePersonalAccessTokenOwnerError(body string) bool {
	if strings.TrimSpace(body) == "" {
		return false
	}
	var root ast.Node
	if err := root.UnmarshalJSON([]byte(body)); err != nil {
		return false
	}
	status, err := root.Get("status").StrictInt64()
	if err != nil || status != http.StatusForbidden {
		return false
	}
	errorCode, err := root.GetByPath("error", "code").StrictString()
	if err != nil {
		return false
	}
	return errorCode == codexInactivePersonalAccessTokenOwnerCode
}

func isCodexNoMatchingAccessRuleError(body string) bool {
	if strings.TrimSpace(body) == "" {
		return false
	}
	var root ast.Node
	if err := root.UnmarshalJSON([]byte(body)); err != nil {
		return false
	}
	errorType, err := root.GetByPath("error", "type").String()
	if err != nil {
		return false
	}
	errorCode, err := root.GetByPath("error", "code").String()
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(errorType), "rejected_by_access_enforcement") &&
		strings.EqualFold(strings.TrimSpace(errorCode), "no_matching_rule")
}
