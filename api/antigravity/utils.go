package antigravity

import (
	"slices"
	"strings"

	"github.com/bytedance/sonic/ast"
	"github.com/google/uuid"
)

func normalizeGeminiRequestForAntigravity(body []byte, modelName string) []byte {
	root, parseErr := ast.NewParser(string(body)).Parse()
	if parseErr != 0 || !astObjectExists(&root) {
		return body
	}

	if _, err := root.Unset("safetySettings"); err != nil {
		return body
	}

	isClaude := strings.Contains(strings.ToLower(modelName), "claude")
	if isClaude {
		// Claude 模型要求 Gemini 工具声明携带 parameters 而不是 parametersJsonSchema
		if err := normalizeClaudeTools(root.Get("tools")); err != nil {
			return body
		}
	}

	// 补齐 functionCall 和 functionResponse 的 Tool ID
	if err := normalizeContents(root.Get("contents"), isClaude); err != nil {
		return body
	}

	out, err := root.MarshalJSON()
	if err != nil {
		return body
	}
	return out
}

func normalizeClaudeTools(tools *ast.Node) error {
	for i := 0; i < nodeLen(tools); i++ {
		declarations := tools.Index(i).Get("functionDeclarations")
		for j := 0; j < nodeLen(declarations); j++ {
			declaration := declarations.Index(j)
			parameters := declaration.Get("parameters")
			// 已经有 parameters 时不再使用 parametersJsonSchema 覆盖
			if !astObjectExists(parameters) {
				if schema := declaration.Get("parametersJsonSchema"); astObjectExists(schema) {
					if _, err := declaration.Set("parameters", *schema); err != nil {
						return err
					}
					if _, err := declaration.Unset("parametersJsonSchema"); err != nil {
						return err
					}
					parameters = declaration.Get("parameters")
				}
			}
			if astObjectExists(parameters) {
				// 清理 parameters 内 Google 不接受的 JSON Schema 关键字
				if err := cleanAntigravityGeminiSchema(parameters, false); err != nil {
					return err
				}
			}
		}
	}
	return nil
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

func cleanAntigravityGeminiSchema(node *ast.Node, propertiesMap bool) error {
	switch astNodeType(node) {
	case ast.V_OBJECT:
		// 先递归清理子 schema，再处理当前 schema 节点自身
		if err := rewriteObjectChildren(node, func(key string, child *ast.Node) error {
			return cleanAntigravityGeminiSchema(child, key == "properties")
		}); err != nil {
			return err
		}
		if !propertiesMap {
			return cleanCurrentSchemaObject(node)
		}
	case ast.V_ARRAY:
		return rewriteArray(node, func(child *ast.Node) error {
			return cleanAntigravityGeminiSchema(child, false)
		})
	}
	return nil
}

func cleanCurrentSchemaObject(node *ast.Node) error {
	if constNode := node.Get("const"); constNode.Exists() {
		// const 语义接近单值 enum，先转换再删除原字段
		if !node.Get("enum").Exists() {
			if _, err := node.Set("enum", ast.NewArray([]ast.Node{*constNode})); err != nil {
				return err
			}
		}
		if _, err := node.Unset("const"); err != nil {
			return err
		}
	}

	if err := flattenStringEnumAnyOf(node); err != nil {
		return err
	}

	// properties 的子字段名可能刚好叫 pattern 或 const，不能当作 schema 关键字删除
	for key := range unsupportedGeminiSchemaKeywords {
		if _, err := node.Unset(key); err != nil {
			return err
		}
	}

	if astString(node.Get("type")) != "object" {
		return nil
	}
	// Claude input_schema 要求 object schema 显式声明 properties 和 required
	if !astObjectExists(node.Get("properties")) {
		if _, err := node.Set("properties", ast.NewObject(nil)); err != nil {
			return err
		}
	}
	if _, ok := astStringArray(node.Get("required")); !ok {
		if _, err := node.Set("required", ast.NewArray(nil)); err != nil {
			return err
		}
	}
	return nil
}

func flattenStringEnumAnyOf(node *ast.Node) error {
	values, ok, err := collectStringEnumUnionValues(node.Get("anyOf"))
	if err != nil || !ok || len(values) == 0 {
		return err
	}

	enumValues := make([]ast.Node, 0, len(values))
	for _, value := range values {
		enumValues = append(enumValues, ast.NewString(value))
	}
	if _, err := node.Set("type", ast.NewString("string")); err != nil {
		return err
	}
	if _, err := node.Set("enum", ast.NewArray(enumValues)); err != nil {
		return err
	}
	_, err = node.Unset("anyOf")
	return err
}

func collectStringEnumUnionValues(union *ast.Node) ([]string, bool, error) {
	if astNodeType(union) != ast.V_ARRAY {
		return nil, false, nil
	}
	n, err := union.Len()
	if err != nil || n == 0 {
		return nil, false, err
	}

	seen := map[string]struct{}{}
	values := make([]string, 0)
	for i := 0; i < n; i++ {
		variant := union.Index(i)
		if astString(variant.Get("type")) != "string" {
			return nil, false, nil
		}

		enumValues, ok := astStringArray(variant.Get("enum"))
		if !ok {
			return nil, false, nil
		}
		for _, value := range enumValues {
			if _, exists := seen[value]; exists {
				continue
			}
			seen[value] = struct{}{}
			values = append(values, value)
		}
	}
	return values, true, nil
}

func normalizeContents(contents *ast.Node, claudeRules bool) error {
	if claudeRules {
		return normalizeClaudeContents(contents)
	}

	pending := pendingFunctionCallIDs{}
	return rewriteArray(contents, func(content *ast.Node) error {
		return ensureContentFunctionCallIDs(content, pending)
	})
}

func normalizeClaudeContents(contents *ast.Node) error {
	n := nodeLen(contents)
	normalized := make([]ast.Node, 0, n)
	for i := 0; i < n; i++ {
		content := contents.Index(i)
		// 跳过可能产生的空 assistant 消息
		keep, err := normalizeClaudeContent(content)
		if err != nil {
			return err
		}
		if keep {
			normalized = append(normalized, *content)
		}
	}

	normalized, err := keepPairedToolParts(normalized)
	if err != nil {
		return err
	}

	// Claude 不支持 assistant message prefill，发送前必须以 user 消息结尾
	for len(normalized) > 0 && astString(normalized[len(normalized)-1].Get("role")) == "model" {
		normalized = normalized[:len(normalized)-1]
	}

	*contents = ast.NewArray(normalized)
	return nil
}

func normalizeClaudeContent(content *ast.Node) (bool, error) {
	parts := content.Get("parts")
	n := nodeLen(parts)
	role := astString(content.Get("role"))
	thoughts := make([]ast.Node, 0)
	regular := make([]ast.Node, 0, n)
	calls := make([]ast.Node, 0)
	for i := 0; i < n; i++ {
		part := parts.Index(i)
		if !partHasPayload(part) {
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

	kept := append(thoughts, regular...)
	kept = append(kept, calls...)
	if len(kept) == 0 {
		return false, nil
	}

	// Antigravity 会按 functionCall 拆分 model 消息
	_, err := content.Set("parts", ast.NewArray(kept))
	return true, err
}

func keepPairedToolParts(contents []ast.Node) ([]ast.Node, error) {
	out := make([]ast.Node, 0, len(contents))
	for i := range contents {
		content := contents[i]

		if astString(content.Get("role")) == "model" && len(functionParts(&content, "functionCall")) > 0 {
			var next *ast.Node
			if i+1 < len(contents) {
				next = &contents[i+1]
			}
			paired, err := ensurePairedFunctionIDs(&content, next)
			if err != nil {
				return nil, err
			}
			if !paired {
				// 没有下一轮 tool_result 就移除 tool_use
				keep, err := filterFunctionParts(&content, "functionCall", nil)
				if err != nil {
					return nil, err
				}
				if !keep {
					continue
				}
			}
		}

		var allowed []string
		if len(out) > 0 {
			allowed = contentFunctionIDs(&out[len(out)-1], "functionCall")
		}
		keep, err := filterFunctionParts(&content, "functionResponse", allowed)
		if err != nil {
			return nil, err
		}
		if !keep {
			continue
		}

		out = append(out, content)
	}
	return out, nil
}

func ensurePairedFunctionIDs(content, next *ast.Node) (bool, error) {
	if next == nil {
		return false, nil
	}

	calls := functionParts(content, "functionCall")
	responses := functionParts(next, "functionResponse")
	used := make([]bool, len(responses))
	for _, call := range calls {
		matched := -1
		for i, response := range responses {
			if used[i] || !functionPartsMatch(call, response) {
				continue
			}
			matched = i
			used[i] = true
			break
		}
		if matched == -1 {
			return false, nil
		}

		id := call.id
		if id == "" {
			id = "toolu_" + uuid.NewString()
			if err := setFunctionPartID(call, "functionCall", id); err != nil {
				return false, err
			}
		}
		if responses[matched].id == "" {
			if err := setFunctionPartID(responses[matched], "functionResponse", id); err != nil {
				return false, err
			}
		}
	}
	return true, nil
}

func functionPartsMatch(call, response functionPart) bool {
	if call.id != "" {
		return response.id == call.id || response.id == "" && call.name != "" && call.name == response.name
	}
	return response.id == "" && call.name != "" && call.name == response.name
}

func setFunctionPartID(part functionPart, kind, id string) error {
	if _, err := part.payload.Set("id", ast.NewString(id)); err != nil {
		return err
	}
	_, err := part.part.Set(kind, *part.payload)
	return err
}

func filterFunctionParts(content *ast.Node, kind string, allowed []string) (bool, error) {
	parts := content.Get("parts")
	n := nodeLen(parts)
	kept := make([]ast.Node, 0, n)
	for i := 0; i < n; i++ {
		part := parts.Index(i)
		functionPart := part.Get(kind)
		if !functionPart.Exists() || allowed != nil && slices.Contains(allowed, strings.TrimSpace(astString(functionPart.Get("id")))) {
			kept = append(kept, *part)
		}
	}
	if len(kept) == 0 {
		return false, nil
	}
	_, err := content.Set("parts", ast.NewArray(kept))
	return true, err
}

func ensureContentFunctionCallIDs(content *ast.Node, pending pendingFunctionCallIDs) error {
	parts := content.Get("parts")
	for i := 0; i < nodeLen(parts); i++ {
		part := parts.Index(i)
		if functionCall := part.Get("functionCall"); functionCall.Exists() {
			if err := ensureFunctionCallID(part, functionCall, pending); err != nil {
				return err
			}
			continue
		}
		if functionResponse := part.Get("functionResponse"); functionResponse.Exists() {
			if err := ensureFunctionResponseID(part, functionResponse, pending); err != nil {
				return err
			}
		}
	}
	return nil
}

func ensureFunctionCallID(part, functionCall *ast.Node, pending pendingFunctionCallIDs) error {
	id := strings.TrimSpace(astString(functionCall.Get("id")))
	if id == "" {
		id = "toolu_" + uuid.NewString()
		if _, err := functionCall.Set("id", ast.NewString(id)); err != nil {
			return err
		}
		if _, err := part.Set("functionCall", *functionCall); err != nil {
			return err
		}
	}
	pending.push(astString(functionCall.Get("name")), id)
	return nil
}

func ensureFunctionResponseID(part, functionResponse *ast.Node, pending pendingFunctionCallIDs) error {
	if id := strings.TrimSpace(astString(functionResponse.Get("id"))); id != "" {
		return nil
	}
	id := pending.pop(astString(functionResponse.Get("name")))
	if id == "" {
		return nil
	}
	if _, err := functionResponse.Set("id", ast.NewString(id)); err != nil {
		return err
	}
	if _, err := part.Set("functionResponse", *functionResponse); err != nil {
		return err
	}
	return nil
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
	ids := make([]string, 0)
	for _, part := range functionParts(content, kind) {
		if part.id != "" {
			ids = append(ids, part.id)
		}
	}
	return ids
}

type functionPart struct {
	part    *ast.Node
	payload *ast.Node
	name    string
	id      string
}

func functionParts(content *ast.Node, kind string) []functionPart {
	parts := content.Get("parts")
	out := make([]functionPart, 0)
	for i := 0; i < nodeLen(parts); i++ {
		part := parts.Index(i)
		payload := part.Get(kind)
		if !payload.Exists() {
			continue
		}
		out = append(out, functionPart{
			part:    part,
			payload: payload,
			name:    strings.TrimSpace(astString(payload.Get("name"))),
			id:      strings.TrimSpace(astString(payload.Get("id"))),
		})
	}
	return out
}

func astObjectExists(node *ast.Node) bool {
	return astNodeType(node) == ast.V_OBJECT
}

func astStringArray(node *ast.Node) ([]string, bool) {
	if astNodeType(node) != ast.V_ARRAY {
		return nil, false
	}
	n := nodeLen(node)
	values := make([]string, 0, n)
	for i := 0; i < n; i++ {
		item := node.Index(i)
		if astNodeType(item) != ast.V_STRING {
			return nil, false
		}
		values = append(values, astString(item))
	}
	return values, true
}

func astNodeType(node *ast.Node) int {
	if node == nil || !node.Exists() {
		return ast.V_NONE
	}
	if err := node.Load(); err != nil {
		return ast.V_NONE
	}
	return node.TypeSafe()
}

func nodeLen(node *ast.Node) int {
	if astNodeType(node) != ast.V_ARRAY {
		return 0
	}
	n, err := node.Len()
	if err != nil {
		return 0
	}
	return n
}

func rewriteArray(node *ast.Node, rewrite func(*ast.Node) error) error {
	for i := 0; i < nodeLen(node); i++ {
		if err := rewrite(node.Index(i)); err != nil {
			return err
		}
	}
	return nil
}

func rewriteObjectChildren(node *ast.Node, rewrite func(string, *ast.Node) error) error {
	var rewriteErr error
	if err := node.ForEach(func(seq ast.Sequence, child *ast.Node) bool {
		if err := rewrite(*seq.Key, child); err != nil {
			rewriteErr = err
			return false
		}
		return true
	}); err != nil {
		return err
	}
	return rewriteErr
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
