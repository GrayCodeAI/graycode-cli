package analytics

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// SLO defines a Service Level Objective for a performance metric.
type SLO struct {
	Name         string
	Metric       string
	Target       float64
	Window       time.Duration
	Current      float64
	Measurements []float64
	Status       string // "met", "at_risk", "violated"
}

// BudgetViolation records when an SLO was violated.
type BudgetViolation struct {
	SLO       string
	Expected  float64
	Actual    float64
	Timestamp time.Time
	Duration  time.Duration
}

// PerfBudget tracks agent performance against defined SLOs.
type PerfBudget struct {
	SLOs       map[string]*SLO
	Violations []BudgetViolation
	mu         sync.RWMutex
}

// NewPerfBudget creates a PerfBudget with default SLOs.
func NewPerfBudget() *PerfBudget {
	pb := &PerfBudget{
		SLOs:       make(map[string]*SLO),
		Violations: []BudgetViolation{},
	}

	// Default SLOs
	pb.SLOs["response_time"] = &SLO{
		Name:         "Response Time (P95)",
		Metric:       "response_time",
		Target:       5.0,
		Window:       time.Hour,
		Measurements: []float64{},
		Status:       "met",
	}
	pb.SLOs["accuracy"] = &SLO{
		Name:         "Success Rate",
		Metric:       "accuracy",
		Target:       90.0,
		Window:       time.Hour,
		Measurements: []float64{},
		Status:       "met",
	}
	pb.SLOs["cost_per_task"] = &SLO{
		Name:         "Cost per Task",
		Metric:       "cost_per_task",
		Target:       0.50,
		Window:       time.Hour,
		Measurements: []float64{},
		Status:       "met",
	}
	pb.SLOs["token_efficiency"] = &SLO{
		Name:         "Token Efficiency",
		Metric:       "token_efficiency",
		Target:       5000.0,
		Window:       time.Hour,
		Measurements: []float64{},
		Status:       "met",
	}
	pb.SLOs["tool_error_rate"] = &SLO{
		Name:         "Tool Error Rate",
		Metric:       "tool_error_rate",
		Target:       5.0,
		Window:       time.Hour,
		Measurements: []float64{},
		Status:       "met",
	}

	return pb
}

// Record adds a measurement for the given metric.
func (pb *PerfBudget) Record(metric string, value float64) {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	slo, ok := pb.SLOs[metric]
	if !ok {
		return
	}

	slo.Measurements = append(slo.Measurements, value)
	slo.Current = pb.computeCurrent(slo)
	pb.evaluateSLO(slo)
}

// computeCurrent calculates the current metric value based on the SLO type.
func (pb *PerfBudget) computeCurrent(slo *SLO) float64 {
	if len(slo.Measurements) == 0 {
		return 0
	}

	switch slo.Metric {
	case "response_time":
		// P95
		return percentile(slo.Measurements, 95)
	case "accuracy":
		// Average (success rate as percentage)
		return average(slo.Measurements)
	case "cost_per_task":
		// Average cost
		return average(slo.Measurements)
	case "token_efficiency":
		// Average tokens per task
		return average(slo.Measurements)
	case "tool_error_rate":
		// Average error rate
		return average(slo.Measurements)
	default:
		return average(slo.Measurements)
	}
}

// evaluateSLO determines the SLO status and records violations.
func (pb *PerfBudget) evaluateSLO(slo *SLO) {
	if len(slo.Measurements) == 0 {
		slo.Status = "met"
		return
	}

	var violated bool
	var atRisk bool

	switch slo.Metric {
	case "accuracy":
		// Higher is better: target is a minimum
		violated = slo.Current < slo.Target
		// At risk if within 5 percentage points above target
		atRisk = slo.Current >= slo.Target && slo.Current < slo.Target+5.0
	default:
		// Lower is better: target is a maximum
		violated = slo.Current > slo.Target
		atRisk = slo.Current > slo.Target*0.8 && !violated
	}

	if violated {
		slo.Status = "violated"
		pb.Violations = append(pb.Violations, BudgetViolation{
			SLO:       slo.Name,
			Expected:  slo.Target,
			Actual:    slo.Current,
			Timestamp: time.Now(),
		})
	} else if atRisk {
		slo.Status = "at_risk"
	} else {
		slo.Status = "met"
	}
}

// Check evaluates all SLOs and returns status per SLO.
func (pb *PerfBudget) Check() map[string]string {
	pb.mu.RLock()
	defer pb.mu.RUnlock()

	result := make(map[string]string)
	for key, slo := range pb.SLOs {
		result[key] = slo.Status
	}
	return result
}

// GetViolations returns violations that occurred since the given time.
func (pb *PerfBudget) GetViolations(since time.Time) []BudgetViolation {
	pb.mu.RLock()
	defer pb.mu.RUnlock()

	var results []BudgetViolation
	for _, v := range pb.Violations {
		if v.Timestamp.After(since) || v.Timestamp.Equal(since) {
			results = append(results, v)
		}
	}
	return results
}

