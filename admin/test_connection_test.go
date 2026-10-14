package admin

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/codex2api/auth"
	"github.com/codex2api/database"
	"github.com/codex2api/proxy"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func antigravityTestSSEBody(text string) string {
	return "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"" + text + "\"}\n\n" +
		"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[]}}\n\n"
}

func newAntigravityConnectionTestAccount() *auth.Account {
	return &auth.Account{
		DBID:                 7,
		UpstreamType:         auth.UpstreamAntigravity,
		AccessToken:          "google-token",
		AntigravityProjectID: "project-1",
		Models:               []string{"gemini-3.6-flash-low", "gemini-3.6-flash-medium", "gemini-3.6-flash-high"},
		Status:               auth.StatusReady,
		HealthTier:           auth.HealthTierHealthy,
	}
}

func TestConnectionAntigravityUsesNativeExecutorAndStreamsContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := auth.NewStore(nil, nil, nil)
	account := newAntigravityConnectionTestAccount()
	store.AddAccount(account)
	handler := &Handler{store: store}
	var gotModel, gotPrompt string
	var gotStream bool
	handler.antigravityCapabilityProbe = func(_ context.Context, acc *auth.Account, model string, body []byte, stream bool, _ string) (*http.Response, error) {
		if acc != account {
			t.Fatalf("executor received account %+v, want runtime account", acc)
		}
		gotModel, gotStream = model, stream
		gotPrompt = gjson.GetBytes(body, "input.0.content.0.text").String()
		if gjson.GetBytes(body, "model").String() != model || !gjson.GetBytes(body, "input").Exists() {
			t.Fatalf("unexpected Responses payload: %s", body)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(antigravityTestSSEBody("pong"))),
		}, nil
	}
	router := gin.New()
	router.GET("/api/admin/accounts/:id/test", handler.TestConnection)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/accounts/7/test?prompt=manual+prompt", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !gotStream {
		t.Fatal("Antigravity connection test must request a streamed response")
	}
	if gotModel != "gemini-3.6-flash-low" {
		t.Fatalf("test model = %q, want published cheapest flash tier", gotModel)
	}
	if gotPrompt != "manual prompt" {
		t.Fatalf("test prompt = %q, want manual prompt", gotPrompt)
	}
	body := recorder.Body.String()
	for _, needle := range []string{`"type":"test_start"`, `"text":"pong"`, `"type":"test_complete"`, `"success":true`} {
		if !strings.Contains(body, needle) {
			t.Fatalf("SSE response %q missing %s", body, needle)
		}
	}
	if strings.Contains(body, "尚未接入") {
		t.Fatalf("Antigravity test still rejected: %s", body)
	}
}

func TestConnectionAntigravityRejectsUnknownModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := auth.NewStore(nil, nil, nil)
	account := newAntigravityConnectionTestAccount()
	store.AddAccount(account)
	handler := &Handler{store: store}
	handler.antigravityCapabilityProbe = func(context.Context, *auth.Account, string, []byte, bool, string) (*http.Response, error) {
		t.Fatal("executor must not run for an unsupported model")
		return nil, nil
	}
	router := gin.New()
	router.GET("/api/admin/accounts/:id/test", handler.TestConnection)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/accounts/7/test?model=gpt-5.5", nil))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "gpt-5.5") {
		t.Fatalf("status=%d body=%s, want 400 naming the rejected model", recorder.Code, recorder.Body.String())
	}
}

func TestRunSingleBatchTestAntigravityUsesNativeExecutor(t *testing.T) {
	store := auth.NewStore(nil, nil, nil)
	account := newAntigravityConnectionTestAccount()
	store.AddAccount(account)
	handler := &Handler{store: store}
	calls := 0
	handler.antigravityCapabilityProbe = func(_ context.Context, _ *auth.Account, model string, _ []byte, stream bool, _ string) (*http.Response, error) {
		calls++
		if !stream || model != "gemini-3.6-flash-low" {
			t.Fatalf("executor args model=%q stream=%v", model, stream)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(antigravityTestSSEBody("ok"))),
		}, nil
	}
	status, message := handler.runSingleBatchTest(context.Background(), account)
	if status != "success" || calls != 1 {
		t.Fatalf("runSingleBatchTest() = (%q, %q) calls=%d, want success via Antigravity executor", status, message, calls)
	}
}

