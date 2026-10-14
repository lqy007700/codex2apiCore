package admin

import (
	"context"

	"testing"

	"github.com/codex2api/auth"
	"github.com/codex2api/cache"
	"github.com/codex2api/database"
)

// 另一 IP 不受影响

// 窗口外恢复

func newSelfServiceHandler(t *testing.T, enabled bool) (*Handler, *database.DB) {
	t.Helper()
	db := newTestAdminDB(t)
	tc := cache.NewMemory(1)
	t.Cleanup(func() { tc.Close() })
	store := auth.NewStore(db, tc, nil)
	handler := NewHandler(store, db, tc, nil, "admin-secret")

	ctx := context.Background()
	settings, err := db.GetSystemSettings(ctx)
	if err != nil {
		t.Fatalf("GetSystemSettings: %v", err)
	}
	if settings == nil {
		settings = &database.SystemSettings{}
	}
	settings.PublicAccountPortalPageEnabled = enabled
	if err := db.UpdateSystemSettings(ctx, settings); err != nil {
		t.Fatalf("UpdateSystemSettings: %v", err)
	}
	return handler, db
}

// 无效邮箱 → 400

// 有效邮箱 → 200 且返回 auth_url + session_id

// 未进调度池

// 重复提交 → errDuplicateOAuthIdentity

// 管理员批准（enable=true）应把账号加载进调度池
