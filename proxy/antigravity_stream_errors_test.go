package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/codex2api/auth"
	"github.com/codex2api/config"
	"github.com/codex2api/database"
	"github.com/gin-gonic/gin"
)

// A real Cloud Code shape: HTTP 200, a partial text event, then a bare
// pretty-printed backend error document with no SSE framing and no finishReason.
const antigravityTrailingCapacityStream = "data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"partial\"}]}}]}}\n\n" +
	"{\n" +
	"  \"error\": {\n" +
	"    \"code\": 503,\n" +
	"    \"message\": \"No capacity available for model gemini-3.8-flash-medium on the server\",\n" +
	"    \"status\": \"UNAVAILABLE\",\n" +
	"    \"details\": [\n" +
	"      {\n" +
	"        \"@type\": \"type.googleapis.com/google.rpc.ErrorInfo\",\n" +
	"        \"reason\": \"MODEL_CAPACITY_EXHAUSTED\",\n" +
	"        \"domain\": \"cloudcode-pa.googleapis.com\",\n" +
	"        \"metadata\": {\n" +
	"          \"model\": \"gemini-3.8-flash-medium\",\n" +
	"          \"error_number\": \"2010\"\n" +
	"        }\n" +
	"      }\n" +
	"    ]\n" +
	"  }\n" +
	"}\n"

const antigravityTrailingQuotaStream = "data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"partial\"}]}}]}}\n\n" +
	`data: {"error":{"code":429,"message":"You have exhausted your capacity on this model.","status":"RESOURCE_EXHAUSTED","details":[` +
	`{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"QUOTA_EXHAUSTED","metadata":{"model":"gemini-3.7-flash-tiered","consumer":"projects/123456789"}},` +
	`{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"4m50s"},` +
	`{"@type":"type.googleapis.com/google.rpc.Help","links":[{"url":"https://example.invalid"}]}]}}` + "\n\n"

type antigravityStreamErrReader struct{ err error }

func (r antigravityStreamErrReader) Read([]byte) (int, error) { return 0, r.err }

func collectAntigravitySSEPayloads(t *testing.T, reader io.Reader) ([]string, error) {
	t.Helper()
	events := newAntigravitySSEReader(bufio.NewReader(reader))
	var payloads []string
	for {
		payload, err := events.next()
		if err != nil {
			return payloads, err
		}
		var compact bytes.Buffer
		if compactErr := json.Compact(&compact, payload); compactErr != nil {
			t.Fatalf("payload %q is not valid JSON: %v", payload, compactErr)
		}
		payloads = append(payloads, compact.String())
	}
}

