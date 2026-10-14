package admin

import (
	"net/http"
	"net/http/httptest"

	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func getImageJobResult(t *testing.T, router *gin.Engine, path, key string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func assertImageJobResultFields(t *testing.T, body []byte) {
	t.Helper()
	if len(body) > 4096 {
		t.Fatalf("result response unexpectedly large: %d bytes", len(body))
	}
	for _, field := range []string{"prompt", "params_json", "api_key_id", "api_key_name", "api_key_masked"} {
		if gjson.GetBytes(body, "job."+field).Exists() {
			t.Errorf("result must omit %s", field)
		}
	}
	for _, asset := range gjson.GetBytes(body, "job.assets").Array() {
		for _, field := range []string{"revised_prompt", "cache_b64_json", "storage_path"} {
			if asset.Get(field).Exists() {
				t.Errorf("result asset must omit %s", field)
			}
		}
	}
}
