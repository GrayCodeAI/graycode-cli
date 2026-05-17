package pathtrie

import "strings"

type node struct {
	name     string
	depth    int
	children map[string]*node
	count    uint64
}

func (n *node) addChild(name string) *node {
	if child, exists := n.children[name]; exists {
		return child
	}
	child := &node{name: name, depth: n.depth + 1, children: make(map[string]*node)}
	n.children[name] = child
	return child
}

func (n *node) mergeSubtree(other *node) {
	n.count += other.count
	for _, otherChild := range other.children {
		if target, exists := n.children[otherChild.name]; exists {
			target.mergeSubtree(otherChild)
		} else {
			n.children[otherChild.name] = otherChild
		}
	}
}

func (n *node) collapseToWildcard() {
	if _, ok := n.children["*"]; ok && len(n.children) == 1 {
		return
	}
	star := &node{name: "*", depth: n.depth + 1, children: make(map[string]*node)}
	for _, child := range n.children {
		star.mergeSubtree(child)
	}
	n.children = map[string]*node{"*": star}
}

func (n *node) collect(prefix []string) []Pattern {
	var results []Pattern
	path := append(prefix, n.name)

	if n.count > 0 {
		p := make([]string, len(path))
		copy(p, path)
		results = append(results, Pattern{Parts: p, Count: n.count})
	}

	for _, child := range n.children {
		results = append(results, child.collect(path)...)
	}
	return results
}

// Pattern is a generalized path pattern with its occurrence count.
type Pattern struct {
	Parts []string
	Count uint64
}

func (p Pattern) String() string {
	return strings.Join(p.Parts, "/")
}

// Trie auto-collapses high-cardinality path segments into wildcards.
type Trie struct {
	root      *node
	threshold int
}

// New creates a Trie with the given high-cardinality threshold.
// When a node has more children than threshold, they collapse into "*".
func New(highCardinalityThreshold int) *Trie {
	if highCardinalityThreshold <= 0 {
		highCardinalityThreshold = 10
	}
	return &Trie{
		root:      &node{name: "", depth: 0, children: make(map[string]*node)},
		threshold: highCardinalityThreshold,
	}
}

// Insert adds a path (split by separator) into the trie.
func (t *Trie) Insert(path string, sep string) {
	if sep == "" {
		sep = "/"
	}
	parts := strings.Split(strings.Trim(path, sep), sep)
	t.InsertParts(parts, 1)
}

// InsertParts inserts pre-split path parts with a count.
func (t *Trie) InsertParts(parts []string, count uint64) {
	current := t.root
	for _, segment := range parts {
		if wildcard, ok := current.children["*"]; ok {
			current = wildcard
			continue
		}
		if segment == "*" {
			current.collapseToWildcard()
			current = current.children["*"]
			continue
		}
		if current.depth > 0 && len(current.children) >= t.threshold {
			current.collapseToWildcard()
			current = current.children["*"]
			continue
		}
		current = current.addChild(segment)
	}
	current.count += count
}

// Patterns returns all accumulated patterns.
func (t *Trie) Patterns() []Pattern {
	var results []Pattern
	for _, child := range t.root.children {
		results = append(results, child.collect(nil)...)
	}
	return results
}
