package admin

import (
	"encoding/json"

	"time"
)

// importGrokSSOReq 是 SSO 批量导入的请求体。
// Tokens 是原始导入内容（JSON {"accounts":[...]} 或每行一个 sso token 的纯文本）。
type importGrokSSOReq struct {
	Tokens   string   `json:"tokens"`
	BaseURL  string   `json:"base_url"`
	Models   []string `json:"models"`
	ProxyURL string   `json:"proxy_url"`
	// GroupIDs 让导入时就把新账号绑进指定分组；跳过的重复账号不受影响。
	GroupIDs json.RawMessage `json:"group_ids"`
}

type grokSSOImportItem struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	ID    int64  `json:"id,omitempty"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	// Updated/Revived 标记该条命中既有身份:凭据已合并进账号 ID(Revived 表示
	// 该账号原在回收站,本次已复活),而非新建。
	Updated bool `json:"updated,omitempty"`
	Revived bool `json:"revived,omitempty"`
}

const (
	grokSSOImportMaxTokens = 50
	// SSO 每条要跑完整个 device flow（多次往返），并发略高于 refresh 以缩短总墙钟。
	grokSSOImportConcurrent = 6
	grokSSOImportPerToken   = 75 * time.Second
)

// ImportGrokSSO 批量把 Grok Web 的 sso token 转成 Build(OAuth) 账号并入库。
// POST /api/admin/accounts/grok/sso/import

// 分组校验放在导入之前：分组 ID 打错时不该留下一批没绑上分组的账号。

// 每个 token 独立超时，有界并发转换；不复用请求 ctx（请求可能先于转换结束）。

// 按 subject 去重：预载已有 Grok 账号，锁保护并发读写，避免重复导入同一账号。

// 身份栅栏含回收站账号:命中时合并凭据(回收站复活)而非报错,
// 否则删过的号重新导入永远失败(issue #602)。

// 异步 billing 探针，与其它添加路径一致

// importGrokRefreshReq 是 refresh_token 批量导入的请求体（Tokens 每行一个 refresh_token）。
type importGrokRefreshReq struct {
	Tokens   string   `json:"tokens"`
	BaseURL  string   `json:"base_url"`
	Models   []string `json:"models"`
	ProxyURL string   `json:"proxy_url"`
	// GroupIDs 让导入时就把新账号绑进指定分组；跳过的重复账号不受影响。
	GroupIDs json.RawMessage `json:"group_ids"`
}

const (
	grokRefreshImportMaxTokens = 5000
	grokRefreshImportPerToken  = 30 * time.Second
)

// parseTokenLines 把纯文本按行拆成去重后的 token 列表（trim、跳过空行与 # 注释）。

// ImportGrokRefreshTokens 批量用 refresh_token 刷出 OAuth 账号并入库。
// POST /api/admin/accounts/grok/refresh/import

// 分组校验放在导入之前：分组 ID 打错时不该留下一批没绑上分组的账号。

// createGrokOAuthAccount 会写 store，去重锁保护 subject 集合的并发读写。

// 上游未轮换时沿用原 refresh_token

// 身份栅栏含回收站账号:命中时合并凭据(回收站复活)而非报错,
// 否则删过的号重新导入永远失败(issue #602)。

// importedGrokAccountIDs 从导入结果里挑出真正新建的账号 ID（跳过重复与失败的条目）。
