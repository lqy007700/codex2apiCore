package proxy

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestCopyClaudeNativeResponseHeadersPreservesUsageMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	header := http.Header{
		"anthropic-ratelimit-unified-5h-utilization":    []string{"0.42"},
		"anthropic-ratelimit-unified-5h-reset":          []string{"4102444800"},
		"anthropic-ratelimit-unified-7d_oi-utilization": []string{"0.0"},
		"anthropic-ratelimit-unified-7d_oi-reset":       []string{"4102444800"},
		"anthropic-ratelimit-unified-status":            []string{"allowed"},
		"anthropic-version":                             []string{"2023-06-01"},
		"Authorization":                                 []string{"Bearer secret"},
		"Set-Cookie":                                    []string{"secret=1"},
		"X-Leak":                                        []string{"nope"},
	}
	copyClaudeNativeResponseHeaders(ctx, header)
	if recorder.Header().Get("anthropic-ratelimit-unified-7d_oi-utilization") != "0.0" || recorder.Header().Get("anthropic-ratelimit-unified-7d_oi-reset") != "4102444800" {
		t.Fatalf("Fable usage headers were not forwarded: %#v", recorder.Header())
	}
	if recorder.Header().Get("anthropic-ratelimit-unified-5h-utilization") != "0.42" || recorder.Header().Get("anthropic-version") != "2023-06-01" {
		t.Fatalf("Claude usage headers were not forwarded: %#v", recorder.Header())
	}
	if recorder.Header().Get("Authorization") != "" || recorder.Header().Get("Set-Cookie") != "" || recorder.Header().Get("X-Leak") != "" {
		t.Fatalf("sensitive/unallowlisted headers leaked: %#v", recorder.Header())
	}
}

func TestProtocolNonStreamFailureRejectsPseudoSuccessPayloads(t *testing.T) {
	tests := []struct {
		name     string
		protocol GrokProtocol
		payload  string
		failed   bool
	}{
		{name: "responses success", protocol: GrokProtocolResponses, payload: `{"id":"resp_ok","status":"completed"}`},
		{name: "responses missing terminal status", protocol: GrokProtocolResponses, payload: `{"id":"resp_unknown"}`, failed: true},
		{name: "responses error object", protocol: GrokProtocolResponses, payload: `{"error":{"code":"rate_limited"}}`, failed: true},
		{name: "responses failed status", protocol: GrokProtocolResponses, payload: `{"id":"resp_bad","status":"failed"}`, failed: true},
		{name: "responses failed event", protocol: GrokProtocolResponses, payload: `{"type":"response.failed","response":{"status":"failed"}}`, failed: true},
		{name: "chat error", protocol: GrokProtocolChatCompletions, payload: `{"type":"error","error":{"message":"busy"}}`, failed: true},
		{name: "chat missing choices", protocol: GrokProtocolChatCompletions, payload: `{"id":"chat_unknown"}`, failed: true},
		{name: "messages missing message type", protocol: GrokProtocolMessages, payload: `{"id":"message_unknown"}`, failed: true},
		{name: "invalid JSON", protocol: GrokProtocolResponses, payload: `not-json`, failed: true},
		{name: "empty", protocol: GrokProtocolResponses, payload: ``, failed: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			outcome, failed := protocolNonStreamFailure(tc.protocol, []byte(tc.payload))
			if failed != tc.failed {
				t.Fatalf("failed = %v, want %v; outcome=%#v", failed, tc.failed, outcome)
			}
			if failed && (outcome.logStatusCode == http.StatusOK || strings.TrimSpace(outcome.failureMessage) == "") {
				t.Fatalf("pseudo-success failure was not classified: %#v", outcome)
			}
		})
	}
}

func TestForwardGrokNativePrivateAttemptDoesNotPublishFailedHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"X-Request-Id": []string{"failed-attempt-request"},
			"Retry-After":  []string{"60"},
		},
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"private\"}\n\n" +
				"event: error\ndata: {\"error\":{\"code\":\"future_failure\"}}\n\n",
		)),
	}
	attempt := newContinuousRetryStreamAttempt(true, recorder, recorder)
	t.Cleanup(func() { _ = attempt.Close() })

	_, outcome, _, _ := forwardGrokNativeResponseTo(
		ctx.Request.Context(), ctx, resp, GrokProtocolResponses, true, time.Now(), nil,
		attempt.writerOr(recorder), attempt.flusherOr(recorder),
	)
	if outcome.logStatusCode == http.StatusOK {
		t.Fatalf("failed attempt outcome = %#v", outcome)
	}
	if got := recorder.Header().Get("X-Request-Id"); got != "" {
		t.Fatalf("failed attempt request id leaked downstream: %q", got)
	}
	if got := recorder.Header().Get("Retry-After"); got != "" {
		t.Fatalf("failed attempt retry header leaked downstream: %q", got)
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("failed private attempt body leaked downstream: %q", recorder.Body.String())
	}
}

// TestForwardGrokNativeFailureBeforeVisibleOutputReturnsProtocolHTTPError 验证首个可见事件前
// 的失败仍返回协议对应的 HTTP 错误，而不会伪造成功流。

