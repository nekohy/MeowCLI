package codex

import (
	"strings"

	"github.com/bytedance/sonic/ast"
)

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
