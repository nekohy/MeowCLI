package bridge

import (
	"fmt"
	"strings"

	"github.com/nekohy/MeowCLI/utils"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
	"github.com/zeebo/xxh3"
)

// contentElementFingerprint 是一个请求序列元素的进程内快速指纹
// 类型通过派生 seed 隔离，Trie 中只保留 8 字节摘要，不保存原始请求内容
type contentElementFingerprint uint64

// contentFingerprint 按“上下文、tools、对话”的顺序保存元素
// firstDialogue 从 1 开始记录首条对话位置，避免仅上下文相同就粘到同一账号
type contentFingerprint struct {
	elements      []contentElementFingerprint
	firstDialogue int
}

const minimumContentAffinityElements = 4

func (f contentFingerprint) valid() bool {
	return f.firstDialogue > 0 &&
		f.firstDialogue <= len(f.elements) &&
		len(f.elements) >= minimumContentAffinityElements
}

type affinityFeatureKind byte

const (
	affinityFeatureDialogue affinityFeatureKind = 'd'
	affinityFeatureContext  affinityFeatureKind = 'c'
	affinityFeatureTools    affinityFeatureKind = 't'
)

type contentFingerprintBuilder struct {
	seed          uint64
	elements      []contentElementFingerprint
	firstDialogue int
	failed        bool
}

// contentAffinityRequest 将模型名和内容指纹放在一起
// Trie 按模型名隔离，因此不同模型不会共享内容粘性，即使它们属于同一渠道
type contentAffinityRequest struct {
	modelName   string
	fingerprint contentFingerprint
}

// buildContentFingerprint 从已解析的 Sonic AST 构造指纹
func buildContentFingerprint(root *ast.Node, seed uint64, apiType utils.APIType) (contentFingerprint, bool) {
	if root == nil || root.TypeSafe() != ast.V_OBJECT {
		return contentFingerprint{}, false
	}

	builder := &contentFingerprintBuilder{seed: seed}
	builder.collectRoot(root, apiType)
	if builder.failed {
		return contentFingerprint{}, false
	}
	fingerprint := contentFingerprint{
		elements:      builder.elements,
		firstDialogue: builder.firstDialogue,
	}
	return fingerprint, fingerprint.valid()
}

func (b *contentFingerprintBuilder) collectRoot(root *ast.Node, apiType utils.APIType) {
	switch apiType {
	case utils.APIResponses, utils.APIResponsesCompact, utils.APIResponsesWebsocket:
		b.collectResponses(root)
	case utils.APICompletion:
		b.collectChatCompletions(root)
	case utils.APIMessages:
		b.collectAnthropicMessages(root)
	case utils.APIGemini:
		b.collectGemini(root)
	}
}

func (b *contentFingerprintBuilder) collectResponses(root *ast.Node) {
	b.collectWholeValue(root.Get("instructions"), affinityFeatureContext, ast.V_STRING)
	b.collectObjectArrayWhole(root.Get("tools"), affinityFeatureTools)
	b.collectResponsesInput(root.Get("input"))
}

func (b *contentFingerprintBuilder) collectChatCompletions(root *ast.Node) {
	b.collectObjectArrayWhole(root.Get("tools"), affinityFeatureTools)
	b.collectObjectSequence(root.Get("messages"), nil)
}

func (b *contentFingerprintBuilder) collectAnthropicMessages(root *ast.Node) {
	system := root.Get("system")
	switch astNodeType(system) {
	case ast.V_NONE, ast.V_NULL:
	case ast.V_STRING:
		b.appendMeaningfulElement(system, affinityFeatureContext)
	case ast.V_ARRAY:
		normalized, err := marshalArrayWithoutItemCacheControl(system)
		if err != nil {
			b.failed = true
			return
		}
		b.appendNormalizedElement(system, normalized, affinityFeatureContext)
	default:
		b.failed = true
		return
	}

	tools := root.Get("tools")
	toolsType := astNodeType(tools)
	if toolsType != ast.V_NONE && toolsType != ast.V_NULL {
		normalized, err := marshalArrayWithoutItemCacheControl(tools)
		if err != nil {
			b.failed = true
			return
		}
		b.appendNormalizedElement(tools, normalized, affinityFeatureTools)
	}
	b.collectObjectSequence(root.Get("messages"), marshalAnthropicMessageWithoutCacheControl)
}

