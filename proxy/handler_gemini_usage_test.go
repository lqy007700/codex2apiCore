package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/codex2api/database"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	geminiUsageTestMetadata = `{"promptTokenCount":100,"candidatesTokenCount":20,"thoughtsTokenCount":5,"cachedContentTokenCount":30,"totalTokenCount":125}`
	geminiUsageTestTerminal = `{"response":{"candidates":[{"content":{"parts":[{"text":"answer"}]},"finishReason":"STOP"}],"usageMetadata":` + geminiUsageTestMetadata + `}}`
	geminiUsageTestStopOnly = `{"response":{"candidates":[{"content":{"parts":[{"text":"answer"}]},"finishReason":"STOP"}]}}`
	geminiUsageTestTail     = `{"response":{"usageMetadata":` + geminiUsageTestMetadata + `}}`
)

// Accounting must not replace the native wire counts with our internal
// output total (which includes thinking), or alter thought/tool content.

func assertGeminiNativeUsage(t *testing.T, entry database.UsageLog, input, output, reasoning, cached, total int) {
	t.Helper()
	if entry.InputTokens != input || entry.OutputTokens != output || entry.ReasoningTokens != reasoning || entry.CachedTokens != cached || entry.TotalTokens != total {
		t.Fatalf("usage input/output/reasoning/cache/total=%d/%d/%d/%d/%d, want %d/%d/%d/%d/%d", entry.InputTokens, entry.OutputTokens, entry.ReasoningTokens, entry.CachedTokens, entry.TotalTokens, input, output, reasoning, cached, total)
	}
	if entry.PromptTokens != input || entry.CompletionTokens != output {
		t.Fatalf("legacy prompt/completion=%d/%d, want %d/%d", entry.PromptTokens, entry.CompletionTokens, input, output)
	}
}

func TestGeminiNativeUsageMatchesResponsesAccounting(t *testing.T) {
	for _, raw := range []string{
		geminiUsageTestMetadata,
		`{"promptTokenCount":100,"candidatesTokenCount":20,"thoughtsTokenCount":5,"cachedContentTokenCount":30}`,
		`{"promptTokenCount":100,"thoughtsTokenCount":5}`,
	} {
		t.Run(raw, func(t *testing.T) {
			native := geminiNativeUsageFromBody([]byte(`{"usageMetadata":` + raw + `}`))
			metadata := gjson.Parse(raw).Value().(map[string]any)
			response := gjson.Parse(antigravityJSON(antigravityUsage(metadata)))
			converted := extractUsageFromResult(response)
			if native.InputTokens != converted.InputTokens || native.OutputTokens != converted.OutputTokens || native.ReasoningTokens != converted.ReasoningTokens || native.CachedTokens != converted.CachedTokens || native.TotalTokens != int(response.Get("total_tokens").Int()) {
				t.Fatalf("native=%+v Responses=%s", native, response.Raw)
			}
		})
	}
}

func TestGeminiNativeStreamFirstTokenRequiresContent(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload string
		content bool
	}{
		{"keepalive", `{}`, false},
		{"usage", `{"usageMetadata":` + geminiUsageTestMetadata + `}`, false},
		{"empty_part", `{"candidates":[{"content":{"parts":[{"text":""}]}}]}`, false},
		{"signature", `{"candidates":[{"content":{"parts":[{"thoughtSignature":"sig"}]}}]}`, false},
		{"stop", `{"candidates":[{"finishReason":"STOP"}]}`, false},
		{"text", `{"candidates":[{"content":{"parts":[{"text":"answer"}]}}]}`, true},
		{"thought", `{"candidates":[{"content":{"parts":[{"text":"thinking","thought":true}]}}]}`, true},
		{"tool", `{"candidates":[{"content":{"parts":[{"functionCall":{"name":"lookup","args":{}}}]}}]}`, true},
		{"image", `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"test"}}]}}]}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini:streamGenerateContent", nil)
			wire := "data: " + tc.payload + "\n\n"
			result := forwardGeminiNativeStream(c, strings.NewReader(wire), time.Now().Add(-time.Second))
			if result.readErr != nil || result.writeErr != nil {
				t.Fatalf("forward errors: %v / %v", result.readErr, result.writeErr)
			}
			if (result.firstTokenMs > 0) != tc.content || (tc.content && result.firstTokenMs < 1000) {
				t.Fatalf("first_token_ms=%d content=%v", result.firstTokenMs, tc.content)
			}
			if recorder.Body.String() != wire || !gjson.Valid(tc.payload) {
				t.Fatalf("wire changed: %s", recorder.Body.String())
			}
		})
	}
}
