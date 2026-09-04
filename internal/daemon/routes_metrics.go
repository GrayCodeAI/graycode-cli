package daemon

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/observability/metrics"
)

// handleMetrics handles GET /v1/metrics. It exposes daemon-level metrics in
// Prometheus text exposition format. The output includes counters, gauges,
// and timers from the daemon's metrics registry, plus runtime-derived values
// (active sessions, concurrency slots used, process uptime).
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	// Allow Prometheus format override: ?format=prometheus (default) or
	// ?format=json for human-readable JSON.
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "prometheus"
	}

	if strings.ToLower(format) == "json" {
		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, http.StatusOK, s.metrics.Snapshot())
		return
	}

	// Prometheus text exposition format.
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	// Emit Prometheus-style metric lines from the registry snapshot.
	snap := s.metrics.Snapshot()
	names := make([]string, 0, len(snap))
	for name := range snap {
		names = append(names, name)
	}
	sort.Strings(names)

	var sb strings.Builder
	for _, name := range names {
		val := snap[name]
		switch v := val.(type) {
		case map[string]int64:
			// Counter or gauge: {value: N}
			if counterVal, ok := v["value"]; ok {
				sb.WriteString(fmt.Sprintf("# TYPE %s counter\n", sanitizeMetricName(name)))
				sb.WriteString(fmt.Sprintf("%s %d\n", sanitizeMetricName(name), counterVal))
			}
		case metrics.TimerStats:
			// Timer: count, total, mean, min, max
			sb.WriteString(fmt.Sprintf("# TYPE %s histogram\n", sanitizeMetricName(name)))
			sb.WriteString(fmt.Sprintf("%s_count %d\n", sanitizeMetricName(name), v.Count))
		case metrics.GaugeStats:
			sb.WriteString(fmt.Sprintf("# TYPE %s gauge\n", sanitizeMetricName(name)))
			sb.WriteString(fmt.Sprintf("%s %d\n", sanitizeMetricName(name), v.Value))
		}
	}

	// Emit runtime-derived metrics.
	s.emitRuntimeMetrics(&sb)

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(sb.String()))
}

// emitRuntimeMetrics appends runtime-derived gauge metrics (active sessions,
// concurrency usage, uptime) to the Prometheus output buffer.
func (s *Server) emitRuntimeMetrics(sb *strings.Builder) {
	// Active sessions gauge
	activeSessions := 0
	s.sessions.Range(func(_, _ any) bool {
		activeSessions++
		return true
	})

	sb.WriteString(fmt.Sprintf("# TYPE graycode_daemon_active_sessions gauge\n"))
	sb.WriteString(fmt.Sprintf("graycode_daemon_active_sessions %d\n", activeSessions))

	// Concurrency slots used
	sb.WriteString(fmt.Sprintf("# TYPE graycode_daemon_chat_concurrency_used gauge\n"))
	sb.WriteString(fmt.Sprintf("graycode_daemon_chat_concurrency_used %d\n", len(s.concurrencySem)))

	// Uptime
	sb.WriteString(fmt.Sprintf("# TYPE graycode_daemon_uptime_seconds gauge\n"))
	sb.WriteString(fmt.Sprintf("graycode_daemon_uptime_seconds %.0f\n", time.Since(s.startedAt).Seconds()))
}

// sanitizeMetricName converts a dotted metric name to Prometheus naming
// conventions (underscores, alphanumeric + underscore only).
func sanitizeMetricName(name string) string {
	// Replace dots and hyphens with underscores, strip non-alphanumeric chars.
	s := strings.NewReplacer(".", "_", "-", "_").Replace(name)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
