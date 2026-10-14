package proxy

import (
	"context"
	"errors"

	"net/http"
	"net/http/httptest"

	"strings"

	"testing"
	"time"

	"github.com/codex2api/database"
)

const (
	geminiStreamTestThought = `{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"THOUGHT_ONLY","thought":true}]}}]}}`
	geminiStreamTestAnswer  = `{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"ANSWER TEST_END"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":20,"thoughtsTokenCount":5,"totalTokenCount":35}}}`
	geminiStreamTestRequest = `{"contents":[{"role":"user","parts":[{"text":"give ANSWER TEST_END"}]}]}`
)

func geminiStreamTestHTTPRequest(ctx context.Context, method string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-3.7-flash-low:"+method+"?alt=sse", strings.NewReader(geminiStreamTestRequest)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func assertGeminiStreamUsageStatus(t *testing.T, db *database.DB, want int, stream bool) database.UsageLog {
	t.Helper()
	db.FlushUsageLogs()
	logs, err := db.ListUsageLogsByFilter(context.Background(), database.UsageLogFilter{
		Start: time.Now().Add(-time.Minute), End: time.Now().Add(time.Minute), IncludeCanceled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("usage logs = %d, want 1: %+v", len(logs), logs)
	}
	if entry := logs[0]; entry.StatusCode != want || entry.Stream != stream {
		t.Fatalf("usage status=%d stream=%v, want status=%d stream=%v", entry.StatusCode, entry.Stream, want, stream)
	}
	return *logs[0]
}

// The upstream remains silent until the client receives a heartbeat. This
// proves keepalive continues during body reads, after the headers and thought.

// The Google SDK expects a bare JSON error after the SSE data
// events. A data-prefixed error is silently discarded by older SDKs.

type geminiStreamDisconnectWriter struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
	fail   bool
	wrote  bool
}

func (w *geminiStreamDisconnectWriter) Write(p []byte) (int, error) {
	w.wrote = true
	if w.fail {
		return 0, errors.New("test downstream disconnected")
	}
	n, err := w.ResponseRecorder.Write(p)
	w.cancel()
	return n, err
}

func (w *geminiStreamDisconnectWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}
