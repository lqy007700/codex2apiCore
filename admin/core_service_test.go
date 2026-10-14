package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/codex2api/auth"
	"github.com/codex2api/cache"
	"github.com/codex2api/database"
	"github.com/codex2api/proxy"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoreServiceRoutesAndSettingsBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.New("sqlite", filepath.Join(t.TempDir(), "core.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	nativeID, err := db.InsertAccount(context.Background(), "native", "test-refresh", "")
	if err != nil {
		t.Fatal(err)
	}
	legacyID, err := db.InsertOpenAIResponsesAccount(context.Background(), "legacy", map[string]interface{}{"upstream_type": "openai_responses", "api_key": "test"}, "")
	if err != nil {
		t.Fatal(err)
	}
	db.EnableCodexOnly()
	settings := defaultBootstrapSettings()
	if err := db.UpdateSystemSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	tc := cache.NewMemory(8)
	defer tc.Close()
	store := auth.NewStore(db, tc, settings)
	store.EnableCodexOnly()
	defer store.Stop()
	h := NewHandler(store, db, tc, proxy.NewRateLimiter(0), "core-test-admin")
	router := gin.New()
	h.RegisterRoutes(router)
	paths := map[string]bool{}
	for _, r := range router.Routes() {
		paths[r.Method+" "+r.Path] = true
		for _, retired := range []string{"/images", "/prompt-filter", "/prompt-policy", "/quality", "/channel-monitor", "/accounts/grok", "/accounts/claude", "/accounts/antigravity", "/api/key-usage", "/api/account-portal", "/proxy-risk"} {
			if strings.Contains(r.Path, retired) {
				t.Fatalf("retired route exposed: %s", r.Path)
			}
		}
	}
	for _, retired := range []string{
		"GET /api/admin/stats",
		"GET /api/admin/accounts/analysis",
		"GET /api/admin/accounts/page-stats",
		"GET /api/admin/accounts/live",
		"GET /api/admin/accounts/recycle-bin",
		"GET /api/admin/accounts/:id/auth-json",
		"GET /api/admin/accounts/:id/subscription",
		"GET /api/admin/accounts/:id/wham-daily-usage",
		"GET /api/admin/ops/overview",
		"POST /api/admin/models/sync",
		"POST /api/admin/models/refresh-all",
		"GET /api/admin/account-groups",
	} {
		if paths[retired] {
			t.Fatalf("retired route exposed: %s", retired)
		}
	}
	for _, path := range []string{"GET /api/admin/core-status", "GET /api/admin/accounts", "PATCH /api/admin/accounts/:id/scheduler", "GET /api/admin/settings", "PUT /api/admin/settings", "POST /api/admin/keys"} {
		if !paths[path] {
			t.Fatalf("missing core route %s", path)
		}
	}
	call := func(method, path, body string, authorized bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		if authorized {
			req.Header.Set("X-Admin-Key", "core-test-admin")
		}
		out := httptest.NewRecorder()
		router.ServeHTTP(out, req)
		return out
	}
	if out := call("GET", "/api/admin/core-status", "", false); out.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d", out.Code)
	}
	for _, action := range []string{"batch-update", "batch-delete", "batch-reset-status", "batch-refresh"} {
		body := fmt.Sprintf(`{"ids":[%d,%d],"enabled":false}`, nativeID, legacyID)
		if out := call("POST", "/api/admin/accounts/"+action, body, true); out.Code != http.StatusBadRequest {
			t.Fatalf("legacy batch accepted: %s status=%d body=%s", action, out.Code, out.Body.String())
		}
	}
	native, err := db.GetAccountByID(context.Background(), nativeID)
	if err != nil || !native.Enabled {
		t.Fatal("rejected mixed batch changed native account")
	}
	legacy, err := db.GetAccountByID(context.Background(), legacyID)
	if err != nil || !legacy.Enabled {
		t.Fatal("rejected mixed batch changed legacy account")
	}
	out := call("GET", "/api/admin/settings", "", true)
	if out.Code != 200 {
		t.Fatalf("settings: %d %s", out.Code, out.Body.String())
	}
	var view map[string]interface{}
	if err := json.Unmarshal(out.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	for key := range view {
		if strings.HasPrefix(key, "prompt_") || strings.HasPrefix(key, "image_") || strings.HasPrefix(key, "grok_") || strings.HasPrefix(key, "claude_") || strings.HasPrefix(key, "antigravity_") || key == "codex_telemetry_enabled" {
			t.Fatalf("retired setting exposed: %s", key)
		}
	}
	out = call("PUT", "/api/admin/settings", `{"prompt_filter_enabled":true}`, true)
	if out.Code != 400 {
		t.Fatalf("retired setting accepted: %d %s", out.Code, out.Body.String())
	}
	current, _ := db.GetSystemSettings(context.Background())
	if current.PromptFilterEnabled {
		t.Fatal("rejected patch changed persisted policy")
	}
	out = call("PUT", "/api/admin/settings", `{"max_concurrency":3}`, true)
	if out.Code != 200 || store.GetMaxConcurrency() != 3 {
		t.Fatalf("core setting failed: %d %s", out.Code, out.Body.String())
	}
}
