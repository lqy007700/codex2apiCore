package admin

import (
	"context"

	"testing"

	"github.com/codex2api/auth"
)

func insertAntigravityExportAccount(t *testing.T, handler *Handler, name string, credentials map[string]any) int64 {
	t.Helper()
	id, err := handler.db.InsertAccountWithUpstream(context.Background(), name, "google", auth.UpstreamAntigravity, credentials, "")
	if err != nil {
		t.Fatalf("insert Antigravity account: %v", err)
	}
	return id
}

// 默认导出:多个账号合成一个 JSON 数组文件,导入器可原样读回。

// format=zip 仍保留每账号一个文件的压缩包。
