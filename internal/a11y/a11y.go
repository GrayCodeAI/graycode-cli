// Package a11y compresses Chrome accessibility trees into token-efficient,
// uid-addressable snapshots for agent browser interaction, adopting
// caveman-browse's contract: a compressed indented tree where actionable
// nodes carry stable uid handles, a query mode that keeps only the top task
// matches plus their ancestors, byte-exact raw payload retention for
// recovery, and fail-closed behavior — if compression does not actually
// shrink the representation, callers must not dump the raw tree.
package a11y

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrNotSmaller reports that compression produced no reduction; per the
// fail-closed rule the caller must keep its previous snapshot instead of
// rendering an uncompressed tree.
var ErrNotSmaller = errors.New("a11y: compression did not reduce size")

// Node is the minimal view of a CDP accessibility node this package needs.
type Node struct {
	ID           string   `json:"nodeId"`
	Ignored      bool     `json:"ignored"`
	Role         string   `json:"role"`
	Name         string   `json:"name"`
	Value        string   `json:"value"`
	ChildIDs     []string `json:"childIds"`
	BackendDOMID int64    `json:"backendDOMId"`
}

// Ref identifies one actionable element from a snapshot.
type Ref struct {
	UID          string `json:"uid"`
	Role         string `json:"role"`
	Name         string `json:"name"`
	BackendDOMID int64  `json:"backend_dom_id"`
}

// Snapshot is the compressed, agent-facing result.
type Snapshot struct {
	// Text is the compact indented tree shown to the model.
	Text string
	// Refs maps uid -> element descriptor for act actions.
	Refs map[string]Ref
	// RawJSON retains the exact source payload for byte-exact recovery.
	RawJSON string
	// Truncated reports that query mode omitted branches (ancestors kept).
	Truncated bool
}

// actionRoles are the AX roles an agent can meaningfully act on. Everything
// else renders as structure or text without consuming a uid.
var actionRoles = map[string]bool{
	"button": true, "link": true, "textbox": true, "searchbox": true,
	"combobox": true, "listbox": true, "checkbox": true, "radio": true,
	"menuitem": true, "menuitemcheckbox": true, "menuitemradio": true,
	"tab": true, "slider": true, "switch": true, "option": true,
}

// skipRoles are pure layout containers whose only contribution is depth;
// dropping them keeps the render shallow without losing actionable leaves.
var skipRoles = map[string]bool{
	"generic": true, "none": true, "presentation": true, "group": true,
	"list": true, "region": true, "banner": true, "contentinfo": true,
	"main": true, "navigation": true, "complementary": true, "form": true,
}

// MaxMatches bounds query-mode results (caveman-browse keeps 12).
const MaxMatches = 12

