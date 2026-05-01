package cmd

import (
	"fmt"
	"os"
	"time"
)

// QueryProfile tracks timing for a single query.
type QueryProfile struct {
	StartTime time.Time
	TTFT      time.Duration // time to first token
	APICall   time.Duration
	ToolExec  time.Duration
	TotalTime time.Duration
	TokensIn  int
	TokensOut int

	firstTokenRecorded bool
	apiStart           time.Time
	toolStart          time.Time
}

// startProfile begins profiling a query.
func startProfile() *QueryProfile {
	return &QueryProfile{
		StartTime: time.Now(),
	}
}

// RecordTTFT records the time to first token.
func (p *QueryProfile) RecordTTFT() {
	if !p.firstTokenRecorded {
		p.TTFT = time.Since(p.StartTime)
		p.firstTokenRecorded = true
	}
}

// RecordAPICallStart marks the beginning of an API call.
func (p *QueryProfile) RecordAPICallStart() {
	p.apiStart = time.Now()
}

// RecordAPICallEnd marks the end of an API call and accumulates duration.
func (p *QueryProfile) RecordAPICallEnd() {
	if !p.apiStart.IsZero() {
		p.APICall += time.Since(p.apiStart)
		p.apiStart = time.Time{}
	}
}

// RecordToolExecStart marks the beginning of a tool execution.
func (p *QueryProfile) RecordToolExecStart() {
	p.toolStart = time.Now()
}

// RecordToolExecEnd marks the end of a tool execution and accumulates duration.
func (p *QueryProfile) RecordToolExecEnd() {
	if !p.toolStart.IsZero() {
		p.ToolExec += time.Since(p.toolStart)
		p.toolStart = time.Time{}
	}
}

// Finish completes the profile and records the total time.
func (p *QueryProfile) Finish() {
	p.TotalTime = time.Since(p.StartTime)
}

// String returns a formatted summary of the profile, suitable for debug output.
func (p *QueryProfile) String() string {
	return fmt.Sprintf(
		"Profile: total=%s ttft=%s api=%s tools=%s tokens_in=%d tokens_out=%d",
		p.TotalTime.Round(time.Millisecond),
		p.TTFT.Round(time.Millisecond),
		p.APICall.Round(time.Millisecond),
		p.ToolExec.Round(time.Millisecond),
		p.TokensIn,
		p.TokensOut,
	)
}

// isDebug returns true when HAWK_DEBUG is set to a truthy value.
func isDebug() bool {
	v := os.Getenv("HAWK_DEBUG")
	return v == "1" || v == "true" || v == "yes"
}