func (b *contentFingerprintBuilder) collectGemini(root *ast.Node) {
	b.collectWholeValue(root.Get("systemInstruction"), affinityFeatureContext, ast.V_OBJECT)
	b.collectObjectArrayWhole(root.Get("tools"), affinityFeatureTools)
	b.collectObjectSequence(root.Get("contents"), nil)
}

func (b *contentFingerprintBuilder) collectResponsesInput(node *ast.Node) {
	switch astNodeType(node) {
	case ast.V_NONE, ast.V_NULL:
		return
	case ast.V_STRING:
		b.appendMeaningfulElement(node, affinityFeatureDialogue)
	case ast.V_ARRAY:
		b.collectObjectSequence(node, nil)
	default:
		b.failed = true
	}
}

func (b *contentFingerprintBuilder) collectObjectArrayWhole(node *ast.Node, kind affinityFeatureKind) {
	typ := astNodeType(node)
	if typ == ast.V_NONE || typ == ast.V_NULL {
		return
	}
	if typ != ast.V_ARRAY || !validateArrayItems(node, ast.V_OBJECT) {
		b.failed = true
		return
	}
	b.appendMeaningfulElement(node, kind)
}

func (b *contentFingerprintBuilder) collectWholeValue(node *ast.Node, kind affinityFeatureKind, expectedType int) {
	typ := astNodeType(node)
	if typ == ast.V_NONE || typ == ast.V_NULL {
		return
	}
	if typ != expectedType {
		b.failed = true
		return
	}
	b.appendMeaningfulElement(node, kind)
}

func (b *contentFingerprintBuilder) collectObjectSequence(node *ast.Node, normalize func(*ast.Node) ([]byte, error)) {
	typ := astNodeType(node)
	if typ == ast.V_NONE || typ == ast.V_NULL {
		return
	}
	if typ != ast.V_ARRAY {
		b.failed = true
		return
	}
	length, err := node.Len()
	if err != nil {
		b.failed = true
		return
	}
	for index := 0; index < length; index++ {
		item := node.Index(index)
		if astNodeType(item) != ast.V_OBJECT {
			b.failed = true
			return
		}
		if normalize != nil {
			normalized, err := normalize(item)
			if err != nil {
				b.failed = true
				return
			}
			b.appendRawElement(normalized, affinityFeatureDialogue)
			continue
		}
		b.appendMeaningfulElement(item, affinityFeatureDialogue)
		if b.failed {
			return
		}
	}
}

func (b *contentFingerprintBuilder) appendMeaningfulElement(node *ast.Node, kind affinityFeatureKind) {
	empty, err := affinityValueEmpty(node)
	if err != nil {
		b.failed = true
		return
	}
	if empty {
		return
	}
	// 直接对最终请求 AST 的元素编码并哈希，不再从完整请求字节重新解析
	rawJSON, err := node.MarshalJSON()
	if err != nil {
		b.failed = true
		return
	}
	b.appendRawElement(rawJSON, kind)
}

func (b *contentFingerprintBuilder) appendNormalizedElement(original *ast.Node, normalized []byte, kind affinityFeatureKind) {
	empty, err := affinityValueEmpty(original)
	if err != nil {
		b.failed = true
		return
	}
	if empty {
		return
	}
	b.appendRawElement(normalized, kind)
}

func (b *contentFingerprintBuilder) appendRawElement(rawJSON []byte, kind affinityFeatureKind) {
	if kind == affinityFeatureDialogue && b.firstDialogue == 0 {
		b.firstDialogue = len(b.elements) + 1
	}
	b.elements = append(b.elements, elementDigest(b.seed, kind, rawJSON))
}

func affinityValueEmpty(node *ast.Node) (bool, error) {
	switch astNodeType(node) {
	case ast.V_NONE, ast.V_NULL:
		return true, nil
	case ast.V_STRING:
		value, err := node.StrictString()
		return value == "", err
	case ast.V_ARRAY, ast.V_OBJECT:
		length, err := node.Len()
		return length == 0, err
	default:
		return false, nil
	}
}

func marshalArrayWithoutItemCacheControl(node *ast.Node) ([]byte, error) {
	if astNodeType(node) != ast.V_ARRAY {
		return nil, fmt.Errorf("cacheable value must be an array")
	}
	length, err := node.Len()
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, 2+length*32)
	out = append(out, '[')
	for index := 0; index < length; index++ {
		item := node.Index(index)
		if astNodeType(item) != ast.V_OBJECT {
			return nil, fmt.Errorf("cacheable array item %d must be an object", index)
		}
		raw, err := marshalObjectWithoutField(item, "cache_control")
		if err != nil {
			return nil, err
		}
		if index > 0 {
			out = append(out, ',')
		}
		out = append(out, raw...)
	}
	out = append(out, ']')
	return out, nil
}