// Compress renders the flat node list (as returned by CDP
// Accessibility.getFullAXTree) as a compact uid-addressed tree. rawJSON is
// kept verbatim on the snapshot for byte-exact recovery. When query is
// non-empty, ranking retains at most MaxMatches actionable nodes plus their
// ancestor chains and sets Truncated. Fail-closed: ErrNotSmaller when the
// rendered text would be at least as large as the raw payload.
func Compress(nodes []Node, rawJSON, query string) (*Snapshot, error) {
	if len(nodes) == 0 {
		return nil, ErrNotSmaller
	}
	byID := make(map[string]*Node, len(nodes))
	for i := range nodes {
		byID[nodes[i].ID] = &nodes[i]
	}
	root := pickRoot(nodes)

	// Query mode: decide what survives before rendering. keepSet holds top
	// matches plus every ancestor; contains marks subtrees that hold anything
	// worth rendering. Non-matching branches are pruned wholesale.
	keepSet := map[string]bool{}
	if query != "" {
		for _, m := range rankMatches(nodes, byID, root, query) {
			for anc := m; anc != "" && !keepSet[anc]; anc = parentOf(byID, anc) {
				keepSet[anc] = true
			}
		}
	}
	contains := map[string]bool{}
	var mark func(id string) bool
	mark = func(id string) bool {
		n, ok := byID[id]
		if !ok {
			return false
		}
		c := keepSet[id]
		for _, ch := range n.ChildIDs {
			if byID[ch] != nil { // unresolvable childIds are leaves, not errors
				c = mark(ch) || c
			}
		}
		contains[id] = c
		return c
	}
	if query != "" {
		mark(root)
	}

	var lines []string
	refs := map[string]Ref{}
	counter := 0

	var walk func(id string, depth int)
	walk = func(id string, depth int) {
		n, ok := byID[id]
		if !ok || n.Ignored {
			return
		}
		render := query == "" || keepSet[id] || contains[id]
		if !render {
			return // whole branch pruned: no matches beneath it
		}
		indent := strings.Repeat("  ", depth)
		switch {
		case actionRoles[n.Role]:
			if query != "" && !keepSet[id] {
				break // actionable but not a selected match/ancestor path
			}
			counter++
			uid := fmt.Sprintf("u%d", counter)
			name := oneLine(n.Name)
			lines = append(lines, fmt.Sprintf("%s- %s %s %q", indent, n.Role, uid, name))
			refs[uid] = Ref{UID: uid, Role: n.Role, Name: name, BackendDOMID: n.BackendDOMID}
		case skipRoles[n.Role]:
			// Layout container: contributes depth only.
		default:
			label := oneLine(n.Name)
			if label == "" && n.Value != "" {
				label = oneLine(n.Value)
			}
			if label != "" {
				role := n.Role
				if role == "" {
					role = "text"
				}
				lines = append(lines, fmt.Sprintf("%s- %s %q", indent, role, label))
			}
		}
		for _, c := range n.ChildIDs {
			child, ok := byID[c]
			if !ok {
				// Unresolvable childId: an iframe leaf under site isolation.
				lines = append(lines, strings.Repeat("  ", depth+1)+"- frame (separate document)")
				continue
			}
			walk(child.ID, depth+1)
		}
	}
	walk(root, 0)

	text := strings.Join(lines, "\n")
	if strings.TrimSpace(text) == "" || len(text) >= len(rawJSON) {
		return nil, ErrNotSmaller
	}
	snap := &Snapshot{Text: text, Refs: refs, RawJSON: rawJSON}
	if query != "" {
		snap.Truncated = countActionable(nodes) > len(refs)
	}
	return snap, nil
}

func countActionable(nodes []Node) int {
	n := 0
	for _, nd := range nodes {
		if actionRoles[nd.Role] && !nd.Ignored {
			n++
		}
	}
	return n
}

// pickRoot chooses the first non-ignored node with no resolvable parent.
func pickRoot(nodes []Node) string {
	hasParent := map[string]bool{}
	for _, n := range nodes {
		for _, c := range n.ChildIDs {
			hasParent[c] = true
		}
	}
	for _, n := range nodes {
		if !n.Ignored && !hasParent[n.ID] {
			return n.ID
		}
	}
	if len(nodes) > 0 {
		return nodes[0].ID
	}
	return ""
}

func parentOf(byID map[string]*Node, id string) string {
	for _, n := range byID {
		for _, c := range n.ChildIDs {
			if c == id {
				return n.ID
			}
		}
	}
	return ""
}

// rankMatches scores actionable nodes against the query terms and returns
// at most MaxMatches node ids, best first.
func rankMatches(nodes []Node, byID map[string]*Node, root, query string) []string {
	terms := strings.Fields(strings.ToLower(query))
	type scored struct {
		id    string
		score float64
	}
	var out []scored
	for _, n := range nodes {
		if !actionRoles[n.Role] || n.Ignored {
			continue
		}
		hay := strings.ToLower(n.Name + " " + n.Role + " " + n.Value)
		var sc float64
		for _, t := range terms {
			if t == "" {
				continue
			}
			if hay == t {
				sc += 3
			} else if strings.HasPrefix(hay, t) || strings.HasSuffix(hay, t) {
				sc += 2
			} else if strings.Contains(hay, t) {
				sc += 1
			}
		}
		if sc > 0 {
			out = append(out, scored{id: n.ID, score: sc})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].score > out[j].score })
	if len(out) > MaxMatches {
		out = out[:MaxMatches]
	}
	ids := make([]string, len(out))
	for i, s := range out {
		ids[i] = s.id
	}
	return ids
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 120 {
		s = s[:120] + "…"
	}
	return s
}
