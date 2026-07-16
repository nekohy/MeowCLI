// Package messages builds Anthropic Messages content fingerprints.
package messages

import (
	"fmt"

	"github.com/nekohy/MeowCLI/internal/bridge/contenthash"

	"github.com/bytedance/sonic/ast"
)

type Builder struct{}

var _ contenthash.Protocol = Builder{}

// Normalize removes request metadata that does not contribute to the prompt cache key.
func (Builder) Normalize(root *ast.Node) error {
	if root == nil || !root.Exists() {
		return nil
	}
	if err := root.Load(); err != nil {
		return err
	}
	return removeCacheControl(root)
}

func (Builder) Collect(root *ast.Node, collector *contenthash.Collector) error {
	system := root.Get("system")
	switch contenthash.NodeType(system) {
	case ast.V_NONE, ast.V_NULL:
	case ast.V_STRING:
		if err := collector.Add(system, contenthash.KindContext); err != nil {
			return err
		}
	case ast.V_ARRAY:
		if !contenthash.ValidateArrayItems(system, ast.V_OBJECT) {
			return fmt.Errorf("Messages system must be an object array")
		}
		if err := collector.Add(system, contenthash.KindContext); err != nil {
			return err
		}
	default:
		return fmt.Errorf("Messages system must be a string or object array")
	}

	if err := collector.CollectObjectArray(root.Get("tools"), contenthash.KindTools); err != nil {
		return err
	}
	if err := collectMessages(collector, root.Get("messages")); err != nil {
		return err
	}
	return nil
}

func collectMessages(collector *contenthash.Collector, node *ast.Node) error {
	typ := contenthash.NodeType(node)
	if typ == ast.V_NONE || typ == ast.V_NULL {
		return nil
	}
	if typ != ast.V_ARRAY {
		return fmt.Errorf("Messages messages must be an array")
	}

	length, err := node.Len()
	if err != nil {
		return err
	}
	for index := 0; index < length; index++ {
		item := node.Index(index)
		if contenthash.NodeType(item) != ast.V_OBJECT {
			return fmt.Errorf("Messages message %d must be an object", index)
		}
		content := item.Get("content")
		switch contenthash.NodeType(content) {
		case ast.V_STRING:
		case ast.V_ARRAY:
			if !contenthash.ValidateArrayItems(content, ast.V_OBJECT) {
				return fmt.Errorf("Messages message %d content must contain objects", index)
			}
		default:
			return fmt.Errorf("Messages message %d content must be a string or object array", index)
		}
		if err := collector.Add(item, contenthash.KindDialogue); err != nil {
			return err
		}
	}
	return nil
}

func removeCacheControl(node *ast.Node) error {
	if node == nil || !node.Exists() {
		return nil
	}

	switch node.TypeSafe() {
	case ast.V_ARRAY, ast.V_OBJECT:
		var visitErr error
		if err := node.ForEach(func(sequence ast.Sequence, child *ast.Node) bool {
			if sequence.Key != nil && *sequence.Key == "cache_control" {
				return true
			}
			visitErr = removeCacheControl(child)
			return visitErr == nil
		}); err != nil {
			return err
		}
		if visitErr != nil {
			return visitErr
		}
	}

	if node.TypeSafe() != ast.V_OBJECT {
		return nil
	}
	for {
		removed, err := node.Unset("cache_control")
		if err != nil {
			return fmt.Errorf("remove cache_control: %w", err)
		}
		if !removed {
			return nil
		}
	}
}
