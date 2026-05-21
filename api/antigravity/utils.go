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
	contentsChanged, err := normalizeContents(contents, isClaude)
	if err != nil {
		return body
	}
	if contentsChanged {
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
	return rewriteArray(tools, func(tool *ast.Node) (bool, error) {
		declarations := tool.Get("functionDeclarations")
		changed, err := rewriteArray(declarations, normalizeClaudeFunctionDeclaration)
		if err != nil || !changed {
			return changed, err
		}
		_, err = tool.Set("functionDeclarations", *declarations)
		return true, err
	})
}

func normalizeClaudeFunctionDeclaration(declaration *ast.Node) (bool, error) {
	if !astObjectExists(declaration) {
		return false, nil
	}

	changed := false
	// 已经有 parameters 时不再使用 parametersJsonSchema 覆盖
	if !astObjectExists(declaration.Get("parameters")) {
		if schema := declaration.Get("parametersJsonSchema"); astObjectExists(schema) {
			if _, err := declaration.Set("parameters", *schema); err != nil {
				return false, err
			}
			if _, err := declaration.Unset("parametersJsonSchema"); err != nil {
				return false, err
			}
			changed = true
		}
	}

	parameters := declaration.Get("parameters")
	// 清理 parameters 内 Google 不接受的 JSON Schema 关键字
	schemaChanged, err := cleanAntigravityGeminiSchema(parameters, false)
	if err != nil || !schemaChanged {
		return changed, err
	}
	if _, err := declaration.Set("parameters", *parameters); err != nil {
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

func cleanAntigravityGeminiSchema(node *ast.Node, propertiesMap bool) (bool, error) {
	nodeType, ok, err := astNodeType(node)
	if err != nil || !ok {
		return false, err
	}

	changed := false
	switch nodeType {
	case ast.V_OBJECT:
		// 先递归清理子 schema，再处理当前 schema 节点自身
		childrenChanged, err := rewriteObjectChildren(node, func(key string, child *ast.Node) (bool, error) {
			return cleanAntigravityGeminiSchema(child, key == "properties")
		})
		if err != nil {
			return false, err
		}
		changed = childrenChanged

		if propertiesMap {
			return changed, nil
		}
		schemaChanged, err := cleanCurrentSchemaObject(node)
		if err != nil {
			return false, err
		}
		return changed || schemaChanged, nil
	case ast.V_ARRAY:
		return rewriteArray(node, func(child *ast.Node) (bool, error) {
			return cleanAntigravityGeminiSchema(child, false)
		})
	default:
		return false, nil
	}
}

func cleanCurrentSchemaObject(node *ast.Node) (bool, error) {
	changed := false
	if constNode := node.Get("const"); constNode.Exists() {
		// const 语义接近单值 enum，先转换再删除原字段
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

	unionChanged, err := flattenStringEnumAnyOf(node)
	if err != nil {
		return false, err
	}
	changed = changed || unionChanged

	// properties 的子字段名可能刚好叫 pattern 或 const，不能当作 schema 关键字删除
	for key := range unsupportedGeminiSchemaKeywords {
		removed, err := node.Unset(key)
		if err != nil {
			return false, err
		}
		changed = changed || removed
	}

	if astString(node.Get("type")) != "object" {
		return changed, nil
	}
	// Claude input_schema 要求 object schema 显式声明 properties 和 required
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

func flattenStringEnumAnyOf(node *ast.Node) (bool, error) {
	anyOf := node.Get("anyOf")
	values, ok, err := collectStringEnumUnionValues(anyOf)
	if err != nil || !ok || len(values) == 0 {
		return false, err
	}

	enumValues := make([]ast.Node, 0, len(values))
	for _, value := range values {
		enumValues = append(enumValues, ast.NewString(value))
	}
	if _, err := node.Set("type", ast.NewString("string")); err != nil {
		return false, err
	}
	if _, err := node.Set("enum", ast.NewArray(enumValues)); err != nil {
		return false, err
	}
	if _, err := node.Unset("anyOf"); err != nil {
		return false, err
	}
	return true, nil
}

func collectStringEnumUnionValues(union *ast.Node) ([]string, bool, error) {
	variants, ok, err := astArrayItems(union)
	if err != nil || !ok {
		return nil, false, err
	}

	seen := map[string]struct{}{}
	values := make([]string, 0)
	for i := range variants {
		variant := &variants[i]
		if !astObjectExists(variant) || astString(variant.Get("type")) != "string" {
			return nil, false, nil
		}

		enumItems, ok, err := astArrayItems(variant.Get("enum"))
		if err != nil || !ok {
			return nil, false, err
		}
		for j := range enumItems {
			valueNode := &enumItems[j]
			nodeType, ok, err := astNodeType(valueNode)
			if err != nil || !ok || nodeType != ast.V_STRING {
				return nil, false, err
			}
			value := astString(valueNode)
			if _, exists := seen[value]; exists {
				continue
			}
			seen[value] = struct{}{}
			values = append(values, value)
		}
	}
	return values, true, nil
}

func normalizeContents(contents *ast.Node, claudeRules bool) (bool, error) {
	pending := pendingFunctionCallIDs{}
	changed, err := rewriteArray(contents, func(content *ast.Node) (bool, error) {
		return ensureContentFunctionCallIDs(content, pending)
	})
	if err != nil || !claudeRules {
		return changed, err
	}

	claudeChanged, err := normalizeClaudeContents(contents)
	return changed || claudeChanged, err
}

func normalizeClaudeContents(contents *ast.Node) (bool, error) {
	items, ok, err := astArrayItems(contents)
	if err != nil || !ok {
		return false, err
	}

	changed := false
	normalized := make([]ast.Node, 0, len(items))
	for i := range items {
		content := &items[i]
		// 跳过 web 模式可能产生的空 assistant 消息
		contentChanged, keep, err := normalizeClaudeContent(content)
		if err != nil {
			return false, err
		}
		changed = changed || contentChanged || !keep
		if keep {
			normalized = append(normalized, *content)
		}
	}

	paired, pairingChanged, err := keepPairedToolParts(normalized)
	if err != nil {
		return false, err
	}
	normalized = paired
	changed = changed || pairingChanged

	// Claude 不支持 assistant message prefill，发送前必须以 user 消息结尾
	for len(normalized) > 0 && astString(normalized[len(normalized)-1].Get("role")) == "model" {
		normalized = normalized[:len(normalized)-1]
		changed = true
	}

	if !changed {
		return false, nil
	}
	*contents = ast.NewArray(normalized)
	return true, nil
}

func normalizeClaudeContent(content *ast.Node) (bool, bool, error) {
	parts, ok, err := astArrayItems(content.Get("parts"))
	if err != nil || !ok {
		return false, false, err
	}

	role := astString(content.Get("role"))
	changed := false
	thoughts := make([]ast.Node, 0)
	regular := make([]ast.Node, 0, len(parts))
	calls := make([]ast.Node, 0)
	for i := range parts {
		part := &parts[i]
		if !partHasPayload(part) {
			changed = true
			continue
		}
		switch {
		case role == "model" && part.Get("functionCall").Exists():
			calls = append(calls, *part)
		case role == "model" && isSignedThoughtPart(part):
			thoughts = append(thoughts, *part)
		default:
			regular = append(regular, *part)
		}
	}

	kept := make([]ast.Node, 0, len(thoughts)+len(regular)+len(calls))
	kept = append(kept, thoughts...)
	kept = append(kept, regular...)
	kept = append(kept, calls...)
	if len(kept) == 0 {
		return true, false, nil
	}

	if role == "model" && len(calls) > 0 && len(regular) > 0 {
		// Antigravity 会按 functionCall 拆分 model 消息
		changed = true
	}
	if !changed {
		return false, true, nil
	}
	_, err = content.Set("parts", ast.NewArray(kept))
	return true, true, err
}

func keepPairedToolParts(contents []ast.Node) ([]ast.Node, bool, error) {
	changed := false
	out := make([]ast.Node, 0, len(contents))
	for i := range contents {
		content := contents[i]

		callIDs := contentFunctionIDs(&content, "functionCall")
		if astString(content.Get("role")) == "model" && len(callIDs) > 0 {
			nextResponseIDs := map[string]struct{}{}
			if i+1 < len(contents) {
				nextResponseIDs = idSet(contentFunctionIDs(&contents[i+1], "functionResponse"))
			}
			if !idsCovered(callIDs, nextResponseIDs) {
				// 没有下一轮 tool_result 就移除 tool_use
				contentChanged, keep, err := filterContentParts(&content, func(part *ast.Node) bool {
					return !part.Get("functionCall").Exists()
				})
				if err != nil {
					return nil, false, err
				}
				changed = changed || contentChanged || !keep
				if !keep {
					continue
				}
			}
		}

		responseIDs := contentFunctionIDs(&content, "functionResponse")
		if len(responseIDs) > 0 {
			allowed := map[string]struct{}{}
			if len(out) > 0 {
				allowed = idSet(contentFunctionIDs(&out[len(out)-1], "functionCall"))
			}
			contentChanged, keep, err := filterContentParts(&content, func(part *ast.Node) bool {
				functionResponse := part.Get("functionResponse")
				if !functionResponse.Exists() {
					return true
				}
				_, ok := allowed[strings.TrimSpace(astString(functionResponse.Get("id")))]
				return ok
			})
			if err != nil {
				return nil, false, err
			}
			changed = changed || contentChanged || !keep
			if !keep {
				continue
			}
		}

		out = append(out, content)
	}
	return out, changed, nil
}

func filterContentParts(content *ast.Node, keep func(*ast.Node) bool) (bool, bool, error) {
	parts, ok, err := astArrayItems(content.Get("parts"))
	if err != nil || !ok {
		return false, true, err
	}

	kept := make([]ast.Node, 0, len(parts))
	for i := range parts {
		if keep(&parts[i]) {
			kept = append(kept, parts[i])
		}
	}
	if len(kept) == len(parts) {
		return false, len(kept) > 0, nil
	}
	if len(kept) == 0 {
		return true, false, nil
	}
	_, err = content.Set("parts", ast.NewArray(kept))
	return true, true, err
}

func ensureContentFunctionCallIDs(content *ast.Node, pending pendingFunctionCallIDs) (bool, error) {
	parts := content.Get("parts")
	changed, err := rewriteArray(parts, func(part *ast.Node) (bool, error) {
		if functionCall := part.Get("functionCall"); functionCall.Exists() {
			return ensureFunctionCallID(part, functionCall, pending)
		}
		if functionResponse := part.Get("functionResponse"); functionResponse.Exists() {
			return ensureFunctionResponseID(part, functionResponse, pending)
		}
		return false, nil
	})
	if err != nil || !changed {
		return changed, err
	}
	_, err = content.Set("parts", *parts)
	return true, err
}

func ensureFunctionCallID(part, functionCall *ast.Node, pending pendingFunctionCallIDs) (bool, error) {
	id := strings.TrimSpace(astString(functionCall.Get("id")))
	changed := false
	if id == "" {
		id = "toolu_" + uuid.NewString()
		if _, err := functionCall.Set("id", ast.NewString(id)); err != nil {
			return false, err
		}
		if _, err := part.Set("functionCall", *functionCall); err != nil {
			return false, err
		}
		changed = true
	}
	pending.push(astString(functionCall.Get("name")), id)
	return changed, nil
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

type pendingFunctionCallIDs map[string][]string

func (pending pendingFunctionCallIDs) push(name, id string) {
	name = strings.TrimSpace(name)
	if name != "" && id != "" {
		pending[name] = append(pending[name], id)
	}
}

func (pending pendingFunctionCallIDs) pop(name string) string {
	name = strings.TrimSpace(name)
	ids := pending[name]
	if len(ids) == 0 {
		return ""
	}
	pending[name] = ids[1:]
	return ids[0]
}

func partHasPayload(part *ast.Node) bool {
	if part.Get("thought").Exists() {
		return isSignedThoughtPart(part)
	}
	return strings.TrimSpace(astString(part.Get("text"))) != "" ||
		part.Get("functionCall").Exists() ||
		part.Get("functionResponse").Exists() ||
		part.Get("inlineData").Exists()
}

func isSignedThoughtPart(part *ast.Node) bool {
	return part.Get("thought").Exists() &&
		strings.TrimSpace(astString(part.Get("text"))) != "" &&
		strings.TrimSpace(astString(part.Get("thoughtSignature"))) != ""
}

func contentFunctionIDs(content *ast.Node, kind string) []string {
	parts, ok, _ := astArrayItems(content.Get("parts"))
	if !ok {
		return nil
	}

	ids := make([]string, 0)
	for i := range parts {
		id := strings.TrimSpace(astString(parts[i].Get(kind).Get("id")))
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func idsCovered(ids []string, set map[string]struct{}) bool {
	for _, id := range ids {
		if _, ok := set[id]; !ok {
			return false
		}
	}
	return true
}

func idSet(ids []string) map[string]struct{} {
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}

func astStringArrayExists(node *ast.Node) bool {
	items, ok, _ := astArrayItems(node)
	if !ok {
		return false
	}
	for i := range items {
		nodeType, ok, _ := astNodeType(&items[i])
		if !ok || nodeType != ast.V_STRING {
			return false
		}
	}
	return true
}

func astObjectExists(node *ast.Node) bool {
	nodeType, ok, _ := astNodeType(node)
	return ok && nodeType == ast.V_OBJECT
}

func astArrayItems(node *ast.Node) ([]ast.Node, bool, error) {
	nodeType, ok, err := astNodeType(node)
	if err != nil || !ok || nodeType != ast.V_ARRAY {
		return nil, false, err
	}
	n, err := node.Len()
	if err != nil {
		return nil, false, err
	}
	items := make([]ast.Node, 0, n)
	for i := 0; i < n; i++ {
		items = append(items, *node.Index(i))
	}
	return items, true, nil
}

func astNodeType(node *ast.Node) (int, bool, error) {
	if node == nil || !node.Exists() {
		return ast.V_NONE, false, nil
	}
	if err := node.Load(); err != nil {
		return ast.V_NONE, false, err
	}
	return node.TypeSafe(), true, nil
}

func rewriteArray(node *ast.Node, rewrite func(*ast.Node) (bool, error)) (bool, error) {
	items, ok, err := astArrayItems(node)
	if err != nil || !ok {
		return false, err
	}

	changed := false
	for i := range items {
		childChanged, err := rewrite(&items[i])
		if err != nil {
			return false, err
		}
		if !childChanged {
			continue
		}
		if _, err := node.SetByIndex(i, items[i]); err != nil {
			return false, err
		}
		changed = true
	}
	return changed, nil
}

func rewriteObjectChildren(node *ast.Node, rewrite func(string, *ast.Node) (bool, error)) (bool, error) {
	updates := make([]ast.Pair, 0)
	var rewriteErr error
	if err := node.ForEach(func(seq ast.Sequence, child *ast.Node) bool {
		changed, err := rewrite(*seq.Key, child)
		if err != nil {
			rewriteErr = err
			return false
		}
		if changed {
			updates = append(updates, ast.NewPair(*seq.Key, *child))
		}
		return true
	}); err != nil {
		return false, err
	}
	if rewriteErr != nil {
		return false, rewriteErr
	}
	for _, update := range updates {
		if _, err := node.Set(update.Key, update.Value); err != nil {
			return false, err
		}
	}
	return len(updates) > 0, nil
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
