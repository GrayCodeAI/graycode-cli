package observability

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// LogLevel represents the severity of a log entry.
type LogLevel int

const (
	LevelDebug LogLevel = 0
	LevelInfo  LogLevel = 1
	LevelWarn  LogLevel = 2
	LevelError LogLevel = 3
	LevelFatal LogLevel = 4
)

// Predefined field keys for structured logging.
const (
	FieldTool     = "tool"
	FieldFile     = "file"
	FieldDuration = "duration"
	FieldTokens   = "tokens"
	FieldModel    = "model"
	FieldProvider = "provider"
	FieldError    = "error"
	FieldCost     = "cost"
)

// LogEntry represents a single structured log record.
type LogEntry struct {
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Timestamp time.Time              `json:"timestamp"`
	SessionID string                 `json:"session_id,omitempty"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
	Caller    string                 `json:"caller,omitempty"`
}

// StructuredLogger provides context-rich structured logging.
type StructuredLogger struct {
	Level     LogLevel
	Output    io.Writer
	Format    string // "json" or "text"
	Fields    map[string]interface{}
	SessionID string
	mu        sync.Mutex
}

// NewStructuredLogger creates a new StructuredLogger with the given level and output.
func NewStructuredLogger(level LogLevel, output io.Writer) *StructuredLogger {
	return &StructuredLogger{
		Level:  level,
		Output: output,
		Format: "json",
		Fields: make(map[string]interface{}),
	}
}

// WithFields returns a new logger with additional context fields merged in.
func (l *StructuredLogger) WithFields(fields map[string]interface{}) *StructuredLogger {
	merged := make(map[string]interface{}, len(l.Fields)+len(fields))
	for k, v := range l.Fields {
		merged[k] = v
	}
	for k, v := range fields {
		merged[k] = v
	}
	return &StructuredLogger{
		Level:     l.Level,
		Output:    l.Output,
		Format:    l.Format,
		Fields:    merged,
		SessionID: l.SessionID,
	}
}

// WithSession returns a new logger with the given session ID.
func (l *StructuredLogger) WithSession(sessionID string) *StructuredLogger {
	return &StructuredLogger{
		Level:     l.Level,
		Output:    l.Output,
		Format:    l.Format,
		Fields:    l.Fields,
		SessionID: sessionID,
	}
}

// Debug logs a message at debug level.
func (l *StructuredLogger) Debug(msg string, fields ...map[string]interface{}) {
	l.log(LevelDebug, msg, fields...)
}

// Info logs a message at info level.
func (l *StructuredLogger) Info(msg string, fields ...map[string]interface{}) {
	l.log(LevelInfo, msg, fields...)
}

// Warn logs a message at warn level.
func (l *StructuredLogger) Warn(msg string, fields ...map[string]interface{}) {
	l.log(LevelWarn, msg, fields...)
}

// Error logs a message at error level.
func (l *StructuredLogger) Error(msg string, fields ...map[string]interface{}) {
	l.log(LevelError, msg, fields...)
}

func (l *StructuredLogger) log(level LogLevel, msg string, fields ...map[string]interface{}) {
	if level < l.Level {
		return
	}

	merged := make(map[string]interface{}, len(l.Fields))
	for k, v := range l.Fields {
		merged[k] = v
	}
	for _, f := range fields {
		for k, v := range f {
			merged[k] = v
		}
	}

	// Capture caller information (skip 2 frames: log -> Debug/Info/Warn/Error -> caller).
	caller := ""
	if _, file, line, ok := runtime.Caller(2); ok {
		caller = fmt.Sprintf("%s:%d", filepath.Base(file), line)
	}

	entry := LogEntry{
		Level:     levelString(level),
		Message:   msg,
		Timestamp: time.Now(),
		SessionID: l.SessionID,
		Fields:    merged,
		Caller:    caller,
	}

	var output string
	if l.Format == "text" {
		output = l.formatText(entry)
	} else {
		output = l.formatJSON(entry)
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = fmt.Fprintln(l.Output, output)
}

func (l *StructuredLogger) formatJSON(entry LogEntry) string {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Sprintf(`{"level":"error","message":"failed to marshal log entry: %s"}`, err)
	}
	return string(data)
}

func (l *StructuredLogger) formatText(entry LogEntry) string {
	var sb strings.Builder
	sb.WriteString(entry.Timestamp.Format(time.RFC3339))
	sb.WriteString(" [")
	sb.WriteString(entry.Level)
	sb.WriteString("] ")
	sb.WriteString(entry.Message)

	for k, v := range entry.Fields {
		sb.WriteString(" ")
		sb.WriteString(k)
		sb.WriteString("=")
		switch val := v.(type) {
		case time.Duration:
			sb.WriteString(val.String())
		case error:
			sb.WriteString(val.Error())
		default:
			sb.WriteString(fmt.Sprintf("%v", val))
		}
	}

	return sb.String()
}

func levelString(level LogLevel) string {
	switch level {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// ParseLevel converts a string to a LogLevel.
func ParseLevel(s string) LogLevel {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	case "fatal":
		return LevelFatal
	default:
		return LevelInfo
	}
}

// AgentLogger wraps a StructuredLogger with automatic agent context.
type AgentLogger struct {
	Logger *StructuredLogger
	Turn   int
	Model  string
}

// LogToolCall logs a tool invocation with duration and optional error.
func (a *AgentLogger) LogToolCall(tool, file string, duration time.Duration, err error) {
	fields := map[string]interface{}{
		FieldTool:     tool,
		FieldFile:     file,
		FieldDuration: duration.String(),
		"turn":        a.Turn,
	}
	if err != nil {
		fields[FieldError] = err.Error()
		a.Logger.Error("tool.execute", fields)
	} else {
		a.Logger.Info("tool.execute", fields)
	}
}

// LogAPICall logs an LLM API call with token usage and cost.
func (a *AgentLogger) LogAPICall(model string, tokens int, cost float64, duration time.Duration) {
	a.Logger.Info("api.call", map[string]interface{}{
		FieldModel:    model,
		FieldTokens:   tokens,
		FieldCost:     cost,
		FieldDuration: duration.String(),
		"turn":        a.Turn,
	})
}

// LogCompaction logs a context compaction event.
func (a *AgentLogger) LogCompaction(before, after int) {
	a.Logger.Info("context.compact", map[string]interface{}{
		"before": before,
		"after":  after,
		"turn":   a.Turn,
	})
}

// LogPermission logs a permission request and its outcome.
func (a *AgentLogger) LogPermission(tool string, granted bool) {
	a.Logger.Info("permission.check", map[string]interface{}{
		FieldTool: tool,
		"granted": granted,
		"turn":    a.Turn,
	})
}

// RotatingWriter implements io.Writer with automatic log file rotation.
type RotatingWriter struct {
	Dir      string
	Prefix   string
	MaxSize  int64
	MaxFiles int

	mu      sync.Mutex
	file    *os.File
	size    int64
	current string
}

// NewRotatingWriter creates a RotatingWriter with sensible defaults.
func NewRotatingWriter(dir, prefix string) (*RotatingWriter, error) {
	rw := &RotatingWriter{
		Dir:      dir,
		Prefix:   prefix,
		MaxSize:  10 * 1024 * 1024, // 10MB
		MaxFiles: 5,
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating log directory: %w", err)
	}
	if err := rw.openNew(); err != nil {
		return nil, err
	}
	return rw, nil
}

// Write implements io.Writer. It rotates the file when MaxSize is exceeded.
func (rw *RotatingWriter) Write(p []byte) (n int, err error) {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	if rw.size+int64(len(p)) > rw.MaxSize {
		if rotateErr := rw.rotate(); rotateErr != nil {
			return 0, rotateErr
		}
	}

	n, err = rw.file.Write(p)
	rw.size += int64(n)
	return n, err
}

// Close closes the underlying file.
func (rw *RotatingWriter) Close() error {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	if rw.file != nil {
		return rw.file.Close()
	}
	return nil
}

func (rw *RotatingWriter) openNew() error {
	name := fmt.Sprintf("%s.log", rw.Prefix)
	path := filepath.Join(rw.Dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("stat log file: %w", err)
	}
	rw.file = f
	rw.size = info.Size()
	rw.current = path
	return nil
}

func (rw *RotatingWriter) rotate() error {
	if rw.file != nil {
		_ = rw.file.Close()
	}

	// Shift existing rotated files.
	for i := rw.MaxFiles - 1; i >= 1; i-- {
		src := filepath.Join(rw.Dir, fmt.Sprintf("%s.%d.log", rw.Prefix, i))
		dst := filepath.Join(rw.Dir, fmt.Sprintf("%s.%d.log", rw.Prefix, i+1))
		_ = os.Rename(src, dst)
	}

	// Rename current to .1.log.
	rotated := filepath.Join(rw.Dir, fmt.Sprintf("%s.1.log", rw.Prefix))
	_ = os.Rename(rw.current, rotated)

	// Remove excess files.
	excess := filepath.Join(rw.Dir, fmt.Sprintf("%s.%d.log", rw.Prefix, rw.MaxFiles+1))
	_ = os.Remove(excess)

	return rw.openNew()
}