func TestRunSingleBatchTestAntigravityCapacity503IsRateLimited(t *testing.T) {
	store := auth.NewStore(nil, nil, nil)
	account := newAntigravityConnectionTestAccount()
	store.AddAccount(account)
	handler := &Handler{store: store}
	handler.antigravityCapabilityProbe = func(context.Context, *auth.Account, string, []byte, bool, string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"code":503,"status":"UNAVAILABLE","message":"No capacity available for model gemini-3.6-flash-low on the server","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"MODEL_CAPACITY_EXHAUSTED"}]}}`)),
		}, nil
	}
	status, _ := handler.runSingleBatchTest(context.Background(), account)
	if status != "rate_limited" {
		t.Fatalf("status = %q, want rate_limited for shared capacity exhaustion", status)
	}
	if account.RuntimeStatus() == "error" {
		t.Fatal("shared capacity exhaustion must not mark the account as error")
	}
}

func TestBuildConnectionTestPayloadUsesStoreContent(t *testing.T) {
	store := auth.NewStore(nil, nil, nil)
	store.SetTestContent("say pong")

	payload := buildConnectionTestPayload(store, "gpt-5.5")
	if got := gjson.GetBytes(payload, "input.0.content.0.text").String(); got != "say pong" {
		t.Fatalf("test content = %q, want say pong", got)
	}
}

// TestBuildConnectionTestPayloadRandomizesMultiLineContent 验证多行测活内容
// 按行随机抽取并展开变量（issue #320）。
func TestBuildConnectionTestPayloadRandomizesMultiLineContent(t *testing.T) {
	store := auth.NewStore(nil, nil, nil)
	store.SetTestContent("ping-a\nping-b\ncount {{rand:1-1}}")

	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		payload := buildConnectionTestPayload(store, "gpt-5.5")
		seen[gjson.GetBytes(payload, "input.0.content.0.text").String()] = true
	}
	for _, want := range []string{"ping-a", "ping-b", "count 1"} {
		if !seen[want] {
			t.Fatalf("candidate %q never sent in 200 draws; seen=%v", want, seen)
		}
	}
	if len(seen) != 3 {
		t.Fatalf("unexpected payload variants: %v", seen)
	}
}

func TestFormatUsageLimitedTestErrorReportsSuccessfulProbeAsLimited(t *testing.T) {
	msg, limited := formatUsageLimitedTestError(proxy.CodexUsageSyncResult{
		Premium5hRateLimited: true,
		UsagePct5h:           100,
		Reset5hAt:            time.Now().Add(time.Hour),
	})

	if !limited {
		t.Fatal("limited = false, want true")
	}
	for _, want := range []string{"返回 200", "5h 用量头", "限流状态"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message %q does not contain %q", msg, want)
		}
	}
}

func TestFormatUsageLimitedTestErrorAcceptsSuccessfulProbeWhenIgnored(t *testing.T) {
	msg, limited := formatUsageLimitedTestError(proxy.CodexUsageSyncResult{
		Premium5hRateLimited:     false,
		UsagePct5h:               100,
		HasUsage5h:               true,
		UsageWindowLimitsIgnored: true,
	})

	if limited || msg != "" {
		t.Fatalf("formatUsageLimitedTestError() = (%q, %v), want empty successful result", msg, limited)
	}
}

