package localize

// Option configures a Localize call using the functional-options pattern.
type Option func(*config)

type config struct {
	maxFiles   int
	maxSymbols int
	language   string // empty = auto-detect from extension
	contextLines int  // lines of context around a symbol for Stage 3
}

func defaults() *config {
	return &config{
		maxFiles:     10,
		maxSymbols:   20,
		language:     "",
		contextLines: 3,
	}
}

// WithMaxFiles sets the maximum number of files returned from Stage 1.
func WithMaxFiles(n int) Option {
	return func(c *config) {
		if n > 0 {
			c.maxFiles = n
		}
	}
}

// WithMaxSymbols sets the maximum number of symbols returned from Stage 2.
func WithMaxSymbols(n int) Option {
	return func(c *config) {
		if n > 0 {
			c.maxSymbols = n
		}
	}
}

// WithLanguage restricts symbol extraction to a single language.
// Valid values: "go", "python", "typescript", "javascript", "rust", "java".
// An empty string means auto-detect from extension (the default).
func WithLanguage(lang string) Option {
	return func(c *config) {
		c.language = lang
	}
}

// WithContextLines sets the number of surrounding lines included in
// Stage 3 CodeBlocks.
func WithContextLines(n int) Option {
	return func(c *config) {
		if n >= 0 {
			c.contextLines = n
		}
	}
}
