package admin

// Claude 账号的两条补充授权入口:
//   1. sessionKey(cookie)一键换号:POST /accounts/claude/oauth/exchange-session-key
//      用户从已登录的 claude.ai 复制 sessionKey,服务端代替浏览器跑完 OAuth 三步,
//      可选择换出可刷新 OAuth 凭据或长效 Setup Token;
//   2. 令牌批量粘贴导入:POST /accounts/claude/import-tokens(旧名 import-setup-tokens)
//      直接吃官方 `claude setup-token` 产出的 sk-ant-oat01-… 长效令牌,以及
//      sk-ant-ort01-… OAuth refresh token(入库前先刷新换出 AT),多枚任意分隔。
//
// 两条入口最终都汇入 createClaudeAccount,与网页 OAuth / JSON 导入共享查重、指纹、
// 分组、预热等后续流程。

import (
	"github.com/codex2api/auth"
	"github.com/codex2api/database"
)

// exchangeClaudeSessionKeyReq 是 sessionKey 一键换号请求体。
type exchangeClaudeSessionKeyReq struct {
	SessionKey string `json:"session_key"`
	// Mode:oauth(默认)或 setup_token。
	Mode         string `json:"mode"`
	Name         string `json:"name"`
	ProxyURL     string `json:"proxy_url"`
	UseProxyPool bool   `json:"use_proxy_pool"`
	Timezone     string `json:"timezone"`
}

// ExchangeClaudeSessionKey 用 claude.ai sessionKey 一键换取凭据并入库。

// refreshClaudeCredentialsForImport 用裸 RT 换出 AT 与身份(邮箱/账号 UUID/套餐)。
// 测试可通过 Handler.refreshClaudeTokensForImport 注入,避免真打 platform.claude.com。

// importClaudeSetupTokensReq 是令牌批量粘贴导入请求体。text 与 tokens 可同时给,
// 服务端统一按前缀抽取并去重:sk-ant-oat01- 按 Setup Token(长效)入库,
// sk-ant-ort01- 按 OAuth refresh token 入库(入库前先刷新换出 AT)。
type importClaudeSetupTokensReq struct {
	Text   string   `json:"text"`
	Tokens []string `json:"tokens"`
	// Name 作为备注前缀;空=沿用现有 claude-N 序号继续编号。单枚令牌时直接用作备注。
	Name         string           `json:"name"`
	ProxyURL     string           `json:"proxy_url"`
	UseProxyPool bool             `json:"use_proxy_pool"`
	Timezone     string           `json:"timezone"`
	GroupRefs    []claudeGroupRef `json:"group_refs"`
}

// claudeSetupTokenImportMaxEntries 单次粘贴上限,防止误贴整份日志。
const claudeSetupTokenImportMaxEntries = 200

// ImportClaudeSetupTokens 批量导入 Setup Token。

// 代理:显式 proxy_url 对全部令牌固定;use_proxy_pool 时每枚令牌各取一条,
// 避免一批号挤在同一出口。

// 备注:Setup Token 没有身份信息,按 <prefix>-N 编号;Refresh Token 刷新后能拿到
// 邮箱,留空让 createClaudeAccount 回退到邮箱(单枚且用户给了备注时直接采用)。

// claudeSetupTokenNamePrefix 是自动编号备注的默认前缀。
const claudeSetupTokenNamePrefix = "claude"

// claudeSetupTokenAccountName 生成 `<prefix>-N` 形式的备注。

// nextClaudeSetupTokenIndex 扫描现有 Claude 账号里 `<prefix>-N` 形式的备注,返回最大 N+1;
// 没有则从 1 开始。读库失败时也从 1 开始(备注允许重复,不影响正确性)。

// nextClaudeIndexedName 在名字集合里找 `<prefix>-N`(前缀不区分大小写)的最大 N,返回 N+1。

// claudeAuthKindForRow 返回行的 Claude 凭据形态(非 Claude 行返回空),供列表/详情投影使用。
func claudeAuthKindForRow(row *database.AccountRow, isClaude bool) string {
	if row == nil || !isClaude {
		return ""
	}
	return auth.InferClaudeAuthKind(row.GetCredential(auth.ClaudeAuthKindCredentialKey), row.GetCredential("access_token"), row.GetCredential("refresh_token"))
}
