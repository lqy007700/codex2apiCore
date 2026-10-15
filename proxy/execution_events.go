package proxy

import (
	"context"
	"encoding/json"
	"time"

	"github.com/codex2api/database"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// Events contain only allow-listed timing/identity fields, never headers,
// prompts, credentials, proxy URLs or raw upstream errors. They ride the
// existing bounded usage-log buffer and share its retention policy.
type executionEvent struct {
	Sequence   int    `json:"sequence"`
	Kind       string `json:"kind"`
	AtMs       int64  `json:"at_ms"`
	DurationMs *int64 `json:"duration_ms,omitempty"`
	AccountID  int64  `json:"account_id,omitempty"`
}
type executionTrace struct {
	StartedAt time.Time        `json:"started_at"`
	ElapsedMs int64            `json:"elapsed_ms"`
	Events    []executionEvent `json:"events"`
	Truncated bool             `json:"truncated"`
}

func RecordExecutionEvent(ctx context.Context, kind string, accountID int64, duration *time.Duration) {
	audit := upstreamTraceFromContext(ctx)
	if audit == nil {
		return
	}
	audit.mu.Lock()
	defer audit.mu.Unlock()
	if len(audit.events) >= 128 {
		audit.eventsTruncated = true
		return
	}
	event := executionEvent{Sequence: len(audit.events) + 1, Kind: kind, AtMs: time.Since(audit.startedAt).Milliseconds(), AccountID: accountID}
	if duration != nil {
		ms := max(duration.Milliseconds(), 0)
		event.DurationMs = &ms
	}
	audit.events = append(audit.events, event)
}
func executionSpan(ctx context.Context, kind string) func() {
	start := time.Now()
	RecordExecutionEvent(ctx, kind+"_started", 0, nil)
	return func() { duration := time.Since(start); RecordExecutionEvent(ctx, kind+"_finished", 0, &duration) }
}
func snapshotExecutionJSON(ctx context.Context) string {
	audit := upstreamTraceFromContext(ctx)
	if audit == nil {
		return ""
	}
	audit.mu.Lock()
	defer audit.mu.Unlock()
	raw, _ := json.Marshal(executionTrace{StartedAt: audit.startedAt, ElapsedMs: time.Since(audit.startedAt).Milliseconds(), Events: audit.events, Truncated: audit.eventsTruncated})
	return string(raw)
}
func populateExecutionEvents(c *gin.Context, input *database.UsageLogInput) {
	if c == nil || c.Request == nil || input == nil {
		return
	}
	// Hidden continuation rounds carry an earlier trace snapshot. Do not attach
	// another round's current execution events to those historical usage rows.
	if input.RequestID != "" {
		return
	}
	ctx := c.Request.Context()
	kind := "attempt_finished"
	if input.StatusCode >= 400 {
		kind = "attempt_failed"
	}
	RecordExecutionEvent(ctx, kind, input.AccountID, nil)
	if input.IsRetryAttempt {
		RecordExecutionEvent(ctx, "retry_planned", input.AccountID, nil)
	}
	input.ExecutionTrace = snapshotExecutionJSON(ctx)
}

// The normal TTFT metric can use loose progress events. Timeline content timing
// must use the strict content predicate, independently of that metric's flag.
func recordFirstExecutionContent(ctx context.Context, accountID int64, parsed gjson.Result) {
	if !isFirstTokenResult(parsed) {
		return
	}
	audit := upstreamTraceFromContext(ctx)
	if audit == nil {
		return
	}
	audit.mu.Lock()
	if audit.current == nil || audit.current.accountID != accountID || audit.current.firstContent {
		audit.mu.Unlock()
		return
	}
	audit.current.firstContent = true
	audit.mu.Unlock()
	RecordExecutionEvent(ctx, "first_content", accountID, nil)
}
