// Package gemini builds Gemini content fingerprints.
package gemini

import (
	"github.com/nekohy/MeowCLI/internal/bridge/contenthash"

	"github.com/bytedance/sonic/ast"
)

type Builder struct{}

var _ contenthash.Protocol = Builder{}

func (Builder) Normalize(_ *ast.Node) error {
	return nil
}

func (Builder) Build(root *ast.Node, seed uint64) (contenthash.Fingerprint, error) {
	collector := contenthash.NewCollector(seed)
	if err := collector.CollectWholeValue(root.Get("systemInstruction"), contenthash.KindContext, ast.V_OBJECT); err != nil {
		return contenthash.Fingerprint{}, err
	}
	if err := collector.CollectObjectArray(root.Get("tools"), contenthash.KindTools); err != nil {
		return contenthash.Fingerprint{}, err
	}
	if err := collector.CollectObjectSequence(root.Get("contents")); err != nil {
		return contenthash.Fingerprint{}, err
	}
	return collector.Fingerprint(), nil
}
