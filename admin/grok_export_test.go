package admin

import (
	"time"

	"github.com/codex2api/auth"
	"github.com/codex2api/database"
)

func grokOAuthRow() *database.AccountRow {
	return &database.AccountRow{
		ID:       5,
		Platform: "xai",
		Enabled:  true,
		Credentials: map[string]interface{}{
			"upstream_type":         auth.UpstreamGrok,
			"email":                 "mashir0zhao@gmail.com",
			"account_id":            "a7f5ab5b-8384-4a04-93d8-f55f55403b4e",
			"access_token":          "eyJhdCI.payload.sig",
			"refresh_token":         "rt-abc123",
			"id_token":              "eyJpZCI.payload.sig",
			"grok_client_id":        "b1a00492-073a-47ea-816f-4c329264a828",
			"grok_token_endpoint":   "https://auth.x.ai/oauth2/token",
			"grok_oidc_issuer":      "https://auth.x.ai",
			"grok_principal_type":   "User",
			"grok_principal_id":     "84ac4dd3-35d6-451c-84b6-019ad7419b41",
			"plan_type":             "supergrok_heavy",
			"base_url":              "https://cli-chat-proxy.grok.com/v1",
			"expires_at":            time.Now().Add(6 * time.Hour).UTC().Format(time.RFC3339),
			"grok_billing_detail":   `{"plan":"supergrok_heavy"}`,
			"grok_weekly_usage_pct": "22",
		},
		UpdatedAt: time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC),
	}
}

// auth_kind 是 CLI 文件的字段名，auth_mode 是本项目解析器认的字段名，两个都要有。

// expires_in 是相对当下的剩余秒数，6 小时后到期应落在合理区间。

// 用量/计费字段不进导出：导出只为迁移凭据，用量在新实例会重新探测。

// TestGrokExportRoundTripsThroughImporter 是补字段这个决策的验证点：
// 导出的文件必须能被 ParseGrokAuthJSON 解回来，且 client_id 不依赖 access_token
// 的 JWT claims —— AT 过期或缺失时也要能凭 refresh_token 继续刷新。

// 关键断言：access_token 是伪造的、JWT 掏不出 claims，client_id 只能来自显式字段。

// TestGrokExportAPIKeyRoundTrip API Key 账号必须写 auth_mode（解析器只认这个键），
// 否则会被当 OAuth 处理然后报缺 refresh_token。

// TestGrokExportFillsMissingOIDCDefaults 实测里 free 账号缺 base_url / oidc_issuer，
// 缺失时要补 Grok CLI 通道的固定值，不能留空导致回灌后刷不了 token。

// 无 expires_at 时不应输出 expired / expires_in。

// TestGrokExportDownloadName 下载文件名沿用仓库既有约定
// codex2api-<平台>-<时间戳>-<数量>.<ext>。

// 时间戳部分必须是 20060102-150405 形态（8 位日期 + 6 位时间）。

// TestBuildGrokExportZIPMembersUseEmailNames ZIP 内部成员按 <邮箱>.json 命名，
// 下载物改名不影响成员命名——成员名承载账号身份，也是解开后逐个导入的依据。

// TestBuildGrokExportZIPDedupesSharedEmail 多个 API Key 账号的 email 都是
// 占位串 xai-api-key，同名成员必须被追加序号而不是互相覆盖。

// 同名条目：第二个应被追加序号，不能互相覆盖。

// 每个成员都必须是合法 JSON。

// TestAccountRowToExportEntryDispatchesByPlatform 通用导出端点原先把 Grok 账号
// 也标成 type:"codex" 且丢掉 Grok 字段，导出的文件回灌必然失败。

// TestIsGrokAccountRow 平台字段与凭据 upstream_type 任一命中即算 Grok 账号。

// TestMarshalGrokExportEntryFormatting 排版对齐参考文件：2 空格缩进、键按字母序。

// 字母序：access_token 必须排在 auth_kind 之前。
