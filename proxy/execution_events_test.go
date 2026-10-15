package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/codex2api/auth"
	"github.com/codex2api/database"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func TestExecutionEventsRoundTripAndBounds(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	attachUpstreamTrace(c, auth.NewStore(nil, nil, nil))
	duration := 13 * time.Millisecond
	RecordExecutionEvent(c.Request.Context(), "queue_finished", 0, &duration)
	for i := 0; i < 200; i++ {
		RecordExecutionEvent(c.Request.Context(), "first_content", 3, nil)
	}
	var trace executionTrace
	if err := json.Unmarshal([]byte(snapshotExecutionJSON(c.Request.Context())), &trace); err != nil {
		t.Fatal(err)
	}
	if len(trace.Events) != 128 || !trace.Truncated || trace.Events[0].Kind != "request_received" || *trace.Events[1].DurationMs != 13 {
		t.Fatalf("unbounded or lost events: %+v", trace)
	}
	resetUpstreamRequestTrace(c)
	if err := json.Unmarshal([]byte(snapshotExecutionJSON(c.Request.Context())), &trace); err != nil {
		t.Fatal(err)
	}
	if len(trace.Events) != 1 || trace.Truncated {
		t.Fatal("WS turn reset retained earlier events")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); io.WriteString(w, "ok") }))
	defer server.Close()
	req, _ := http.NewRequestWithContext(c.Request.Context(), "GET", server.URL, nil)
	resp, err := doTracedUpstreamRequest(server.Client(), req, &auth.Account{DBID: 3}, "")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	recordFirstExecutionContent(c.Request.Context(), 3, gjson.Parse(`{"type":"codex.rate_limits"}`))
	recordFirstExecutionContent(c.Request.Context(), 3, gjson.Parse(`{"type":"response.output_text.delta","delta":"hello"}`))
	recordFirstExecutionContent(c.Request.Context(), 3, gjson.Parse(`{"type":"response.output_text.delta","delta":"again"}`))
	input := &database.UsageLogInput{AccountID: 3, Endpoint: "/v1/responses", Model: "test", StatusCode: 200, AttemptIndex: 1}
	populateExecutionEvents(c, input)
	populateUpstreamTrace(c, input)
	if !strings.Contains(input.ExecutionTrace, "request_sent") || !strings.Contains(input.ExecutionTrace, "http_connection_finished") || !strings.Contains(input.ExecutionTrace, "attempt_finished") {
		t.Fatalf("missing measured phases: %s", input.ExecutionTrace)
	}
	var measured executionTrace
	if err := json.Unmarshal([]byte(input.ExecutionTrace), &measured); err != nil {
		t.Fatal(err)
	}
	contents := 0
	for _, event := range measured.Events {
		if event.Kind == "first_content" {
			contents++
		}
	}
	if contents != 1 {
		t.Fatalf("progress or repeated content miscounted: %d", contents)
	}
	db, err := database.New("sqlite", filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.InsertUsageLog(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	db.FlushUsageLogs()
	logs, err := db.ListRecentUsageLogs(context.Background(), 1)
	if err != nil || len(logs) != 1 {
		t.Fatalf("logs: %+v %v", logs, err)
	}
	chain, err := db.GetUsageRequestChain(context.Background(), logs[0].ID)
	if err != nil || len(chain.Records) != 1 || chain.Records[0].ExecutionTrace != input.ExecutionTrace {
		t.Fatalf("snapshot not persisted: %+v %v", chain, err)
	}
	RecordExecutionEvent(context.Background(), "ignored", 0, nil)
}