func TestAntigravitySSEReaderAssemblesPayloads(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{
			name: "pretty bare document after an event",
			raw:  "data: {\"a\":1}\n\n{\n  \"error\": {\n    \"code\": 503\n  }\n}\n",
			want: []string{`{"a":1}`, `{"error":{"code":503}}`},
		},
		{
			name: "bare document without a blank separator",
			raw:  "data: {\"a\":1}\n{\"error\":{\"code\":429}}\n",
			want: []string{`{"a":1}`, `{"error":{"code":429}}`},
		},
		{
			name: "data payload continued by unprefixed lines",
			raw:  "data: {\n  \"error\": {\"code\": 500}\n}\n\n",
			want: []string{`{"error":{"code":500}}`},
		},
		{
			name: "fields and comments ignored, data lines joined",
			raw:  ": keepalive\nevent: message\nid: 7\nretry: 10\ndata: {\"a\":\ndata: 1}\n\n",
			want: []string{`{"a":1}`},
		},
		{
			name: "array-wrapped document at EOF without newline",
			raw:  `[{"error":{"code":429}}]`,
			want: []string{`[{"error":{"code":429}}]`},
		},
		{
			name: "indented bare document ended by the next data line",
			raw:  "  {\"error\":{\"code\":503}}\ndata: {\"a\":1}\n\n",
			want: []string{`{"error":{"code":503}}`, `{"a":1}`},
		},
		{
			name: "indented pretty document ended by a blank line",
			raw:  "  {\n    \"error\": {\"code\": 503}\n  }\n\ndata: {\"a\":1}\n\n",
			want: []string{`{"error":{"code":503}}`, `{"a":1}`},
		},
		{
			name: "stray line after a complete event stays ignored",
			raw:  "data: {\"a\":1}\nnoise\n\n",
			want: []string{`{"a":1}`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := collectAntigravitySSEPayloads(t, strings.NewReader(tc.raw))
			if !errors.Is(err, io.EOF) {
				t.Fatalf("end error = %v, want io.EOF", err)
			}
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Fatalf("payloads = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAntigravitySSEReaderKeepsTransportErrorAfterFinalPayload(t *testing.T) {
	reader := io.MultiReader(strings.NewReader(`data: {"a":1}`), antigravityStreamErrReader{err: context.Canceled})
	got, err := collectAntigravitySSEPayloads(t, reader)
	if len(got) != 1 || got[0] != `{"a":1}` {
		t.Fatalf("payloads = %q, want the final event drained first", got)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("end error = %v, want context.Canceled rather than a clean EOF", err)
	}
}

func TestAntigravitySSEReaderBoundsBareDocuments(t *testing.T) {
	raw := "{\n" + strings.Repeat("\"x\": 1,\n", antigravityErrorBodyLimit/8+16)
	events := newAntigravitySSEReader(bufio.NewReader(strings.NewReader(raw)))
	if _, err := events.next(); !errors.Is(err, errAntigravityStreamEventTooLarge) {
		t.Fatalf("error = %v, want the bare document size limit", err)
	}
	if _, err := events.next(); !errors.Is(err, errAntigravityStreamEventTooLarge) {
		t.Fatalf("second error = %v, want the size limit to stay terminal", err)
	}
}

func newAntigravityStreamErrorTestHandler(t *testing.T) *Handler {
	t.Helper()
	store := auth.NewStore(nil, nil, &database.SystemSettings{MaxConcurrency: 1, TestConcurrency: 1, TestModel: "gpt-5.4"})
	t.Cleanup(store.Stop)
	return NewHandler(store, nil, &config.Config{AllowAnonymousV1: true}, nil)
}

func antigravityFailureStreak(account *auth.Account) int {
	account.Mu().RLock()
	defer account.Mu().RUnlock()
	return account.FailureStreak
}

func TestAntigravityStreamErrorStatusCode(t *testing.T) {
	cases := []struct {
		code   any
		status string
		want   int
	}{
		{float64(503), "", http.StatusServiceUnavailable},
		{"429", "", http.StatusTooManyRequests},
		{json.Number("401"), "", http.StatusUnauthorized},
		{nil, "RESOURCE_EXHAUSTED", http.StatusTooManyRequests},
		{nil, "UNAVAILABLE", http.StatusServiceUnavailable},
		{float64(200), "INVALID_ARGUMENT", http.StatusBadRequest},
		{nil, "DEADLINE_EXCEEDED", http.StatusGatewayTimeout},
		{nil, "CANCELLED", http.StatusBadGateway},
		{nil, "", http.StatusBadGateway},
	}
	for _, tc := range cases {
		if got := antigravityStreamErrorStatusCode(tc.code, tc.status); got != tc.want {
			t.Errorf("status(%v, %q) = %d, want %d", tc.code, tc.status, got, tc.want)
		}
	}
}

func newAntigravityRawStreamTestHandler(t *testing.T, stream string) (*Handler, *auth.Account) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, stream)
	}))
	t.Cleanup(upstream.Close)
	previous := antigravityOAuthEndpointBases
	antigravityOAuthEndpointBases = []string{upstream.URL}
	t.Cleanup(func() { antigravityOAuthEndpointBases = previous })

	store := auth.NewStore(nil, nil, &database.SystemSettings{MaxConcurrency: 1, TestConcurrency: 1, MaxRetries: 1})
	t.Cleanup(store.Stop)
	account := &auth.Account{
		DBID:                 7405,
		UpstreamType:         auth.UpstreamAntigravity,
		AccessToken:          "google-access-token",
		RefreshToken:         "google-refresh-token",
		AntigravityProjectID: "google-project",
		Models:               auth.AntigravityDefaultModelIDs(),
	}
	store.AddAccount(account)
	return NewHandler(store, nil, &config.Config{AllowAnonymousV1: true}, nil), account
}

// End to end through the Responses handler: partial text is already delivered,
// so the failure reaches the client, but with Google's status and message, and
// the shared capacity shortage is not charged to the account.
func TestResponsesHandlerSurfacesAntigravityMidStreamCapacityError(t *testing.T) {
	handler, account := newAntigravityRawStreamTestHandler(t, antigravityTrailingCapacityStream)
	recorder := invokeAntigravityTransport(t, handler, "/v1/responses",
		`{"model":"`+antigravityTransportTestModel+`","stream":true,"input":"hello"}`, (*Handler).Responses)

	body := recorder.Body.String()
	if strings.Contains(body, `"type":"response.completed"`) {
		t.Fatalf("backend error became a completed response: %s", body)
	}
	if !strings.Contains(body, `"delta":"partial"`) || !strings.Contains(body, "No capacity available for model gemini-3.8-flash-medium") || !strings.Contains(body, `"status_code":503`) {
		t.Fatalf("downstream stream lost the partial text or the upstream error: %s", body)
	}
	if got := antigravityFailureStreak(account); got != 0 {
		t.Fatalf("mid-stream capacity shortage changed FailureStreak to %d", got)
	}
}

func TestResponsesHandlerCoolsAntigravityModelOnMidStreamQuotaError(t *testing.T) {
	handler, account := newAntigravityRawStreamTestHandler(t, antigravityTrailingQuotaStream)
	recorder := invokeAntigravityTransport(t, handler, "/v1/responses",
		`{"model":"`+antigravityTransportTestModel+`","stream":true,"input":"hello"}`, (*Handler).Responses)

	body := recorder.Body.String()
	if !strings.Contains(body, `"status_code":429`) || !strings.Contains(body, "You have exhausted your capacity on this model.") {
		t.Fatalf("downstream stream lost the upstream quota error: %s", body)
	}
	if !account.IsModelRateLimited(antigravityTransportTestModel) {
		t.Fatal("a mid-stream QUOTA_EXHAUSTED did not cool the model down, so the next request would pick this account again")
	}
}
