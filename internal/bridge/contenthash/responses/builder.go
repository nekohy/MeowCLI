// Package responses builds Responses API content fingerprints.
package responses

import (
	"fmt"

	"github.com/nekohy/MeowCLI/internal/bridge/contenthash"

	"github.com/bytedance/sonic/ast"
)

type Builder struct{}

var _ contenthash.Protocol = Builder{}

func (Builder) Normalize(_ *ast.Node) error {
	return nil
}

func (Builder) Collect(root *ast.Node, collector *contenthash.Collector) error {
	if err := collector.CollectWholeValue(root.Get("instructions"), contenthash.KindContext, ast.V_STRING); err != nil {
		return err
	}
	if err := collector.CollectObjectArray(root.Get("tools"), contenthash.KindTools); err != nil {
		return err
	}
	if err := collectInput(collector, root.Get("input")); err != nil {
		return err
	}
	return nil
}

func collectInput(collector *contenthash.Collector, node *ast.Node) error {
	switch contenthash.NodeType(node) {
	case ast.V_NONE, ast.V_NULL:
		return nil
	case ast.V_STRING:
		return collector.Add(node, contenthash.KindDialogue)
	case ast.V_ARRAY:
		return collector.CollectObjectSequence(node)
	default:
		return fmt.Errorf("Responses input must be a string or object array")
	}
}
