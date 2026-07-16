package bridge

import (
	"crypto/rand"
	"encoding/binary"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nekohy/MeowCLI/internal/settings"

	"github.com/maypok86/otter/v2"
)

const defaultContentAffinityTTL = time.Hour

// contentTrieNode 表示一条压缩后的 Radix 边，一条边可以包含多个连续元素
// Trie 结构由所属模型的锁保护；Otter 只通过节点指针管理终点的 TTL 和容量
type contentTrieNode struct {
	owner           *contentAffinityTrie
	parent          *contentTrieNode
	incoming        []contentElementFingerprint
	children        map[contentElementFingerprint]*contentTrieNode
	terminal        bool
	credential      string
	boundSequence   uint64
	nearestTerminal *contentTrieNode
	depth           uint32
}

type contentAffinityLookup struct {
	matchedDepth    uint32
	nearestTerminal *contentTrieNode
}

// 每个模型独占一棵 Trie 和结构锁，匹配查询不会串行化到一把全局锁上
type contentAffinityTrie struct {
	bindMu       sync.Mutex
	mu           sync.RWMutex
	root         *contentTrieNode
	bindSequence uint64
}

func newContentAffinityTrie() *contentAffinityTrie {
	trie := &contentAffinityTrie{}
	trie.root = &contentTrieNode{owner: trie}
	return trie
}

type contentAffinityTable struct {
	triesMu    sync.RWMutex
	capacityMu sync.Mutex
	seed       uint64
	tries      map[string]*contentAffinityTrie
	entries    *otter.Cache[*contentTrieNode, uint64]
	capacity   atomic.Int64
}

func newContentAffinityTable() *contentAffinityTable {
	capacity := settings.DefaultContentAffinityMaxEntries
	var seedBytes [8]byte
	_, _ = rand.Read(seedBytes[:])
	table := &contentAffinityTable{
		seed:  binary.LittleEndian.Uint64(seedBytes[:]),
		tries: make(map[string]*contentAffinityTrie),
	}
	table.entries = otter.Must(&otter.Options[*contentTrieNode, uint64]{
		MaximumSize:      capacity,
		ExpiryCalculator: otter.ExpiryWriting[*contentTrieNode, uint64](defaultContentAffinityTTL),
		Executor: func(fn func()) {
			fn()
		},
		OnDeletion: expireContentAffinityBinding,
	})
	table.capacity.Store(int64(capacity))
	return table
}

// expireContentAffinityBinding 由 Otter 的后台 timing wheel 或容量淘汰回调触发
// generation 防止旧绑定的替换事件误删刚刚写入的新绑定
func expireContentAffinityBinding(event otter.DeletionEvent[*contentTrieNode, uint64]) {
	node := event.Key
	trie := node.owner
	trie.mu.Lock()
	if node.terminal && node.boundSequence == event.Value {
		trie.expireTerminal(node)
	}
	trie.mu.Unlock()
}

func (t *contentAffinityTable) match(modelName string, fingerprint contentHash) string {
	if modelName == "" || !fingerprint.valid() {
		return ""
	}

	trie := t.modelTrie(modelName, false)
	if trie == nil {
		return ""
	}
	trie.mu.RLock()
	result := trie.lookup(fingerprint)
	minimumMatched := max(fingerprint.firstDialogue, minimumContentAffinityElements)
	if result.matchedDepth < uint32(minimumMatched) || result.nearestTerminal == nil {
		trie.mu.RUnlock()
		return ""
	}
	node := result.nearestTerminal
	generation := node.boundSequence
	credential := node.credential
	trie.mu.RUnlock()

	// Otter 是 TTL/容量的唯一有效性来源；GetIfPresent 同时记录命中热度
	cachedGeneration, ok := t.entries.GetIfPresent(node)
	if !ok || cachedGeneration != generation {
		return ""
	}
	return credential
}

