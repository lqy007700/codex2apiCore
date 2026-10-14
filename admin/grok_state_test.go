package admin

import (
	"context"

	"testing"

	"github.com/codex2api/auth"
	"github.com/codex2api/database"
)

func newGrokAdminTestAccount(t *testing.T, serverURL string, disabled bool) (*Handler, *database.DB, int64) {
	t.Helper()
	db := newTestAdminDB(t)
	credentials := map[string]interface{}{
		"upstream_type": auth.UpstreamGrok,
		"api_key":       "xai-secret-test-key",
		"base_url":      serverURL + "/v1",
		"plan_type":     "api",
	}
	id, err := db.InsertAccountWithUpstream(context.Background(), "grok-test", "xai", auth.UpstreamGrok, credentials, "")
	if err != nil {
		t.Fatalf("InsertAccountWithUpstream: %v", err)
	}
	if disabled {
		if err := db.SetAccountEnabled(context.Background(), id, false); err != nil {
			t.Fatalf("SetAccountEnabled: %v", err)
		}
	}
	store := auth.NewStore(db, nil, nil)
	t.Cleanup(store.Stop)
	if err := store.LoadAccountByID(context.Background(), id); err != nil {
		t.Fatalf("LoadAccountByID: %v", err)
	}
	return &Handler{db: db, store: store}, db, id
}

func newGrokOAuthAdminTestAccount(t *testing.T, serverURL string) (*Handler, *database.DB, int64) {
	t.Helper()
	db := newTestAdminDB(t)
	id, err := db.InsertAccountWithUpstream(context.Background(), "grok-oauth-test", "xai", auth.UpstreamGrok, map[string]interface{}{
		"upstream_type": auth.UpstreamGrok,
		"access_token":  "oauth-access-token",
		"base_url":      serverURL + "/v1",
		"plan_type":     "archive-label",
	}, "")
	if err != nil {
		t.Fatalf("InsertAccountWithUpstream: %v", err)
	}
	store := auth.NewStore(db, nil, nil)
	t.Cleanup(store.Stop)
	if err := store.LoadAccountByID(context.Background(), id); err != nil {
		t.Fatalf("LoadAccountByID: %v", err)
	}
	return &Handler{db: db, store: store}, db, id
}

// TestSyncGrokAccountStatePreservesDeclaredModels 守护批量/编辑设置的模型白名单
// 不会被 5 分钟目录刷新盖成上游当前可见集（免费 OAuth 常见只剩 grok-4.6）。

// 探针体带 max_output_tokens:1，思考型模型的正常收尾是 response.incomplete。
// 只认 completed 会让每个通的 Responses 口被判成 unavailable（http_status=200
// 却 status=unavailable），进而永久关闭 Native 直通。

// response.failed 仍然是失败，不能被截断放行规则带偏。