func TestClassifyResponsesTerminalEvent(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    responsesTerminalOutcome
	}{
		{
			name:    "completed",
			payload: `{"type":"response.completed","response":{"status":"completed"}}`,
			want:    responsesTerminalSuccess,
		},
		{
			name:    "usage limited failure",
			payload: `{"type":"response.failed","response":{"status_details":{"error":{"type":"usage_limit_reached"}}}}`,
			want:    responsesTerminalUsageLimited,
		},
		{
			name:    "generic failure",
			payload: `{"type":"response.failed","response":{"error":{"type":"server_error"}}}`,
			want:    responsesTerminalFailed,
		},
		{
			name:    "non terminal",
			payload: `{"type":"response.output_text.delta","delta":"pong"}`,
			want:    responsesTerminalUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyResponsesTerminalEvent([]byte(tt.payload)); got != tt.want {
				t.Fatalf("classifyResponsesTerminalEvent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyResponsesUsageLimitFailureMarksAuthoritativeCooldown(t *testing.T) {
	store := auth.NewStore(nil, nil, &database.SystemSettings{
		MaxConcurrency:         2,
		TestConcurrency:        1,
		TestModel:              "gpt-5.4",
		IgnoreUsageLimitStatus: true,
	})
	account := &auth.Account{DBID: 44, AccessToken: "token", PlanType: "plus", Status: auth.StatusReady}
	store.AddAccount(account)
	handler := &Handler{store: store}
	payload := []byte(`{"type":"response.failed","response":{"status_details":{"error":{"type":"usage_limit_reached","resets_in_seconds":1800}}}}`)

	if !handler.applyResponsesUsageLimitFailure(account, &http.Response{Header: make(http.Header)}, "gpt-5.4", payload) {
		t.Fatal("usage_limit_reached terminal event was not handled")
	}
	if !account.HasActiveCooldown() || account.IsAvailable() {
		t.Fatal("usage_limit_reached terminal event did not block the account")
	}
	if reason := account.GetCooldownReason(); reason != auth.ResponsesRateLimitedCooldownReason {
		t.Fatalf("CooldownReason = %q, want %q", reason, auth.ResponsesRateLimitedCooldownReason)
	}
}

// TestConnectionDeletedAgentRuntimeMarksBanned 验证连接测试会将 runtime 已删除的账号标记为封禁。

func TestExtractCompletedOutputText(t *testing.T) {
	event := []byte(`{
		"type":"response.completed",
		"response":{
			"status":"completed",
			"output":[
				{"type":"message","content":[{"type":"output_text","text":"hello from completed"}]}
			]
		}
	}`)

	if got := extractCompletedOutputText(event); got != "hello from completed" {
		t.Fatalf("output text = %q, want completed text", got)
	}
}

func TestFormatUpstreamTestErrorIncludesMessageAndEvent(t *testing.T) {
	event := []byte(`{
		"type":"response.failed",
		"response":{
			"error":{"message":"model unavailable","code":"model_not_available"}
		}
	}`)

	got := formatUpstreamTestError(event, "fallback")
	for _, want := range []string{"model unavailable", "model_not_available", "上游事件", "response.failed"} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatted error %q does not contain %q", got, want)
		}
	}
}

func TestFormatNoOutputUpstreamErrorIncludesCompletedEvent(t *testing.T) {
	event := []byte(`{"type":"response.completed","response":{"status":"completed","output":[]}}`)

	got := formatNoOutputUpstreamError(event)
	for _, want := range []string{"没有返回文本输出", "上游事件", `"output": []`} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatted no-output error %q does not contain %q", got, want)
		}
	}
}

func TestPreferredAntigravityFlashLowModelPicksNewestVersion(t *testing.T) {
	models := []string{
		"gemini-3.5-flash-low", "gemini-3.5-flash-high", "gemini-3.6-flash-low",
		"gemini-3.8-flash-low", "gemini-3.7-flash-low", "gemini-3.1-pro-low", "claude-sonnet-4-6",
	}
	if got := preferredAntigravityFlashLowModel(models, nil); got != "gemini-3.8-flash-low" {
		t.Fatalf("preferred = %q, want newest flash low tier", got)
	}
	limited := func(model string) bool { return model == "gemini-3.8-flash-low" }
	if got := preferredAntigravityFlashLowModel(models, limited); got != "gemini-3.7-flash-low" {
		t.Fatalf("preferred with 3.8 cooled = %q, want next newest", got)
	}
	if got := preferredAntigravityFlashLowModel([]string{"claude-sonnet-4-6"}, nil); got != "" {
		t.Fatalf("preferred without flash tiers = %q, want empty", got)
	}
}
