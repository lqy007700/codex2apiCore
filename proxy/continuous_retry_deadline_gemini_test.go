package proxy

import (
	"encoding/json"

	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/codex2api/database"
	"github.com/gin-gonic/gin"
)

func TestContinuousRetryGeminiTimeoutUsesNativeError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name        string
		committed   bool
		lastFailure bool
	}{
		{name: "timeout JSON"},
		{name: "timeout SSE", committed: true},
		{name: "last failure JSON", lastFailure: true},
		{name: "last failure SSE", committed: true, lastFailure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini:streamGenerateContent", nil)
			stop := installContinuousRetryHTTPDeadline(c, database.ContinuousRetryPolicy{Enabled: true, MaxDurationSeconds: 1}, continuousRetryProtocolGemini)
			wantCode, wantStatus, wantMessage := http.StatusGatewayTimeout, "DEADLINE_EXCEEDED", continuousRetryTimeoutMessage
			if tc.lastFailure {
				wantCode, wantStatus, wantMessage = http.StatusTooManyRequests, "RESOURCE_EXHAUSTED", "rate_limit_error · quota exhausted"
				rememberContinuousRetryFailure(c.Request.Context(), continuousRetryFailure{
					status:      wantCode,
					body:        []byte(`{"error":{"message":"quota exhausted","type":"rate_limit_error"}}`),
					contentType: "application/json",
				})
			}
			if tc.committed {
				c.Header("Content-Type", "text/event-stream")
				_, _ = c.Writer.WriteString("data: {}\n\n")
			}
			continuousRetryDeadlineForContext(c.Request.Context()).cancel(errContinuousRetryDeadlineExceeded)
			if !writeContinuousRetryTimeoutResponse(c, continuousRetryProtocolGemini) {
				t.Fatal("timeout was not handled")
			}
			stop()
			wantHTTP := wantCode
			body := recorder.Body.String()
			if tc.committed {
				wantHTTP = http.StatusOK
				if !recorder.Flushed || strings.Count(body, "data: ") != 1 || strings.Count(body, `"error":`) != 1 {
					t.Fatalf("expected one flushed bare JSON terminal error, got %q (flushed=%v)", body, recorder.Flushed)
				}
				body = strings.TrimPrefix(body, "data: {}\n\n")
				if body != strings.TrimSpace(body) {
					t.Fatalf("terminal error must end at JSON without a trailing SSE delimiter: %q", body)
				}
			}
			if recorder.Code != wantHTTP {
				t.Fatalf("HTTP status = %d, want %d", recorder.Code, wantHTTP)
			}
			var payload struct {
				Error struct {
					Code    int    `json:"code"`
					Status  string `json:"status"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &payload); err != nil {
				t.Fatalf("invalid Gemini error %q: %v", body, err)
			}
			if payload.Error.Code != wantCode || payload.Error.Status != wantStatus || payload.Error.Message != wantMessage {
				t.Fatalf("unexpected Gemini error: %+v", payload.Error)
			}
			if strings.Contains(body, "response.failed") {
				t.Fatalf("Gemini timeout contains a Responses event: %q", body)
			}
		})
	}
}

// Install a short real retry timer on the request rather than replacing it
// with the handler's policy timer, whose minimum duration is one second.

func TestWriteGeminiNativeErrorDoesNotAppendSSEToCommittedJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.JSON(http.StatusOK, gin.H{"candidates": []any{}})
	before := recorder.Body.String()
	writeGeminiNativeError(c, http.StatusGatewayTimeout, continuousRetryTimeoutMessage)
	if got := recorder.Body.String(); got != before || !json.Valid([]byte(got)) {
		t.Fatalf("error writer corrupted committed JSON: %s", got)
	}
}