func (t *contentAffinityTable) bind(modelName string, fingerprint contentHash, credential string) {
	if modelName == "" || !fingerprint.valid() || credential == "" {
		return
	}

	trie := t.modelTrie(modelName, true)
	// Otter 回调需要获取 trie.mu，因此 Set 必须位于结构锁外；bindMu 保证同一模型
	// 多个绑定仍按 markTerminal -> Set 的顺序提交，避免同一终点 generation 倒序
	trie.bindMu.Lock()
	trie.mu.Lock()
	node := trie.insert(fingerprint, credential)
	generation := node.boundSequence
	trie.mu.Unlock()
	// Set 在写入或覆盖时重置 ExpiryWriting 的 TTL；删除回调使用旧 generation 时会被忽略
	t.entries.Set(node, generation)
	trie.bindMu.Unlock()
}

func (t *contentAffinityTable) configureCapacity(capacity int) {
	if capacity <= 0 {
		capacity = settings.DefaultContentAffinityMaxEntries
	}
	target := int64(capacity)
	if t.capacity.Load() == target {
		return
	}

	t.capacityMu.Lock()
	defer t.capacityMu.Unlock()
	if t.capacity.Load() != target {
		// 先发布目标值；同值并发调用可直接返回，当前调用仍会完成实际缩放。
		t.capacity.Store(target)
		t.entries.SetMaximum(uint64(capacity))
	}
}

func (t *contentAffinityTable) modelTrie(modelName string, create bool) *contentAffinityTrie {
	t.triesMu.RLock()
	trie := t.tries[modelName]
	t.triesMu.RUnlock()
	if trie != nil || !create {
		return trie
	}
	t.triesMu.Lock()
	trie = t.tries[modelName]
	if trie == nil {
		trie = newContentAffinityTrie()
		t.tries[modelName] = trie
	}
	t.triesMu.Unlock()
	return trie
}

func newContentTrieNode(parent *contentTrieNode, incoming []contentElementFingerprint, depth uint32) *contentTrieNode {
	return &contentTrieNode{
		owner:    parent.owner,
		parent:   parent,
		incoming: append([]contentElementFingerprint(nil), incoming...),
		depth:    depth,
	}
}

func setContentChild(parent, child *contentTrieNode) {
	if parent.children == nil {
		parent.children = make(map[contentElementFingerprint]*contentTrieNode)
	}
	parent.children[child.incoming[0]] = child
}

// lookup 从头查找最长公共前缀；发生分叉后，直接使用节点上预计算的最近终点
func (trie *contentAffinityTrie) lookup(fingerprint contentHash) contentAffinityLookup {
	result := contentAffinityLookup{}
	current := trie.root
	position := 0

	for position < len(fingerprint.elements) {
		child := current.children[fingerprint.elements[position]]
		if child == nil {
			result.nearestTerminal = current.nearestTerminal
			return result
		}
		common := commonElementPrefix(child.incoming, fingerprint.elements[position:])
		position += common
		result.matchedDepth = uint32(position)
		if common < len(child.incoming) {
			result.nearestTerminal = child.nearestTerminal
			return result
		}
		current = child
	}

	result.nearestTerminal = current.nearestTerminal
	return result
}

func commonElementPrefix(left, right []contentElementFingerprint) int {
	limit := min(len(left), len(right))
	for index := 0; index < limit; index++ {
		if left[index] != right[index] {
			return index
		}
	}
	return limit
}

// insert 只为新后缀创建节点，已有公共前缀会继续由不同分支共享
func (trie *contentAffinityTrie) insert(fingerprint contentHash, credential string) *contentTrieNode {
	current := trie.root
	position := 0

	for position < len(fingerprint.elements) {
		child := current.children[fingerprint.elements[position]]
		if child == nil {
			leaf := newContentTrieNode(current, fingerprint.elements[position:], uint32(len(fingerprint.elements)))
			setContentChild(current, leaf)
			trie.markTerminal(leaf, credential)
			trie.propagateTerminal(leaf)
			return leaf
		}

		common := commonElementPrefix(child.incoming, fingerprint.elements[position:])
		if common < len(child.incoming) {
			split := trie.splitChild(current, child, common)
			position += common
			terminal := split
			if position < len(fingerprint.elements) {
				terminal = newContentTrieNode(split, fingerprint.elements[position:], uint32(len(fingerprint.elements)))
				setContentChild(split, terminal)
			}
			trie.markTerminal(terminal, credential)
			trie.propagateTerminal(terminal)
			return terminal
		}

		position += common
		current = child
	}

	trie.markTerminal(current, credential)
	trie.propagateTerminal(current)
	return current
}

