package bridge

import (
	"strings"

	"github.com/nekohy/MeowCLI/internal/bridge/contenthash"
	"github.com/nekohy/MeowCLI/internal/bridge/contenthash/completion"
	"github.com/nekohy/MeowCLI/internal/bridge/contenthash/gemini"
	"github.com/nekohy/MeowCLI/internal/bridge/contenthash/messages"
	"github.com/nekohy/MeowCLI/internal/bridge/contenthash/responses"
	"github.com/nekohy/MeowCLI/utils"

	"github.com/bytedance/sonic/ast"
)

// contentElementFingerprint 是一个请求序列元素的进程内快速指纹。
// 类型通过派生 seed 隔离，Trie 中只保留 8 字节摘要，不保存原始请求内容。
type contentElementFingerprint = contenthash.Element

// contentHash 按“上下文、tools、对话”的顺序保存元素。
// firstDialogue 从 1 开始记录首条对话位置，避免仅上下文相同就粘到同一账号。
type contentHash struct {
	elements      []contentElementFingerprint
	firstDialogue int
}

const minimumContentAffinityElements = 4

func (f contentHash) valid() bool {
	return f.firstDialogue > 0 &&
		f.firstDialogue <= len(f.elements) &&
		len(f.elements) >= minimumContentAffinityElements
}

// contentAffinityRequest 将模型名和内容指纹放在一起。
// Trie 按模型名隔离，因此不同模型不会共享内容粘性，即使它们属于同一渠道。
type contentAffinityRequest struct {
	modelName   string
	fingerprint contentHash
}

var contentHashProtocols = map[utils.APIType]contenthash.Protocol{
	utils.APIResponses:          responses.Builder{},
	utils.APIResponsesCompact:   responses.Builder{},
	utils.APIResponsesWebsocket: responses.Builder{},
	utils.APICompletion:         completion.Builder{},
	utils.APIMessages:           messages.Builder{},
	utils.APIGemini:             gemini.Builder{},
}

// buildContentHash 从已解析的 Sonic AST 构造内容哈希。
func buildContentHash(root *ast.Node, seed uint64, apiType utils.APIType) (contentHash, bool) {
	if root == nil || root.TypeSafe() != ast.V_OBJECT {
		return contentHash{}, false
	}
	protocol := contentHashProtocols[apiType]
	if protocol == nil {
		return contentHash{}, false
	}

	built, err := contenthash.Build(root, seed, protocol)
	if err != nil {
		return contentHash{}, false
	}
	hash := contentHash{
		elements:      built.Elements,
		firstDialogue: built.FirstDialogue,
	}
	return hash, hash.valid()
}

func (h *Handler) buildContentAffinityRequest(modelName string, apiType utils.APIType, root *ast.Node) contentAffinityRequest {
	modelName = strings.TrimSpace(modelName)
	fingerprint, ok := buildContentHash(root, h.contentAffinity.seed, apiType)
	if !ok || modelName == "" {
		return contentAffinityRequest{}
	}
	return contentAffinityRequest{modelName: modelName, fingerprint: fingerprint}
}
