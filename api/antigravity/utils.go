package antigravity

import (
	"strings"

	"github.com/bytedance/sonic/ast"
	"github.com/google/uuid"
)

func normalizeGeminiRequestForAntigravity(body []byte, modelName string) []byte {
	root, parseErr := ast.NewParser(string(body)).Parse()
	if parseErr != 0 || !astObjectExists(&root) {
		return body
	}

	isClaude := strings.Contains(strings.ToLower(modelName), "claude")
	changed := false

	// 去掉 Antigravity 不接受的顶层安全设置
	if removed, err := root.Unset("safetySettings"); err != nil {
		return body
	} else if removed {
		changed = true
	}

	if isClaude {
		// Claude 模型要求 Gemini 工具声明携带 parameters 而不是 parametersJsonSchema
		tools := root.Get("tools")
		toolsChanged, err := normalizeClaudeTools(tools)
		if err != nil {
			return body
		}
		if toolsChanged {
			if _, err := root.Set("tools", *tools); err != nil {
				return body
			}
			changed = true
		}
	}

	contents := root.Get("contents")
	// 补齐 functionCall 和 functionResponse 的 Tool ID
	idsChanged, err := ensureGeminiFunctionCallIDs(contents)
	if err != nil {
		return body
	}
	if idsChanged {
		if _, err := root.Set("contents", *contents); err != nil {
			return body
		}
		changed = true
	}

	if !changed {
		return body
	}
	out, err := root.MarshalJSON()
	if err != nil {
		return body
	}
	return out
}

func normalizeClaudeTools(tools *ast.Node) (bool, error) {
	changed := false
	for i := 0; i < nodeLen(tools); i++ {
		tool := tools.Index(i)
		functionDeclarations := tool.Get("functionDeclarations")
		declarationsChanged := false
		for j := 0; j < nodeLen(functionDeclarations); j++ {
			declaration := functionDeclarations.Index(j)
			declarationChanged, err := ensureClaudeFunctionDeclarationParameters(declaration)
			if err != nil {
				return false, err
			}
			if !declarationChanged {
				continue
			}
			if _, err := functionDeclarations.SetByIndex(j, *declaration); err != nil {
				return false, err
			}
			declarationsChanged = true
		}
		if !declarationsChanged {
			continue
		}
		if _, err := tool.Set("functionDeclarations", *functionDeclarations); err != nil {
			return false, err
		}
		if _, err := tools.SetByIndex(i, *tool); err != nil {
			return false, err
		}
		changed = true
	}
	return changed, nil
}

func ensureClaudeFunctionDeclarationParameters(declaration *ast.Node) (bool, error) {
	if !astObjectExists(declaration) {
		return false, nil
	}

	if astObjectExists(declaration.Get("parameters")) {
		return false, nil
	}
	schema := declaration.Get("parametersJsonSchema")
	if !astObjectExists(schema) {
		_, err := declaration.Set("parameters", emptyObjectInputSchemaNode())
		return true, err
	}
	_, err := declaration.Set("parameters", *schema)
	return true, err
}

func astObjectExists(node *ast.Node) bool {
	if node == nil || !node.Exists() {
		return false
	}
	if err := node.Load(); err != nil {
		return false
	}
	return node.Type() == ast.V_OBJECT
}

func emptyObjectInputSchemaNode() ast.Node {
	return ast.NewObject([]ast.Pair{
		ast.NewPair("type", ast.NewString("object")),
		ast.NewPair("properties", ast.NewObject(nil)),
	})
}

type pendingFunctionCallIDs map[string][]string

func ensureGeminiFunctionCallIDs(contents *ast.Node) (bool, error) {
	changed := false
	pending := pendingFunctionCallIDs{}

	for i := 0; i < nodeLen(contents); i++ {
		content := contents.Index(i)
		contentChanged, err := ensureContentFunctionCallIDs(content, pending)
		if err != nil {
			return false, err
		}
		if contentChanged {
			if _, err := contents.SetByIndex(i, *content); err != nil {
				return false, err
			}
			changed = true
		}
	}
	return changed, nil
}

func ensureContentFunctionCallIDs(content *ast.Node, pending pendingFunctionCallIDs) (bool, error) {
	parts := content.Get("parts")
	changed := false
	for i := 0; i < nodeLen(parts); i++ {
		part := parts.Index(i)
		partChanged := false
		if functionCall := part.Get("functionCall"); functionCall.Exists() {
			changedCall, err := ensureFunctionCallID(part, functionCall, pending)
			if err != nil {
				return false, err
			}
			partChanged = changedCall
		} else if functionResponse := part.Get("functionResponse"); functionResponse.Exists() {
			changedResponse, err := ensureFunctionResponseID(part, functionResponse, pending)
			if err != nil {
				return false, err
			}
			partChanged = changedResponse
		}
		if !partChanged {
			continue
		}
		if _, err := parts.SetByIndex(i, *part); err != nil {
			return false, err
		}
		changed = true
	}
	if !changed {
		return false, nil
	}
	if _, err := content.Set("parts", *parts); err != nil {
		return false, err
	}
	return true, nil
}

func ensureFunctionCallID(part, functionCall *ast.Node, pending pendingFunctionCallIDs) (bool, error) {
	id := strings.TrimSpace(astString(functionCall.Get("id")))
	changed := false
	if id == "" {
		id = "toolu_" + uuid.NewString()
		if _, err := functionCall.Set("id", ast.NewString(id)); err != nil {
			return false, err
		}
		changed = true
	}
	pending.push(astString(functionCall.Get("name")), id)
	if !changed {
		return false, nil
	}
	if _, err := part.Set("functionCall", *functionCall); err != nil {
		return false, err
	}
	return true, nil
}

func ensureFunctionResponseID(part, functionResponse *ast.Node, pending pendingFunctionCallIDs) (bool, error) {
	if id := strings.TrimSpace(astString(functionResponse.Get("id"))); id != "" {
		return false, nil
	}
	id := pending.pop(astString(functionResponse.Get("name")))
	if id == "" {
		return false, nil
	}
	if _, err := functionResponse.Set("id", ast.NewString(id)); err != nil {
		return false, err
	}
	if _, err := part.Set("functionResponse", *functionResponse); err != nil {
		return false, err
	}
	return true, nil
}

func (pending pendingFunctionCallIDs) push(name, id string) {
	name = strings.TrimSpace(name)
	if name == "" || id == "" {
		return
	}
	pending[name] = append(pending[name], id)
}

func (pending pendingFunctionCallIDs) pop(name string) string {
	name = strings.TrimSpace(name)
	ids := pending[name]
	if len(ids) == 0 {
		return ""
	}
	id := ids[0]
	pending[name] = ids[1:]
	return id
}

func nodeLen(node *ast.Node) int {
	if node == nil {
		return 0
	}
	if err := node.Load(); err != nil {
		return 0
	}
	n, err := node.Len()
	if err != nil {
		return 0
	}
	return n
}

func astString(node *ast.Node) string {
	if node == nil {
		return ""
	}
	value, err := node.String()
	if err != nil {
		return ""
	}
	return value
}
