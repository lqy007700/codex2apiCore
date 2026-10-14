package proxy

import (
	"context"

	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/codex2api/auth"
)

func TestUnwrapAntigravityNativeGeminiChunk(t *testing.T) {
	got := unwrapAntigravityNativeGeminiChunk([]byte(`{"response":{"candidates":[{"content":{"parts":[{"text":"hi"}]},"finishReason":"STOP"}],"usageMetadata":{"totalTokenCount":3}}}`))
	if !strings.Contains(string(got), `"text":"hi"`) {
		t.Fatalf("unexpected chunk: %s", got)
	}
}

func TestExecuteAntigravityResponsesRequestKeepsPublicModelIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"response":{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}]}}`)
	}))
	defer server.Close()
	previous := antigravityOAuthEndpointBases
	antigravityOAuthEndpointBases = []string{server.URL}
	t.Cleanup(func() { antigravityOAuthEndpointBases = previous })

	account := &auth.Account{DBID: 9003, UpstreamType: auth.UpstreamAntigravity, AccessToken: "test-token", AntigravityProjectID: "test-project", Models: []string{"gemini-3.8-flash-tiered"}}
	resp, err := ExecuteAntigravityResponsesRequest(context.Background(), account, "gemini-3.8-flash-high", []byte(`{"input":"hello"}`), false, "")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), "gemini-3.8-flash-high") {
		t.Fatalf("response model identity lost: %s", body)
	}
}
