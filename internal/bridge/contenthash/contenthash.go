// Package contenthash builds protocol-specific content hashes from request ASTs.
package contenthash

import (
	"fmt"

	"github.com/bytedance/sonic/ast"
	"github.com/zeebo/xxh3"
)

// Element is the process-local digest of one cacheable request element.
type Element uint64

// Fingerprint preserves the request element order and the first dialogue position.
type Fingerprint struct {
	Elements      []Element
	FirstDialogue int
}

// Kind separates identical JSON used in different request sections.
type Kind byte

const (
	KindDialogue Kind = 'd'
	KindContext  Kind = 'c'
	KindTools    Kind = 't'
)

// Protocol owns protocol-specific AST normalization and element selection.
// Both methods receive a private request AST copy.
type Protocol interface {
	Normalize(root *ast.Node) error
	Collect(root *ast.Node, collector *Collector) error
}

// Build copies and normalizes the request AST, then asks the protocol to collect cacheable elements.
// Elements are counted before any JSON marshaling or hashing; requests below minimumElements are rejected
// without calculating element digests.
func Build(root *ast.Node, seed uint64, minimumElements int, protocol Protocol) (Fingerprint, error) {
	return build(root, seed, minimumElements, protocol, digest)
}

type elementHasher func(seed uint64, kind Kind, rawJSON []byte) Element

func build(root *ast.Node, seed uint64, minimumElements int, protocol Protocol, hasher elementHasher) (Fingerprint, error) {
	if root == nil || !root.Exists() {
		return Fingerprint{}, fmt.Errorf("content fingerprint AST is nil")
	}
	if protocol == nil {
		return Fingerprint{}, fmt.Errorf("content fingerprint protocol is nil")
	}
	if minimumElements <= 0 {
		return Fingerprint{}, fmt.Errorf("content fingerprint minimum elements must be positive")
	}
	if hasher == nil {
		return Fingerprint{}, fmt.Errorf("content fingerprint hasher is nil")
	}

	cloned, err := clone(root)
	if err != nil {
		return Fingerprint{}, err
	}
	if NodeType(cloned) != ast.V_OBJECT {
		return Fingerprint{}, fmt.Errorf("content fingerprint root must be an object")
	}
	if err := protocol.Normalize(cloned); err != nil {
		return Fingerprint{}, err
	}

	collector := NewCollector()
	if err := protocol.Collect(cloned, collector); err != nil {
		return Fingerprint{}, err
	}
	if !collector.hashable(minimumElements) {
		return Fingerprint{FirstDialogue: collector.firstDialogue}, nil
	}
	return collector.fingerprint(seed, hasher)
}

func clone(root *ast.Node) (*ast.Node, error) {
	raw, err := root.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("marshal content fingerprint AST copy: %w", err)
	}

	var cloned ast.Node
	if err := cloned.UnmarshalJSON(raw); err != nil {
		return nil, fmt.Errorf("parse content fingerprint AST copy: %w", err)
	}
	return &cloned, nil
}

type collectedElement struct {
	node *ast.Node
	kind Kind
}

// Collector contains the protocol-independent element validation and collection logic.
// Hashing is deliberately deferred until every element has been collected and counted.
type Collector struct {
	elements      []collectedElement
	firstDialogue int
}

func NewCollector() *Collector {
	return &Collector{}
}

func (c *Collector) hashable(minimumElements int) bool {
	return c.firstDialogue > 0 && len(c.elements) >= minimumElements
}

func (c *Collector) fingerprint(seed uint64, hasher elementHasher) (Fingerprint, error) {
	fingerprint := Fingerprint{
		Elements:      make([]Element, 0, len(c.elements)),
		FirstDialogue: c.firstDialogue,
	}
	for _, element := range c.elements {
		rawJSON, err := element.node.MarshalJSON()
		if err != nil {
			return Fingerprint{}, err
		}
		fingerprint.Elements = append(fingerprint.Elements, hasher(seed, element.kind, rawJSON))
	}
	return fingerprint, nil
}

// Add appends one non-empty AST value to the pending element sequence without hashing it.
func (c *Collector) Add(node *ast.Node, kind Kind) error {
	empty, err := valueEmpty(node)
	if err != nil {
		return err
	}
	if empty {
		return nil
	}

	if kind == KindDialogue && c.firstDialogue == 0 {
		c.firstDialogue = len(c.elements) + 1
	}
	c.elements = append(c.elements, collectedElement{node: node, kind: kind})
	return nil
}

// CollectWholeValue validates and adds one optional value as a single element.
func (c *Collector) CollectWholeValue(node *ast.Node, kind Kind, expectedType int) error {
	typ := NodeType(node)
	if typ == ast.V_NONE || typ == ast.V_NULL {
		return nil
	}
	if typ != expectedType {
		return fmt.Errorf("content fingerprint value has type %d, want %d", typ, expectedType)
	}
	return c.Add(node, kind)
}

// CollectObjectArray validates an optional object array and adds it as one element.
func (c *Collector) CollectObjectArray(node *ast.Node, kind Kind) error {
	typ := NodeType(node)
	if typ == ast.V_NONE || typ == ast.V_NULL {
		return nil
	}
	if typ != ast.V_ARRAY || !ValidateArrayItems(node, ast.V_OBJECT) {
		return fmt.Errorf("content fingerprint value must be an object array")
	}
	return c.Add(node, kind)
}

// CollectObjectSequence adds each object in an optional array as a dialogue element.
func (c *Collector) CollectObjectSequence(node *ast.Node) error {
	typ := NodeType(node)
	if typ == ast.V_NONE || typ == ast.V_NULL {
		return nil
	}
	if typ != ast.V_ARRAY {
		return fmt.Errorf("content fingerprint sequence must be an array")
	}

	length, err := node.Len()
	if err != nil {
		return err
	}
	for index := 0; index < length; index++ {
		item := node.Index(index)
		if NodeType(item) != ast.V_OBJECT {
			return fmt.Errorf("content fingerprint sequence item %d must be an object", index)
		}
		if err := c.Add(item, KindDialogue); err != nil {
			return err
		}
	}
	return nil
}

// NodeType safely resolves missing and lazily parsed Sonic AST nodes.
func NodeType(node *ast.Node) int {
	if node == nil || !node.Exists() {
		return ast.V_NONE
	}
	if err := node.Load(); err != nil {
		return ast.V_ERROR
	}
	return node.TypeSafe()
}

// ValidateArrayItems reports whether every array item has the expected type.
func ValidateArrayItems(node *ast.Node, expectedType int) bool {
	if NodeType(node) != ast.V_ARRAY {
		return false
	}
	length, err := node.Len()
	if err != nil {
		return false
	}
	for index := 0; index < length; index++ {
		if NodeType(node.Index(index)) != expectedType {
			return false
		}
	}
	return true
}

func valueEmpty(node *ast.Node) (bool, error) {
	switch NodeType(node) {
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

func digest(seed uint64, kind Kind, rawJSON []byte) Element {
	const seedMix uint64 = 0x9e3779b97f4a7c15
	kindSeed := seed ^ (uint64(kind) * seedMix)
	return Element(xxh3.HashSeed(rawJSON, kindSeed))
}
