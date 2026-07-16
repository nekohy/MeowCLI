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

// contentAffinityPrefix 是所有内容亲和匹配都必须完全相同的固定前缀。
// 前缀不足 minimumContentAffinityElements 的请求不会绑定，因此不需要进入 Radix Trie。
type contentAffinityPrefix [minimumContentAffinityElements]contentElementFingerprint

// contentTrieNode 表示一条压缩后的 Radix 边，一条边可以包含多个连续元素
// 桶根代表已经匹配的固定前缀，只有第 5 个及后续元素会进入压缩边
// Trie 结构由所属模型的锁保护；Otter 只通过节点指针管理终点的 TTL 和容量
type contentTrieNode struct {
	owner           *contentAffinityTrie
	bucket          *contentAffinityBucket
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

type contentAffinityBucket struct {
	prefix contentAffinityPrefix
	root   *contentTrieNode
}

// 每个模型独占一组固定前缀桶和结构锁，匹配查询不会串行化到一把全局锁上。
// 每个桶只保存共享前四个元素的后缀 Trie，因此不会产生深度 1～3 的分叉节点。
type contentAffinityTrie struct {
	bindMu       sync.Mutex
	mu           sync.RWMutex
	buckets      map[contentAffinityPrefix]*contentAffinityBucket
	bindSequence uint64
}

func newContentAffinityTrie() *contentAffinityTrie {
	return &contentAffinityTrie{
		buckets: make(map[contentAffinityPrefix]*contentAffinityBucket),
	}
}

func newContentAffinityBucket(owner *contentAffinityTrie, prefix contentAffinityPrefix) *contentAffinityBucket {
	bucket := &contentAffinityBucket{prefix: prefix}
	bucket.root = &contentTrieNode{
		owner:  owner,
		bucket: bucket,
		depth:  uint32(minimumContentAffinityElements),
	}
	return bucket
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
	prefix := contentAffinityPrefixFor(fingerprint)
	trie.mu.RLock()
	bucket := trie.buckets[prefix]
	if bucket == nil {
		trie.mu.RUnlock()
		return ""
	}
	result := trie.lookup(bucket, fingerprint)
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
	prefix := contentAffinityPrefixFor(fingerprint)
	bucket := trie.buckets[prefix]
	if bucket == nil {
		bucket = newContentAffinityBucket(trie, prefix)
		trie.buckets[prefix] = bucket
	}
	node := trie.insert(bucket, fingerprint, credential)
	generation := node.boundSequence
	trie.mu.Unlock()
	// Set 在写入或覆盖时重置 ExpiryWriting 的 TTL；删除回调使用旧 generation 时会被忽略
	t.entries.Set(node, generation)
	trie.bindMu.Unlock()
}

func contentAffinityPrefixFor(fingerprint contentHash) contentAffinityPrefix {
	var prefix contentAffinityPrefix
	copy(prefix[:], fingerprint.elements[:minimumContentAffinityElements])
	return prefix
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
		bucket:   parent.bucket,
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

// lookup 已由固定前缀桶确认前四个元素完全相同，只在后缀中查找最长公共前缀。
// 发生分叉后，直接使用节点上预计算的最近终点。
func (trie *contentAffinityTrie) lookup(bucket *contentAffinityBucket, fingerprint contentHash) contentAffinityLookup {
	result := contentAffinityLookup{matchedDepth: uint32(minimumContentAffinityElements)}
	current := bucket.root
	position := minimumContentAffinityElements

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

// insert 只为固定四元素前缀之后的新后缀创建节点，已有后缀继续由不同分支共享。
func (trie *contentAffinityTrie) insert(bucket *contentAffinityBucket, fingerprint contentHash, credential string) *contentTrieNode {
	current := bucket.root
	position := minimumContentAffinityElements

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
	for node := terminal; node != nil; node = node.parent {
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
	bucket := node.bucket
	node.terminal = false
	node.credential = ""
	node.boundSequence = 0

	// 先裁剪无终点的路径，再从第一个仍保留的节点向上重算，避免删除前后重复扫描父节点。
	recomputeFrom := trie.pruneEmptyPath(node)
	if recomputeFrom != nil {
		trie.recomputePath(recomputeFrom)
	}
	if trie.buckets[bucket.prefix] == bucket && !bucket.root.terminal && len(bucket.root.children) == 0 {
		delete(trie.buckets, bucket.prefix)
	}
}

// pruneEmptyPath 删除或合并已经不再承载终点的后缀节点，并返回需要重新计算的最高有效子树根。
// 遇到 terminal 祖先时，该祖先及更高节点的最近终点仍是它自己，无需重新计算。
func (trie *contentAffinityTrie) pruneEmptyPath(node *contentTrieNode) *contentTrieNode {
	bucketRoot := node.bucket.root
	for node != bucketRoot && !node.terminal {
		switch len(node.children) {
		case 0:
			parent := node.parent
			delete(parent.children, node.incoming[0])
			node.parent = nil
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
			node = parent
		default:
			return node
		}
	}
	if node.terminal {
		return nil
	}
	return node
}

func onlyContentChild(node *contentTrieNode) *contentTrieNode {
	for _, child := range node.children {
		return child
	}
	return nil
}