// FormatDashboard returns a formatted string representing the performance budget dashboard.
func (pb *PerfBudget) FormatDashboard() string {
	pb.mu.RLock()
	defer pb.mu.RUnlock()

	var sb strings.Builder

	sb.WriteString("Performance Budget:\n")
	sb.WriteString("═══════════════════════════════════\n")
	sb.WriteString(fmt.Sprintf("%-22s%-10s%-10s%s\n", "SLO", "Target", "Current", "Status"))
	sb.WriteString("─────────────────────────────────────────────────\n")

	// Order for consistent output
	order := []string{"response_time", "accuracy", "cost_per_task", "token_efficiency", "tool_error_rate"}

	metCount := 0
	atRiskCount := 0
	violatedCount := 0

	for _, key := range order {
		slo, ok := pb.SLOs[key]
		if !ok {
			continue
		}

		targetStr := pb.formatTarget(slo)
		currentStr := pb.formatCurrent(slo)
		statusStr := pb.formatStatus(slo)

		sb.WriteString(fmt.Sprintf("%-22s%-10s%-10s%s\n", slo.Name, targetStr, currentStr, statusStr))

		switch slo.Status {
		case "met":
			metCount++
		case "at_risk":
			atRiskCount++
		case "violated":
			violatedCount++
		}
	}

	sb.WriteString("═══════════════════════════════════\n")

	total := metCount + atRiskCount + violatedCount
	var parts []string
	parts = append(parts, fmt.Sprintf("%d/%d SLOs met", metCount, total))
	if atRiskCount > 0 {
		parts = append(parts, fmt.Sprintf("%d at risk", atRiskCount))
	}
	if violatedCount > 0 {
		parts = append(parts, fmt.Sprintf("%d violated", violatedCount))
	}
	sb.WriteString(fmt.Sprintf("Overall: %s\n", strings.Join(parts, ", ")))

	return sb.String()
}

// formatTarget formats the target value for display.
func (pb *PerfBudget) formatTarget(slo *SLO) string {
	switch slo.Metric {
	case "response_time":
		return fmt.Sprintf("< %.1fs", slo.Target)
	case "accuracy":
		return fmt.Sprintf("> %.0f%%", slo.Target)
	case "cost_per_task":
		return fmt.Sprintf("< $%.2f", slo.Target)
	case "token_efficiency":
		return fmt.Sprintf("< %.0f", slo.Target)
	case "tool_error_rate":
		return fmt.Sprintf("< %.0f%%", slo.Target)
	default:
		return fmt.Sprintf("%.2f", slo.Target)
	}
}

// formatCurrent formats the current value for display.
func (pb *PerfBudget) formatCurrent(slo *SLO) string {
	switch slo.Metric {
	case "response_time":
		return fmt.Sprintf("%.1fs", slo.Current)
	case "accuracy":
		return fmt.Sprintf("%.0f%%", slo.Current)
	case "cost_per_task":
		return fmt.Sprintf("$%.2f", slo.Current)
	case "token_efficiency":
		return formatNumber(slo.Current)
	case "tool_error_rate":
		return fmt.Sprintf("%.1f%%", slo.Current)
	default:
		return fmt.Sprintf("%.2f", slo.Current)
	}
}

// formatStatus formats the status with indicator.
func (pb *PerfBudget) formatStatus(slo *SLO) string {
	switch slo.Status {
	case "met":
		return "✓ MET"
	case "at_risk":
		return "⚠ AT RISK"
	case "violated":
		return "✗ VIOLATED"
	default:
		return slo.Status
	}
}

// AddSLO adds or updates an SLO.
func (pb *PerfBudget) AddSLO(slo SLO) {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	if slo.Measurements == nil {
		slo.Measurements = []float64{}
	}
	if slo.Status == "" {
		slo.Status = "met"
	}
	pb.SLOs[slo.Metric] = &slo
}

// ProjectTrend analyzes measurements for a metric and returns the trend direction.
func (pb *PerfBudget) ProjectTrend(metric string) string {
	pb.mu.RLock()
	defer pb.mu.RUnlock()

	slo, ok := pb.SLOs[metric]
	if !ok || len(slo.Measurements) < 3 {
		return "stable"
	}

	measurements := slo.Measurements
	n := len(measurements)

	// Split into first half and second half
	mid := n / 2
	firstHalf := measurements[:mid]
	secondHalf := measurements[mid:]

	firstAvg := average(firstHalf)
	secondAvg := average(secondHalf)

	// Calculate percentage change
	if firstAvg == 0 {
		return "stable"
	}
	change := (secondAvg - firstAvg) / firstAvg

	// For accuracy metric, higher is better
	if metric == "accuracy" {
		if change > 0.05 {
			return "improving"
		} else if change < -0.05 {
			return "degrading"
		}
		return "stable"
	}

	// For other metrics, lower is better
	if change < -0.05 {
		return "improving"
	} else if change > 0.05 {
		return "degrading"
	}
	return "stable"
}

// Reset clears all measurements and violations.
func (pb *PerfBudget) Reset() {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	for _, slo := range pb.SLOs {
		slo.Measurements = []float64{}
		slo.Current = 0
		slo.Status = "met"
	}
	pb.Violations = []BudgetViolation{}
}

// percentile calculates the p-th percentile from a slice of float64.
func percentile(data []float64, p float64) float64 {
	if len(data) == 0 {
		return 0
	}

	sorted := make([]float64, len(data))
	copy(sorted, data)
	sort.Float64s(sorted)

	rank := (p / 100.0) * float64(len(sorted)-1)
	lower := int(math.Floor(rank))
	upper := int(math.Ceil(rank))

	if lower == upper {
		return sorted[lower]
	}

	frac := rank - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}

// average computes the arithmetic mean of a slice of float64.
func average(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range data {
		sum += v
	}
	return sum / float64(len(data))
}

// formatNumber formats a number with comma separators.
func formatNumber(n float64) string {
	intPart := int(math.Round(n))
	str := fmt.Sprintf("%d", intPart)

	if len(str) <= 3 {
		return str
	}

	var result strings.Builder
	remainder := len(str) % 3
	if remainder > 0 {
		result.WriteString(str[:remainder])
		if len(str) > remainder {
			result.WriteString(",")
		}
	}
	for i := remainder; i < len(str); i += 3 {
		if i > remainder {
			result.WriteString(",")
		}
		result.WriteString(str[i : i+3])
	}
	return result.String()
}
