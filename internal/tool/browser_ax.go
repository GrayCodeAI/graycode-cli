package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/chromedp/cdproto/accessibility"
	"github.com/chromedp/cdproto/cdp"
	cdpproto_dom "github.com/chromedp/cdproto/dom"
	cdpproto_runtime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/go-json-experiment/json/jsontext"

	"github.com/GrayCodeAI/hawk/internal/a11y"
)

// Accessibility-tree snapshot support, adopting caveman-browse's contract:
// compressed indented AX tree with uid handles, fail-closed on non-reducing
// compressions (prior uids stay valid; the raw tree is never dumped), and
// byte-exact canonical payload retention for recovery.

// axState holds the uid map of the most recent successful snapshot.
var (
	axMu          sync.Mutex
	axCurrentRefs map[string]a11y.Ref
)

func storeAXRefs(refs map[string]a11y.Ref) {
	axMu.Lock()
	defer axMu.Unlock()
	axCurrentRefs = refs
}

func lookupUID(uid string) (a11y.Ref, bool) {
	axMu.Lock()
	defer axMu.Unlock()
	r, ok := axCurrentRefs[strings.TrimSpace(uid)]
	return r, ok
}

// browserAXNode converts a cdproto accessibility node into the a11y view.
func browserAXNode(n *accessibility.Node) a11y.Node {
	out := a11y.Node{
		ID:           string(n.NodeID),
		Ignored:      n.Ignored,
		BackendDOMID: int64(n.BackendDOMNodeID),
	}
	if n.Role != nil {
		out.Role = strings.ToLower(strings.TrimSpace(string(n.Role.Value)))
	}
	if n.Name != nil {
		out.Name = unquoteAXValue(n.Name.Value)
	}
	if n.Value != nil {
		out.Value = unquoteAXValue(n.Value.Value)
	}
	for _, c := range n.ChildIDs {
		out.ChildIDs = append(out.ChildIDs, string(c))
	}
	return out
}

// unquoteAXValue decodes a jsontext-encoded scalar into its Go string form.
func unquoteAXValue(v jsontext.Value) string {
	var s string
	if err := json.Unmarshal(v, &s); err == nil {
		return s
	}
	return strings.Trim(string(v), `"`)
}

// fetchAXTree pulls the full accessibility tree for the current page in
// canonical (converted) form plus that form's exact JSON encoding.
func fetchAXTree(ctx context.Context) ([]a11y.Node, string, error) {
	var nodes []*accessibility.Node
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
		var err error
		nodes, err = accessibility.GetFullAXTree().Do(c)
		return err
	})); err != nil {
		return nil, "", fmt.Errorf("ax tree: %w", err)
	}
	converted := make([]a11y.Node, len(nodes))
	for i, n := range nodes {
		converted[i] = browserAXNode(n)
	}
	raw, err := json.Marshal(converted)
	if err != nil {
		return nil, "", err
	}
	return converted, string(raw), nil
}

// actByBackendID resolves the DOM node behind an AX node and runs fn with its
// remote object id so agents never need CSS selectors.
func actByBackendID(ctx context.Context, backend int64, jsFn string) error {
	var objID cdpproto_runtime.RemoteObjectID
	if err := chromedp.Run(
		ctx,
		chromedp.ActionFunc(func(c context.Context) error {
			obj, err := cdpproto_dom.ResolveNode().WithBackendNodeID(cdp.BackendNodeID(backend)).Do(c)
			if err != nil {
				return fmt.Errorf("resolve node %d: %w", backend, err)
			}
			if obj == nil || obj.ObjectID == "" {
				return fmt.Errorf("node %d has no remote object", backend)
			}
			objID = obj.ObjectID
			return nil
		}),
		chromedp.ActionFunc(func(c context.Context) error {
			_, _, err := cdpproto_runtime.CallFunctionOn(jsFn).
				WithObjectID(objID).
				WithReturnByValue(true).
				Do(c)
			return err
		}),
	); err != nil {
		return err
	}
	return nil
}

const axClickJS = `function() { this.scrollIntoView({block:'center'}); this.click(); }`

// axSnapshot fetches, compresses, and stores the snapshot. On ErrNotSmaller
// it fails closed keeping prior uids intact (never dump the raw tree, never
// wipe the working uid map).
func axSnapshot(ctx context.Context, query string) (string, error) {
	nodes, raw, err := fetchAXTree(ctx)
	if err != nil {
		return "", err
	}
	snap, cerr := a11y.Compress(nodes, raw, query)
	if cerr != nil {
		return "", fmt.Errorf("snapshot not usable (%w); previous uids remain valid", cerr)
	}
	storeAXRefs(snap.Refs)
	header := fmt.Sprintf("uid map updated: %d actionable elements", len(snap.Refs))
	if snap.Truncated {
		header += " (query mode: pruned to top matches)"
	}
	return header + "\n\n" + snap.Text, nil
}

// axTypeWrapJS wraps axTypeJS as a CallFunctionOn declaration whose parameter
// receives the JSON-encoded text argument.
func axTypeWrapJS(text string) string {
	b, _ := json.Marshal(text)
	return "function(t) { this.focus(); this.value='';" +
		" document.execCommand('insertText', false, t);" +
		" if(this.value!==t){ this.value=t; this.dispatchEvent(new Event('input',{bubbles:true})); } }" +
		"\n" + string(b)
}