// TestSendGrokNativeErrorAfterInitialKeepaliveUsesSSE 验证首个保活提交 SSE 后，
// Grok 各协议的错误仍以协议事件返回。
func TestSendGrokNativeErrorAfterInitialKeepaliveUsesSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		protocol   GrokProtocol
		path       string
		wantMarker string
	}{
		{name: "responses", protocol: GrokProtocolResponses, path: "/v1/responses", wantMarker: `"type":"response.failed"`},
		{name: "chat", protocol: GrokProtocolChatCompletions, path: "/v1/chat/completions", wantMarker: `"error":{"code":"upstream_stream_break"`},
		{name: "messages", protocol: GrokProtocolMessages, path: "/v1/messages", wantMarker: "event: error\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, tc.path, nil)
			stop := installContinuousRetrySSEKeepalive(ctx, true, "text/event-stream")
			defer stop()

			keepalive := continuousRetryKeepaliveForContext(ctx.Request.Context())
			keepalive.Activate()
			requestKeepalive := keepalive.(*requestContinuousRetryKeepalive)
			requestKeepalive.last = time.Time{}
			if err := keepalive.Keepalive(); err != nil {
				t.Fatalf("write keepalive: %v", err)
			}
			(&Handler{}).sendGrokNativeHTTPError(ctx, tc.protocol, streamOutcome{
				logStatusCode:  http.StatusServiceUnavailable,
				failureMessage: "upstream busy",
			})

			body := recorder.Body.String()
			if recorder.Code != http.StatusOK ||
				!strings.Contains(body, downstreamSSEKeepaliveComment) ||
				!strings.Contains(body, tc.wantMarker) ||
				!strings.Contains(body, "upstream busy") {
				t.Fatalf("committed SSE error = status %d body %q", recorder.Code, body)
			}
		})
	}
}

// 真实 Anthropic 流形状：input_tokens 在 message_start 的 message.usage 下，
// output_tokens 在 message_delta 的顶层 usage 下，message_stop 不带 usage。

func TestHoldGrokNativePreOutputResponsesOnlyBuffersLifecycle(t *testing.T) {
	created := parseRawGrokSSEFrame([]byte("data: {\"type\":\"response.created\"}\n\n"))
	itemAdded := parseRawGrokSSEFrame([]byte("data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"reasoning\"}}\n\n"))
	text := parseRawGrokSSEFrame([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"x\"}\n\n"))
	keepAlive := parseRawGrokSSEFrame([]byte(": keep-alive\r\n\r\n"))
	roleOnly := parseRawGrokSSEFrame([]byte("data: {\"id\":\"c1\",\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n"))

	if !holdGrokNativePreOutput(GrokProtocolResponses, created, false, false, false) {
		t.Fatal("Responses created should stay buffered for silent retry")
	}
	if holdGrokNativePreOutput(GrokProtocolResponses, itemAdded, false, false, false) {
		t.Fatal("Responses structure frames must flush before first text")
	}
	if holdGrokNativePreOutput(GrokProtocolResponses, keepAlive, false, false, false) {
		t.Fatal("Responses keep-alives must flush so the client sees progress")
	}
	if holdGrokNativePreOutput(GrokProtocolResponses, text, false, false, true) {
		t.Fatal("visible text must never be held")
	}
	if !holdGrokNativePreOutput(GrokProtocolChatCompletions, roleOnly, false, false, false) {
		t.Fatal("Chat role-only frames must stay buffered until first visible delta")
	}
}

// Responses 流式 usage 位于 response.completed 的 response.usage 下,必须被提取。

func TestReadRawGrokSSEFramesPreservesBoundariesAndCommentOnlyFrame(t *testing.T) {
	raw := []byte(":one\r\n\r\nevent: x\r\ndata: first\r\ndata: second\r\n\r\ndata: [DONE]\r\n\r\n")
	reader := &oneByteReader{data: raw}
	var frames []rawGrokSSEFrame
	if err := readRawGrokSSEFrames(reader, func(frame rawGrokSSEFrame) bool {
		copy := rawGrokSSEFrame{Raw: bytes.Clone(frame.Raw), Data: bytes.Clone(frame.Data), Event: frame.Event, HasData: frame.HasData, Done: frame.Done}
		frames = append(frames, copy)
		return true
	}); err != nil {
		t.Fatal(err)
	}
	if len(frames) != 3 || frames[0].HasData || frames[1].Event != "x" || string(frames[1].Data) != "first\nsecond" || !frames[2].Done {
		t.Fatalf("frames = %#v", frames)
	}
	var joined []byte
	for _, frame := range frames {
		joined = append(joined, frame.Raw...)
	}
	if !bytes.Equal(joined, raw) {
		t.Fatalf("joined frames = %q, want %q", joined, raw)
	}
}

type oneByteReader struct{ data []byte }

func (r *oneByteReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	p[0] = r.data[0]
	r.data = r.data[1:]
	return 1, nil
}
