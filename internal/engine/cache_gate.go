package engine

// Prompt-cache break-even gate: provider-native caching charges a write
// premium on cached input (Anthropic 5m: write=1.25x, read=0.1x) and pays off
// only when the stable prefix is reused. The deterministic client planner in
// cache_planner.go (planCache / cacheDecision) implements this arithmetic:
// segments the stable prefix, computes breakpoints, and enables caching only
// when the expected reuse count beats the write premium. Full wire-format
// lowering and fleet-wide key-sharding remain eyrie-side.
