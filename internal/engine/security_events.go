package engine

import (
	"log/slog"
	"sync"

	"github.com/GrayCodeAI/graycode-cli/internal/securitylog"
	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

// securityLog is the session's tamper-evident event log. It is opened lazily
// on the first recorded event so sessions that never deny an action do not
// touch the filesystem. A nil log means recording is unavailable (log open
// failed) and events are dropped with a single warning.
type securityLog struct {
	mu   sync.Mutex
	log  *securitylog.Log
	warn bool
}

// record opens the log on first use and appends an event.
func (sl *securityLog) record(severity securitylog.EventSeverity, eventType, detail, tool, sessionID string) {
	if sl == nil {
		return
	}
	sl.mu.Lock()
	defer sl.mu.Unlock()
	if sl.log == nil {
		l, err := securitylog.New(securitylog.DefaultDir())
		if err != nil {
			if !sl.warn {
				sl.warn = true
				slog.Warn("security event log unavailable; events will be dropped", "error", err)
			}
			return
		}
		sl.log = l
	}
	if _, err := sl.log.Append(severity, eventType, detail, tool, sessionID); err != nil {
		if !sl.warn {
			sl.warn = true
			slog.Warn("security event append failed; events will be dropped", "error", err)
		}
	}
}

// recordSecurityDenial records a permission or approval denial to the
// tamper-evident security event log.
func (s *Session) recordSecurityDenial(tc types.ToolCall, stage string, reason string) {
	if s == nil {
		return
	}
	s.secLog().record(securitylog.SeverityWarning, "denied", reason, tc.Name, s.executionGraphSessionID())
}

// secLog returns the session's security event log, creating it on first use.
func (s *Session) secLog() *securityLog {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sec == nil {
		s.sec = &securityLog{}
	}
	return s.sec
}
