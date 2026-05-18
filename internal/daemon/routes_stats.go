package daemon

import (
	"net/http"
	"strconv"
	"time"

	"github.com/GrayCodeAI/hawk/internal/observability"
)

// handleStats handles GET /v1/stats — get aggregated usage statistics.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	// Parse ?days=30 query param (default 30)
	days := 30
	if v := r.URL.Query().Get("days"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			days = parsed
		}
	}

	traces, err := analytics.GetTraces()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{
			Error:   "failed to load analytics",
			Code:    "internal_error",
			Details: err.Error(),
		})
		return
	}

	cutoff := time.Now().AddDate(0, 0, -days)

	var totalSessions int
	var totalMessages int
	var totalToolCalls int
	var totalCostUSD float64
	activeDays := make(map[string]struct{})
	modelStats := make(map[string]*ModelStatResp)

	for _, t := range traces {
		if t.StartTime.Before(cutoff) {
			continue
		}

		totalSessions++
		totalMessages += t.MessageCount
		totalToolCalls += t.ToolCalls
		totalCostUSD += t.CostUSD

		// Track active days
		dayKey := t.StartTime.Format("2006-01-02")
		activeDays[dayKey] = struct{}{}

		// Aggregate by model
		if t.Model != "" {
			ms, ok := modelStats[t.Model]
			if !ok {
				ms = &ModelStatResp{Model: t.Model}
				modelStats[t.Model] = ms
			}
			ms.Requests++
			ms.CostUSD += t.CostUSD
		}
	}

	// Convert model stats map to slice
	models := make([]ModelStatResp, 0, len(modelStats))
	for _, ms := range modelStats {
		models = append(models, *ms)
	}

	writeJSON(w, http.StatusOK, StatsResponse{
		TotalSessions:  totalSessions,
		TotalMessages:  totalMessages,
		TotalToolCalls: totalToolCalls,
		TotalCostUSD:   totalCostUSD,
		ActiveDays:     len(activeDays),
		Models:         models,
	})
}
