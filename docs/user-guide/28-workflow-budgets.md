# Workflow Budgets

Hawk exposes several independent limits. Configure the smallest useful scope
for automation and distinguish them when diagnosing termination.

| Budget | Limits | Purpose |
|---|---|---|
| Turns | Model/agent iterations | Stops runaway reasoning loops |
| Tool calls | Tool operations in a workflow | Bounds action volume |
| Agent depth | Nested sub-agent levels | Bounds recursive delegation |
| Wall clock | Operation duration | Prevents hung work |
| Tokens | Context and completion usage | Controls context cost and compaction |
| Cost | Provider spend in USD | Hard financial boundary |

A limit reached is not equivalent to a provider error. Logs and lifecycle
events should identify the specific budget and current/maximum values. A retry
must not reset a consumed budget, and cancellation must remain authoritative.
