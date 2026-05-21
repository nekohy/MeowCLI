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
			declarationChanged, err := normalizeClaudeFunctionDeclaration(declaration)
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

func normalizeClaudeFunctionDeclaration(declaration *ast.Node) (bool, error) {
	if !astObjectExists(declaration) {
		return false, nil
	}

	parametersChanged, err := ensureClaudeFunctionDeclarationParameters(declaration)
	if err != nil {
		return false, err
	}

	// 清理 parameters 内 Google 不接受的 JSON Schema 关键字
	schemasChanged, err := cleanFunctionDeclarationSchemas(declaration)
	if err != nil {
		return false, err
	}
	return parametersChanged || schemasChanged, nil
}

func ensureClaudeFunctionDeclarationParameters(declaration *ast.Node) (bool, error) {
	// 已经有 parameters 时不再使用 parametersJsonSchema 覆盖
	if astObjectExists(declaration.Get("parameters")) {
		return false, nil
	}

	schema := declaration.Get("parametersJsonSchema")
	if !astObjectExists(schema) {
		return false, nil
	}
	if _, err := declaration.Set("parameters", *schema); err != nil {
		return false, err
	}
	_, err := declaration.Unset("parametersJsonSchema")
	return true, err
}

func cleanFunctionDeclarationSchemas(declaration *ast.Node) (bool, error) {
	schema := declaration.Get("parameters")
	schemaChanged, err := cleanAntigravityGeminiSchema(schema, false)
	if err != nil {
		return false, err
	}
	if !schemaChanged {
		return false, nil
	}
	if _, err := declaration.Set("parameters", *schema); err != nil {
		return false, err
	}
	return true, nil
}

// Antigravity 的 Gemini schema parser 不接受这些 JSON Schema 关键字
var unsupportedGeminiSchemaKeywords = map[string]struct{}{
	"$defs":                {},
	"$id":                  {},
	"$ref":                 {},
	"$schema":              {},
	"additionalProperties": {},
	"default":              {},
	"definitions":          {},
	"deprecated":           {},
	"enumTitles":           {},
	"examples":             {},
	"exclusiveMaximum":     {},
	"exclusiveMinimum":     {},
	"format":               {},
	"maxItems":             {},
	"maxLength":            {},
	"minItems":             {},
	"minLength":            {},
	"nullable":             {},
	"pattern":              {},
	"patternProperties":    {},
	"prefill":              {},
	"propertyNames":        {},
	"title":                {},
	"uniqueItems":          {},
}

func cleanAntigravityGeminiSchema(node *ast.Node, parentIsProperties bool) (bool, error) {
	if node == nil || !node.Exists() {
		return false, nil
	}
	if err := node.Load(); err != nil {
		return false, err
	}

	changed := false
	switch node.TypeSafe() {
	case ast.V_OBJECT:
		// object 分支负责清理当前 schema 节点并递归进入 properties 下的子 schema
		childUpdates := make([]ast.Pair, 0)
		var childErr error
		if err := node.ForEach(func(seq ast.Sequence, child *ast.Node) bool {
			key := *seq.Key
			childChanged, err := cleanAntigravityGeminiSchema(child, key == "properties")
			if err != nil {
				childErr = err
				return false
			}
			if childChanged {
				childUpdates = append(childUpdates, ast.NewPair(key, *child))
			}
			return true
		}); err != nil {
			return false, err
		}
		if childErr != nil {
			return false, childErr
		}
		for _, update := range childUpdates {
			if _, err := node.Set(update.Key, update.Value); err != nil {
				return false, err
			}
			changed = true
		}

		if !parentIsProperties {
			// const 语义接近单值 enum，先转换再删除原字段
			if constNode := node.Get("const"); constNode.Exists() {
				if !node.Get("enum").Exists() {
					if _, err := node.Set("enum", ast.NewArray([]ast.Node{*constNode})); err != nil {
						return false, err
					}
				}
				if removed, err := node.Unset("const"); err != nil {
					return false, err
				} else if removed {
					changed = true
				}
			}

			// properties 的子字段名可能刚好叫 pattern 或 const，不能当作 schema 关键字删除
			for key := range unsupportedGeminiSchemaKeywords {
				if removed, err := node.Unset(key); err != nil {
					return false, err
				} else if removed {
					changed = true
				}
			}

			// Claude input_schema 要求 object schema 显式声明 properties 和 required
			objectShapeChanged, err := ensureObjectSchemaShape(node)
			if err != nil {
				return false, err
			}
			if objectShapeChanged {
				changed = true
			}
		}
	case ast.V_ARRAY:
		// array 分支用于进入 anyOf oneOf allOf 这类数组里的 schema 对象
		for i := 0; i < nodeLen(node); i++ {
			child := node.Index(i)
			childChanged, err := cleanAntigravityGeminiSchema(child, false)
			if err != nil {
				return false, err
			}
			if !childChanged {
				continue
			}
			if _, err := node.SetByIndex(i, *child); err != nil {
				return false, err
			}
			changed = true
		}
	}

	return changed, nil
}

func ensureObjectSchemaShape(node *ast.Node) (bool, error) {
	if astString(node.Get("type")) != "object" {
		return false, nil
	}
	changed := false
	if !astObjectExists(node.Get("properties")) {
		if _, err := node.Set("properties", ast.NewObject(nil)); err != nil {
			return false, err
		}
		changed = true
	}
	if !astStringArrayExists(node.Get("required")) {
		if _, err := node.Set("required", ast.NewArray(nil)); err != nil {
			return false, err
		}
		changed = true
	}
	return changed, nil
}

func astStringArrayExists(node *ast.Node) bool {
	if node == nil || !node.Exists() {
		return false
	}
	if err := node.Load(); err != nil {
		return false
	}
	if node.TypeSafe() != ast.V_ARRAY {
		return false
	}
	for i := 0; i < nodeLen(node); i++ {
		child := node.Index(i)
		if child == nil || !child.Exists() {
			return false
		}
		if err := child.Load(); err != nil {
			return false
		}
		if child.TypeSafe() != ast.V_STRING {
			return false
		}
	}
	return true
}

func astObjectExists(node *ast.Node) bool {
	if node == nil || !node.Exists() {
		return false
	}
	if err := node.Load(); err != nil {
		return false
	}
	return node.TypeSafe() == ast.V_OBJECT
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
