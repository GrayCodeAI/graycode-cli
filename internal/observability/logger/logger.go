// Package logger provides structured logging with levels.
package logger

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Level represents a log level.
type Level int

const (
	Debug Level = iota
	Info
	Warn
	Error
	Fatal
)

func (l Level) String() string {
	switch l {
	case Debug:
		return "DEBUG"
	case Info:
		return "INFO"
	case Warn:
		return "WARN"
	case Error:
		return "ERROR"
	case Fatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// Logger is a structured logger.
type Logger struct {
	mu     sync.Mutex
	level  Level
	output io.Writer
	prefix string
}

// New creates a new logger.
func New(output io.Writer, level Level) *Logger {
	if output == nil {
		output = os.Stderr
	}
	return &Logger{
		level:  level,
		output: output,
	}
}

// Default returns a default logger writing to stderr at Info level.
func Default() *Logger {
	return New(os.Stderr, Info)
}

// LoggerDeps holds configurable dependencies for Logger.
// Zero values are filled with production defaults by NewWithDeps.
type LoggerDeps struct {
	Output io.Writer // default: os.Stderr
	Level  Level     // default: Info
}

// NewWithDeps creates a new logger from a deps struct.
// Zero-value fields are replaced with production defaults.
// Note: Level zero-value is Debug (iota=0), so use Default() for Info-level logger.
func NewWithDeps(deps LoggerDeps) *Logger {
	if deps.Output == nil {
		deps.Output = os.Stderr
	}
	return &Logger{
		level:  deps.Level,
		output: deps.Output,
	}
}

// SetLevel sets the minimum log level.
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// WithPrefix returns a new logger with the given prefix.
func (l *Logger) WithPrefix(prefix string) *Logger {
	return &Logger{
		level:  l.level,
		output: l.output,
		prefix: prefix,
	}
}

func (l *Logger) log(level Level, msg string, fields map[string]interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Logging must never panic — recover unconditionally.
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "logger panic recovered: %v\n", r)
		}
	}()

	timestamp := time.Now().Format(time.RFC3339)
	output := fmt.Sprintf("%s [%s]", timestamp, level)
	if l.prefix != "" {
		output += " " + l.prefix
	}
	output += " " + msg

	if len(fields) > 0 {
		output += " {"
		first := true
		for k, v := range fields {
			if !first {
				output += ", "
			}
			output += fmt.Sprintf("%s=%v", k, v)
			first = false
		}
		output += "}"
	}

	_, _ = fmt.Fprintln(l.output, output)
}

// SetOutputForTesting replaces the logger's output writer.
// Test-only; not for production use.
func (l *Logger) SetOutputForTesting(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.output = w
}

// FlushForTesting synchronously flushes any buffered output.
// Test-only; not for production use.
func (l *Logger) FlushForTesting() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if bw, ok := l.output.(*bufferedWriter); ok {
		bw.Flush()
	}
}

// Debug logs a debug message.
func (l *Logger) Debug(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(Debug, msg, f)
}

// Info logs an info message.
func (l *Logger) Info(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(Info, msg, f)
}

// Warn logs a warning message.
func (l *Logger) Warn(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(Warn, msg, f)
}

// Error logs an error message.
func (l *Logger) Error(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(Error, msg, f)
}

// Fatal logs a fatal message and exits.
func (l *Logger) Fatal(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(Fatal, msg, f)
	os.Exit(1)
}
