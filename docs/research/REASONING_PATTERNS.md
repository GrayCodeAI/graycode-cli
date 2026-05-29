# Advanced Reasoning Patterns for Hawk

Design document for implementing research-backed reasoning patterns in the hawk engine.

---

## Overview

Hawk currently implements basic ReAct-style reasoning (Reason + Act loop). This document outlines three advanced reasoning patterns from recent research that would significantly improve hawk's problem-solving capabilities:

1. **Tree of Thoughts (ToT)** - Branching exploration with backtracking
2. **Graph of Thoughts (GoT)** - Graph-based reasoning with merging
3. **Reflexion** - Self-reflection and iterative improvement

---

## 1. Tree of Thoughts (ToT)

**Paper**: "Tree of Thoughts: Deliberate Problem Solving with Large Language Models" (Yao et al., 2023)

### Concept

Instead of a single linear chain of thoughts, ToT explores multiple reasoning paths simultaneously, forming a tree structure. At each step, the agent:
1. Generates multiple candidate thoughts
2. Evaluates each candidate
3. Selects the most promising path(s) to explore
4. Backtracks if a path proves unfruitful

### Architecture

```
Problem
├── Thought 1.1 → Evaluate → [promising]
│   ├── Thought 2.1 → Evaluate → [promising]
│   │   └── Thought 3.1 → Solution ✓
│   └── Thought 2.2 → Evaluate → [dead end] ✗
├── Thought 1.2 → Evaluate → [promising]
│   └── Thought 2.3 → Evaluate → [promising]
│       └── Thought 3.2 → Solution ✓
└── Thought 1.3 → Evaluate → [dead end] ✗
```

### Implementation Plan

**Package**: `internal/engine/tot/`

```go
// TreeOfThought manages branching exploration.
type TreeOfThought struct {
    engine     *Engine
    maxDepth   int
    maxBranch  int
    evaluator  ThoughEvaluator
    strategy   SearchStrategy // BFS, DFS, or Beam
}

// Thought represents a single reasoning step.
type Thought struct {
    Content   string
    Score     float64
    Children  []*Thought
    Parent    *Thought
    Depth     int
}

// ThoughEvaluator scores thoughts.
type ThoughtEvaluator interface {
    Evaluate(ctx context.Context, thought *Thought, problem string) (float64, error)
}

// SearchStrategy defines how to explore the tree.
type SearchStrategy interface {
    Select(nodes []*Thought, k int) []*Thought
}
```

**Strategies**:
- **BFS**: Explore all nodes at current depth before going deeper
- **DFS**: Explore deepest path first, backtracking on dead ends
- **Beam Search**: Keep top-k most promising paths at each depth

### Integration

```go
// In engine options
WithTreeOfThought(maxDepth int, maxBranch int, strategy string)

// Usage
result, err := engine.SolveWithTOT(ctx, problem, engine.ToTOptions{
    MaxDepth:  3,
    MaxBranch: 3,
    Strategy:  "beam",
})
```

---

## 2. Graph of Thoughts (GoT)

**Paper**: "Graph of Thoughts: Solving Elaborate Problems with Large Language Models" (Besta et al., 2024)

### Concept

GoT extends ToT by allowing thoughts to be merged and refined, forming a directed acyclic graph (DAG). This enables:
- **Aggregation**: Combining multiple thoughts into a better one
- **Refinement**: Improving a thought based on feedback
- **Splitting**: Breaking a complex thought into sub-problems

### Architecture

```
Problem
├── Thought A ──────────────────────┐
├── Thought B ──→ Merge(A,B) ──→ Refine ──→ Solution
└── Thought C ──→ Split(C1,C2)
                  ├── C1 ──→ Refine ──→ Solution
                  └── C2 ──→ Merge(C2,B) ──→ Solution
```

### Implementation Plan

**Package**: `internal/engine/got/`

```go
// GraphOfThought manages graph-based reasoning.
type GraphOfThought struct {
    engine    *Engine
    graph     *ThoughtGraph
    operations []Operation
}

// ThoughtGraph is a DAG of thoughts.
type ThoughtGraph struct {
    Nodes map[string]*ThoughtNode
    Edges []ThoughtEdge
}

// ThoughtNode represents a thought in the graph.
type ThoughtNode struct {
    ID       string
    Content  string
    Score    float64
    Status   NodeStatus // pending, evaluated, pruned, merged
}

// Operation transforms thoughts.
type Operation interface {
    Apply(ctx context.Context, inputs []*ThoughtNode) ([]*ThoughtNode, error)
}

// Available operations:
// - Generate: Create new thoughts from existing ones
// - Aggregate: Merge multiple thoughts into one
// - Refine: Improve a thought based on feedback
// - Split: Break a thought into sub-problems
// - Evaluate: Score a thought
// - Prune: Remove low-quality thoughts
```

