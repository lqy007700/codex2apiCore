package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"testing"

	"github.com/codex2api/proxy"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/sjson"
)

func modelTraceBankWithBuiltAt(t *testing.T, builtAt string) []byte {
	t.Helper()
	body, err := sjson.SetBytes(proxy.ModelTraceEmbeddedBankForTest(), "built_at", builtAt)
	if err != nil {
		t.Fatalf("set built_at: %v", err)
	}
	return body
}

func callModelTraceBank(t *testing.T, handler *Handler, method string, fn gin.HandlerFunc, out any) {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, "/api/admin/modeltrace/bank", nil)
	fn(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("%s status=%d body=%s", method, recorder.Code, recorder.Body.String())
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), out); err != nil {
		t.Fatalf("decode %s: %v", recorder.Body.String(), err)
	}
}

// A bank trained before the active one must not replace it.

// Simulates upgrading to a release whose embedded bank is newer than the
// override an operator installed earlier.
