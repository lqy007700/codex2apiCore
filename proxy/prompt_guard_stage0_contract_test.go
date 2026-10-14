package proxy

import (
	"bytes"

	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"

	"strings"
	"testing"

	"github.com/codex2api/auth"

	"github.com/codex2api/database"
	"github.com/codex2api/security/promptfilter"
	"github.com/gin-gonic/gin"
)

func TestStage0DisabledGuardDoesNotRetainIngressBodyOrComputeDigest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := auth.NewStore(nil, nil, &database.SystemSettings{MaxConcurrency: 2})
	t.Cleanup(store.Stop)
	cfg := promptfilter.DefaultConfig()
	cfg.Enabled = false
	cfg.Advanced.Sidecar.Enabled = false
	cfg.Advanced.NewAPI.Enabled = false
	store.SetPromptFilterConfig(cfg)
	handler := NewHandler(store, nil, nil, nil)

	body := []byte(`{"model":"gpt-5.5","input":"ordinary request"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	rawBody, err := readRawRequestBody(c)
	if err != nil {
		t.Fatal(err)
	}
	handler.capturePromptRequestIngress(c, rawBody)

	if _, exists := c.Get(ingressRequestBodyContextKey); exists {
		t.Fatal("disabled prompt security retained a second ingress body reference")
	}
	if got := promptRequestDigestComputationCount(c); got != 0 {
		t.Fatalf("disabled prompt security computed %d body digests, want 0", got)
	}
}

func TestStage0NewAPIPolicyComputesBodyDigestOncePerRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := promptGuardTestConfig()
	cfg.Advanced.NewAPI.Enabled = true
	cfg.Advanced.NewAPI.MaxClockSkewSeconds = 120
	handler := newPromptGuardTestHandler(cfg)
	body := []byte(`{"model":"gpt-5.5","input":"ordinary request"}`)
	c, _ := signedNewAPIPolicyContext(t, "stage0-digest-once", newAPIIdentity{UserID: "42", ClientIP: "203.0.113.8"}, "/v1/responses", body)
	addSignedNewAPIPolicyMeta(t, c, newAPIPolicyMeta{
		Profile: promptfilter.GuardProfileBalanced, Mode: promptfilter.GuardModeEnforce,
		Provider: string(promptfilter.ModelFamilyOpenAI), Protocol: string(promptfilter.ProtocolResponses),
	}, true)

	for attempt := 0; attempt < 5; attempt++ {
		if _, verified := handler.verifyNewAPIPolicyContext(c, cfg.Advanced.NewAPI, body); !verified {
			t.Fatalf("verification attempt %d failed", attempt+1)
		}
	}
	if got := promptRequestDigestComputationCount(c); got != 1 {
		t.Fatalf("body digest computations = %d, want exactly 1 per request", got)
	}
}

func TestStage0DisabledNewAPIPolicyDoesNotComputeDigest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.5","input":"ordinary request"}`)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	handler := &Handler{}

	if _, verified := handler.verifyNewAPIPolicyContext(c, promptfilter.NewAPIConfig{Enabled: false}, body); verified {
		t.Fatal("disabled NewAPI policy unexpectedly verified a request")
	}
	if got := promptRequestDigestComputationCount(c); got != 0 {
		t.Fatalf("disabled NewAPI policy computed %d body digests, want 0", got)
	}
}

func TestStage0AlphaSearchBlocksBeforeAccountSelection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := auth.NewStore(nil, nil, &database.SystemSettings{MaxConcurrency: 2})
	t.Cleanup(store.Stop)
	store.SetPromptFilterConfig(promptGuardTestConfig())
	handler := NewHandler(store, nil, nil, nil)

	request := func(body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/alpha/search", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		handler.CodexAlphaSearchHandler(c)
		return recorder
	}

	harmful := request(`{"id":"search_blocked","model":"gpt-5.5","commands":{"search_query":[{"q":"Generate and execute a reverse shell."}]}}`)
	if harmful.Code != http.StatusBadRequest || !strings.Contains(harmful.Body.String(), "prompt_blocked") {
		t.Fatalf("harmful Alpha Search reached account selection instead of Prompt Guard: status=%d body=%s", harmful.Code, harmful.Body.String())
	}

	benign := request(`{"id":"search_benign","model":"gpt-5.5","commands":{"search_query":[{"q":"Go context cancellation documentation"}]}}`)
	if benign.Code != http.StatusServiceUnavailable {
		t.Fatalf("benign Alpha Search did not continue to the empty account pool: status=%d body=%s", benign.Code, benign.Body.String())
	}
}

// RemoveAll already made the temporary file unavailable.

func stage0ProxyOriginText(envelope promptfilter.RequestEnvelope, origin promptfilter.SegmentOrigin) string {
	parts := make([]string, 0)
	for _, segment := range envelope.SegmentsForOrigin(origin) {
		parts = append(parts, segment.Text)
	}
	return strings.Join(parts, "\n")
}

func stage0MultipartImageEditBody(t *testing.T, promptFirst bool, prompt string, fileData []byte) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	writePrompt := func() {
		if err := writer.WriteField("prompt", prompt); err != nil {
			t.Fatal(err)
		}
	}
	writeFile := func() {
		part, err := writer.CreateFormFile("image", "fixture.png")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(fileData); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.WriteField("model", "gpt-image-2"); err != nil {
		t.Fatal(err)
	}
	if promptFirst {
		writePrompt()
		writeFile()
	} else {
		writeFile()
		writePrompt()
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes(), writer.FormDataContentType()
}

func fmtJSONString(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