func (trie *contentAffinityTrie) splitChild(parent, child *contentTrieNode, common int) *contentTrieNode {
	original := child.incoming
	split := newContentTrieNode(parent, original[:common], parent.depth+uint32(common))
	child.incoming = append([]contentElementFingerprint(nil), original[common:]...)
	child.parent = split
	setContentChild(parent, split)
	setContentChild(split, child)
	trie.recomputeNode(split)
	return split
}

func (trie *contentAffinityTrie) markTerminal(node *contentTrieNode, credential string) {
	trie.bindSequence++
	node.terminal = true
	node.credential = credential
	node.boundSequence = trie.bindSequence
}

func (trie *contentAffinityTrie) recomputePath(node *contentTrieNode) {
	for node != nil {
		trie.recomputeNode(node)
		node = node.parent
	}
}

// 插入终点时只沿父链更新“最近终点”，不会扫描兄弟子树
func (trie *contentAffinityTrie) propagateTerminal(terminal *contentTrieNode) {
	for node := terminal; node != nil && node != trie.root; node = node.parent {
		nearest := node.nearestTerminal
		if nearest != nil && nearest != terminal &&
			(nearest.depth < terminal.depth ||
				(nearest.depth == terminal.depth && !terminalNewer(terminal, nearest))) {
			return
		}
		node.nearestTerminal = terminal
	}
}

func (trie *contentAffinityTrie) recomputeNode(node *contentTrieNode) {
	if node == trie.root {
		// 根节点匹配长度为 0，永远达不到 firstDialogue，因此无需维护全局最近终点
		node.nearestTerminal = nil
		return
	}

	if node.terminal {
		node.nearestTerminal = node
		return
	}

	// 对同一个祖先，相对距离的排序等同于终点绝对深度的排序。
	var best *contentTrieNode
	for _, child := range node.children {
		candidate := child.nearestTerminal
		if candidate == nil {
			continue
		}
		if best == nil || candidate.depth < best.depth ||
			(candidate.depth == best.depth && terminalNewer(candidate, best)) {
			best = candidate
		}
	}
	node.nearestTerminal = best
}

func terminalNewer(left, right *contentTrieNode) bool {
	return left.boundSequence > right.boundSequence
}

func (trie *contentAffinityTrie) expireTerminal(node *contentTrieNode) {
	node.terminal = false
	node.credential = ""
	node.boundSequence = 0
	trie.recomputePath(node)
	trie.pruneEmptyPath(node)
}

func (trie *contentAffinityTrie) pruneEmptyPath(node *contentTrieNode) {
	for node != nil && node != trie.root && !node.terminal {
		switch len(node.children) {
		case 0:
			parent := node.parent
			delete(parent.children, node.incoming[0])
			node.parent = nil
			trie.recomputeNode(parent)
			node = parent
		case 1:
			parent := node.parent
			child := onlyContentChild(node)
			combined := make([]contentElementFingerprint, 0, len(node.incoming)+len(child.incoming))
			combined = append(combined, node.incoming...)
			combined = append(combined, child.incoming...)
			parent.children[node.incoming[0]] = child
			child.parent = parent
			child.incoming = combined
			node.parent = nil
			node.children = nil
			trie.recomputeNode(parent)
			node = parent
		default:
			return
		}
	}
}

func onlyContentChild(node *contentTrieNode) *contentTrieNode {
	for _, child := range node.children {
		return child
	}
	return nil
}