func marshalAnthropicMessageWithoutCacheControl(node *ast.Node) ([]byte, error) {
	if astNodeType(node) != ast.V_OBJECT {
		return nil, fmt.Errorf("anthropic message must be an object")
	}
	out := []byte{'{'}
	first := true
	contentFound := false
	var visitErr error
	if err := node.ForEach(func(sequence ast.Sequence, child *ast.Node) bool {
		if sequence.Key == nil {
			visitErr = fmt.Errorf("anthropic message field has no key")
			return false
		}
		key := *sequence.Key
		var raw []byte
		var err error
		if key == "content" {
			contentFound = true
			switch astNodeType(child) {
			case ast.V_STRING:
				raw, err = child.MarshalJSON()
			case ast.V_ARRAY:
				raw, err = marshalArrayWithoutItemCacheControl(child)
			default:
				err = fmt.Errorf("anthropic message content must be a string or array")
			}
		} else {
			raw, err = child.MarshalJSON()
		}
		if err != nil {
			visitErr = err
			return false
		}
		out, err = appendJSONObjectField(out, &first, key, raw)
		if err != nil {
			visitErr = err
			return false
		}
		return true
	}); err != nil {
		return nil, err
	}
	if visitErr != nil {
		return nil, visitErr
	}
	if !contentFound {
		return nil, fmt.Errorf("anthropic message content is required")
	}
	out = append(out, '}')
	return out, nil
}

func marshalObjectWithoutField(node *ast.Node, excludedKey string) ([]byte, error) {
	if astNodeType(node) != ast.V_OBJECT {
		return nil, fmt.Errorf("field parent must be an object")
	}
	out := []byte{'{'}
	first := true
	var visitErr error
	if err := node.ForEach(func(sequence ast.Sequence, child *ast.Node) bool {
		if sequence.Key == nil {
			visitErr = fmt.Errorf("object field has no key")
			return false
		}
		key := *sequence.Key
		if key == excludedKey {
			return true
		}
		raw, err := child.MarshalJSON()
		if err != nil {
			visitErr = err
			return false
		}
		out, err = appendJSONObjectField(out, &first, key, raw)
		if err != nil {
			visitErr = err
			return false
		}
		return true
	}); err != nil {
		return nil, err
	}
	if visitErr != nil {
		return nil, visitErr
	}
	out = append(out, '}')
	return out, nil
}

func appendJSONObjectField(out []byte, first *bool, key string, rawValue []byte) ([]byte, error) {
	encodedKey, err := sonic.Marshal(key)
	if err != nil {
		return nil, err
	}
	if !*first {
		out = append(out, ',')
	}
	*first = false
	out = append(out, encodedKey...)
	out = append(out, ':')
	out = append(out, rawValue...)
	return out, nil
}

func validateArrayItems(node *ast.Node, expectedType int) bool {
	if astNodeType(node) != ast.V_ARRAY {
		return false
	}
	length, err := node.Len()
	if err != nil {
		return false
	}
	for index := 0; index < length; index++ {
		if astNodeType(node.Index(index)) != expectedType {
			return false
		}
	}
	return true
}

func astNodeType(node *ast.Node) int {
	if node == nil || !node.Exists() {
		return ast.V_NONE
	}
	if err := node.Load(); err != nil {
		return ast.V_ERROR
	}
	return node.TypeSafe()
}

func elementDigest(seed uint64, kind affinityFeatureKind, rawJSON []byte) contentElementFingerprint {
	// 每种元素使用独立的派生 seed，避免相同 JSON 在上下文、tools 和对话间混用
	const seedMix uint64 = 0x9e3779b97f4a7c15
	kindSeed := seed ^ (uint64(kind) * seedMix)
	return contentElementFingerprint(xxh3.HashSeed(rawJSON, kindSeed))
}

func (h *Handler) buildContentAffinityRequest(modelName string, apiType utils.APIType, root *ast.Node) contentAffinityRequest {
	modelName = strings.TrimSpace(modelName)
	fingerprint, ok := buildContentFingerprint(root, h.contentAffinity.seed, apiType)
	if !ok || modelName == "" {
		return contentAffinityRequest{}
	}
	return contentAffinityRequest{modelName: modelName, fingerprint: fingerprint}
}
