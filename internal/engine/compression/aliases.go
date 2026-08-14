// Package session is the Stage-1 namespace for session-lifecycle types in
// package engine. See ../REFACTOR_PLAN.md.
package compression

// Compressor is a shorter name for SessionCompressor.
type Compressor = SessionCompressor

// NewCompressor returns a session compressor using the named strategy.
func NewCompressor(strategy CompressStrategy) *Compressor {
	return NewSessionCompressor(strategy)
}
