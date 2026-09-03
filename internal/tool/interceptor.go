// Package tool implements waterfall-style interception for the tool execution
// path. This is the Go-native port of DeepSeek Harness's tools/pre-execute →
// tools/execute → tools/post-execute pipeline: an ordered chain of typed stages
// where each stage may short-circuit the rest.
package tool

import (
	"context"
	"fmt"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

// Stage identifies where in the tool pipeline an interceptor runs.
type Stage string

const (
	// StagePreExecute runs before the raw tool is resolved and invoked.
	StagePreExecute Stage = "pre"
	// StagePostExecute runs after the raw tool returns, before the result is
	// normalized and surfaced to the model.
	StagePostExecute Stage = "post"
)

// StageAll registers one interceptor at every stage.
const StageAll Stage = "*"

// ToolRequest is the information available to a pre-execute interceptor: the tool
// call as the model issued it, plus the resolved Tool implementation when the
// registry lookup has already happened.
type ToolRequest struct {
	Call types.ToolCall
	Tool Tool
}

// ToolResult is the information available to a post-execute interceptor.
type ToolResult struct {
	Request ToolRequest
	Output  string
	IsError bool
	Err     error
}

// InterceptFn is a pipeline stage. It runs with the request, a mutable result
// slot, and next — the remainder of the chain. A stage passes control downstream
// by returning next(); it short-circuits by returning without calling next. The
// first stage to short-circuit stops the chain and its error (if any) becomes the
// chain's result.
type InterceptFn func(ctx context.Context, req ToolRequest, res *ToolResult, next func() error) error

type chainNode struct {
	fn   InterceptFn
	rest *chainNode
}

// Chain is an ordered, single-linked sequence of InterceptFns. The zero value is
// an empty pass-through chain.
type Chain struct {
	head *chainNode
	tail *chainNode
	len  int
}

// Append adds an interceptor to the end of the chain and returns a disposer that
// removes exactly this node. Appending a nil fn is a no-op.
func (c *Chain) Append(fn InterceptFn) func() {
	if c == nil || fn == nil {
		return func() {}
	}
	node := &chainNode{fn: fn}
	if c.tail == nil {
		c.head = node
	} else {
		c.tail.rest = node
	}
	c.tail = node
	c.len++
	return func() { c.remove(node) }
}

// Len reports the number of interceptors in the chain.
func (c *Chain) Len() int {
	if c == nil {
		return 0
	}
	return c.len
}

func (c *Chain) remove(node *chainNode) {
	if c == nil || c.head == nil {
		return
	}
	if c.head == node {
		c.head = node.rest
		if c.head == nil {
			c.tail = nil
		}
		c.len--
		return
	}
	cur := c.head
	for cur != nil {
		if cur.rest == node {
			cur.rest = node.rest
			if node == c.tail {
				c.tail = cur
			}
			c.len--
			return
		}
		cur = cur.rest
	}
}

// Run executes the chain from head to tail. The first stage to return without
// calling next short-circuits the chain. res may be nil; a fresh slot is used so
// short-circuit errors are still observable via the chain's own return.
func (c *Chain) Run(ctx context.Context, req ToolRequest, res *ToolResult) error {
	if c == nil || c.head == nil {
		return nil
	}
	if res == nil {
		res = &ToolResult{Request: req}
	}
	return c.head.run(ctx, req, res)
}

func (n *chainNode) run(ctx context.Context, req ToolRequest, res *ToolResult) error {
	return n.fn(ctx, req, res, func() error {
		if n.rest == nil {
			return nil
		}
		return n.rest.run(ctx, req, res)
	})
}

// Pipeline is a stage-keyed collection of Chains. It lets distinct lifecycle
// phases register separately, decoupled from call order.
type Pipeline struct {
	chains map[Stage]*Chain
}

// NewPipeline constructs an empty pipeline.
func NewPipeline() *Pipeline {
	return &Pipeline{chains: map[Stage]*Chain{}}
}

// Register installs one interceptor at a single stage. StageAll installs at every
// stage. The returned disposer restores the prior state exactly, so tests and hot
// reload can add/remove stages without restarting the process.
func (p *Pipeline) Register(stage Stage, fn InterceptFn) func() {
	if fn == nil {
		return func() {}
	}
	if stage == StageAll {
		var disposers []func()
		for _, s := range []Stage{StagePreExecute, StagePostExecute} {
			disposers = append(disposers, p.Register(s, fn))
		}
		return func() {
			for i := len(disposers) - 1; i >= 0; i-- {
				disposers[i]()
			}
		}
	}
	chain := p.chains[stage]
	if chain == nil {
		chain = &Chain{}
		p.chains[stage] = chain
	}
	return chain.Append(fn)
}

// Run executes every interceptor for the given stage in registration order. The
// first to short-circuit stops the chain and returns its error. res is the mutable
// result slot (nil allowed) that post-execute stages annotate.
func (p *Pipeline) Run(stage Stage, ctx context.Context, req ToolRequest, res *ToolResult) error {
	if p == nil {
		return nil
	}
	chain, ok := p.chains[stage]
	if !ok || chain == nil || chain.head == nil {
		return nil
	}
	return chain.Run(ctx, req, res)
}

// ShortCircuit is a typed stop returned by a stage that ends the chain and replaces
// the result. nil err denotes a silent stop with no error; a non-empty msg is
// surfaced as the tool result with isErr controlling failure. Implementers return
// it directly from their InterceptFn; callers detect it via errors.As.
type ShortCircuit struct {
	msg   string
	isErr bool
	err   error
}

func (s *ShortCircuit) Error() string {
	if s == nil {
		return "tool pipeline short-circuit"
	}
	if s.err != nil {
		return s.err.Error()
	}
	if s.msg != "" {
		return s.msg
	}
	return "tool pipeline short-circuit"
}

func (s *ShortCircuit) Unwrap() error {
	if s == nil {
		return nil
	}
	return s.err
}

// ToolError carries the result replacement so the pipeline wrapper can surface it.
func (s *ShortCircuit) ToolError() (string, bool) {
	if s == nil {
		return "", false
	}
	return s.msg, s.isErr
}

// ShortCircuitDeny returns the fail-closed primitive: a stage stops the call and
// tells the model why.
func ShortCircuitDeny(msg string) *ShortCircuit {
	if msg == "" {
		msg = "denied by tool pipeline"
	}
	return &ShortCircuit{msg: msg, isErr: true, err: fmt.Errorf("%s", msg)}
}

// ShortCircuitApprove stops the remaining stage chain but lets the call proceed,
// without altering the eventual result.
func ShortCircuitApprove() *ShortCircuit {
	return &ShortCircuit{err: fmt.Errorf("approved by tool pipeline")}
}

// ShortCircuitResult replaces the raw stage result with msg, keeping the error flag.
func ShortCircuitResult(msg string, isErr bool) *ShortCircuit {
	return &ShortCircuit{msg: msg, isErr: isErr, err: fmt.Errorf("%s", msg)}
}
