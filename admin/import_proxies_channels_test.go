package admin

import (
	"context"

	"testing"

	"github.com/codex2api/auth"
	"github.com/codex2api/database"
	"github.com/gin-gonic/gin"
)

// grokFileProxyBinding 是 Grok 侧唯一读取代理的地方（ParseGrokAuthJSON 只认凭据
// 字段），解析漏了就等于开关整体失效。

// 文件是否合法由 ParseGrokAuthJSON 判定并报错，这里再报一次只会搅乱错误信息。

// proxy_enabled 类型不对时不能把它当成 false —— 那会误报"源端禁用"告警。

func newGrokProxyImportHandler(t *testing.T) (*Handler, *auth.Store, *database.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := newTestAdminDB(t)
	store := auth.NewStore(db, nil, &database.SystemSettings{
		MaxConcurrency: 2, ProxyPoolEnabled: true,
	})
	t.Cleanup(store.Stop)
	store.SetProxyPoolEnabled(true)
	if err := store.ReloadProxyPool(); err != nil {
		t.Fatalf("ReloadProxyPool: %v", err)
	}
	return &Handler{db: db, store: store}, store, db
}

type grokProxyImportResponse struct {
	Imported        int                   `json:"imported"`
	Items           []grokBatchImportItem `json:"items"`
	ProxiesImported *int                  `json:"proxies_imported"`
	ProxiesSkipped  *int                  `json:"proxies_skipped"`
	ProxyWarning    string                `json:"proxy_warning"`
}

func grokAccountRowByRefreshToken(t *testing.T, db *database.DB, refreshToken string) *database.AccountRow {
	t.Helper()
	rows, err := db.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	for _, row := range rows {
		if row.GetCredential("refresh_token") == refreshToken {
			return row
		}
	}
	t.Fatalf("no active account with refresh_token %q", refreshToken)
	return nil
}

// 全链路:Grok 文件带代理 → 代理注册进表和内存池 → 账号绑上它 → 立刻可调度。
// 少了池同步那一环，账号绑的就是个已在 managedProxySet 却不在 proxyPoolSet 的
// URL，整批号会被判为无可用出口。

// 开关关闭时文件里的 proxy_url 完全不生效：账号用表单代理，代理表保持干净。

// 命中既有身份时,文件带来的代理不覆盖对方已有的绑定——那里可能已经做过精细
// 分配。表单填的代理是显式换绑意图,维持覆盖语义。
//
// 走的是「删除后重新导入」这条真实路径:活跃账号会在身份去重那一步被直接跳过,
// 只有回收站里的账号才会命中合并/复活分支(issue #602),而这恰恰是代理覆盖语义
// 唯一生效的地方。

// importThenRecycle 先导入一个账号(可带表单代理),再把它软删进回收站,
// 返回账号 ID,供下一次导入命中合并分支。

// 账号还没绑代理时,文件代理该填进去——"不覆盖"不等于"永不写入"。

// 一批文件里的重复代理只注册一条,非法的跳过并让对应账号退回表单代理。

// 非法代理被清空后必须退回表单值，而不是绑着一个没进池的 URL。

type antigravityProxyImportResponse struct {
	Imported        int                       `json:"imported"`
	Items           []AntigravityImportItemJS `json:"items"`
	ProxiesImported *int                      `json:"proxies_imported"`
	ProxyWarning    string                    `json:"proxy_warning"`
}

// AntigravityImportItemJS 只取断言要用的字段，避免和内部结构体耦合。
type AntigravityImportItemJS struct {
	ID    int64  `json:"id"`
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

// Antigravity 导入一直会采用文件里的 proxy_url，开关只决定它要不要入代理表。
// 入表这一步不能少：代理池开启后，绑着未入表 URL 的账号在管理页看不见、也进不了
// 轮转；而一旦入表就必须同步内存池，否则整批号会被判为无可用出口。

// 开关关闭时维持既有行为：账号照旧绑上文件里的代理，只是不入表。

// Antigravity 导出写 proxy_enabled，导入侧要认下来，否则"源端禁用"的告警永远不触发。