### Integration

```go
// Usage
result, err := engine.SolveWithGOT(ctx, problem, engine.GOTOptions{
    Operations: []string{"generate", "evaluate", "aggregate", "refine"},
    MaxNodes:   20,
    PruneThreshold: 0.3,
})
```

---

## 3. Reflexion

**Paper**: "Reflexion: Language Agents with Verbal Reinforcement Learning" (Shinn et al., 2023)

### Concept

Reflexion adds self-reflection to the agent loop. After each attempt:
1. The agent executes a task
2. Evaluates the result
3. Generates a verbal reflection on what went wrong
4. Uses the reflection to improve the next attempt

### Architecture

```
Attempt 1: Execute → Fail → Reflect("I forgot to handle edge case X")
Attempt 2: Execute (with reflection context) → Fail → Reflect("Need to check Y")
Attempt 3: Execute (with reflections 1+2) → Success ✓
```

### Implementation Plan

**Package**: `internal/engine/reflexion/`

```go
// ReflexionAgent iteratively improves through self-reflection.
type ReflexionAgent struct {
    engine        *Engine
    maxAttempts   int
    reflector     Reflector
    memory        *ReflectionMemory
}

// Reflector generates verbal reflections on failures.
type Reflector interface {
    Reflect(ctx context.Context, task string, attempt *Attempt, result *Result) (string, error)
}

// ReflectionMemory stores reflections for future use.
type ReflectionMemory struct {
    reflections []Reflection
    maxMemory   int
}

// Reflection stores a lesson learned.
type Reflection struct {
    Task      string
    Attempt   int
    Content   string
    Success   bool
    Timestamp time.Time
}

// Attempt represents a single try at solving a task.
type Attempt struct {
    Number    int
    Actions   []Action
    Result    *Result
    Reflection string
}
```

### Integration

```go
// In engine options
WithReflexion(maxAttempts int)

// Usage
result, err := engine.SolveWithReflexion(ctx, task, engine.ReflexionOptions{
    MaxAttempts:    3,
    StoreReflections: true,
})
```

---

## Implementation Priority

| Pattern | Complexity | Impact | Priority |
|---------|-----------|--------|----------|
| Reflexion | Medium | High | 1 (implement first) |
| Tree of Thoughts | Medium | Medium | 2 |
| Graph of Thoughts | High | High | 3 |

### Recommended Order

1. **Reflexion** (Week 1-2)
   - Simplest to implement
   - Highest impact for coding tasks
   - Builds on existing agent loop

2. **Tree of Thoughts** (Week 3-4)
   - Moderate complexity
   - Good for exploration-heavy tasks
   - Can reuse Reflexion's evaluation

3. **Graph of Thoughts** (Week 5-6)
   - Most complex
   - Best for complex multi-step problems
   - Builds on ToT infrastructure

---

## Testing Strategy

Each pattern needs:
1. **Unit tests** for core logic
2. **Integration tests** with mock LLM
3. **Benchmark tests** comparing to baseline
4. **Example tasks** demonstrating improvement

### Test Cases

**Reflexion**:
- Code generation with iterative refinement
- Bug fixing with learning from failures
- Task decomposition with reflection

**Tree of Thoughts**:
- Puzzle solving (e.g., Game of 24)
- Creative writing with exploration
- Strategy planning with backtracking

**Graph of Thoughts**:
- Document synthesis from multiple sources
- Complex debugging with hypothesis merging
- System design with component aggregation

---

## Dependencies

All patterns depend on:
- Existing `internal/engine/` infrastructure
- LLM provider abstraction (via eyrie)
- Conversation management
- Token budget tracking

New dependencies:
- None (all patterns use existing LLM calls)

---

## Success Metrics

| Pattern | Metric | Target |
|---------|--------|--------|
| Reflexion | Task success rate | +20% vs baseline |
| Reflexion | Average attempts to success | < 3 |
| ToT | Solution quality score | +15% vs baseline |
| ToT | Exploration efficiency | < 10 thoughts per solution |
| GoT | Complex task success rate | +25% vs baseline |
| GoT | Graph convergence | < 15 nodes per solution |

---

## References

1. Yao et al. (2023). "Tree of Thoughts: Deliberate Problem Solving with Large Language Models"
2. Besta et al. (2024). "Graph of Thoughts: Solving Elaborate Problems with Large Language Models"
3. Shinn et al. (2023). "Reflexion: Language Agents with Verbal Reinforcement Learning"
4. Hao et al. (2023). "Reasoning via Planning with Large Language Models"
5. Zhou et al. (2024). "Language Agent Tree Search Unifies Reasoning, Acting, and Planning"
