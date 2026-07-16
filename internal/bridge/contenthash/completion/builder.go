// Package completion builds Chat Completions content fingerprints.
package completion

import (
	"github.com/nekohy/MeowCLI/internal/bridge/contenthash"

	"github.com/bytedance/sonic/ast"
)

type Builder struct{}

var _ contenthash.Protocol = Builder{}

func (Builder) Normalize(_ *ast.Node) error {
	return nil
}

func (Builder) Collect(root *ast.Node, collector *contenthash.Collector) error {
	if err := collector.CollectObjectArray(root.Get("tools"), contenthash.KindTools); err != nil {
		return err
	}
	if err := collector.CollectObjectSequence(root.Get("messages")); err != nil {
		return err
	}
	return nil
}
