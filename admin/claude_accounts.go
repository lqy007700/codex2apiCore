package admin

// Claude Code(Anthropic)账号的后台授权/导入端点。
//
// 凭据形态见 auth/claude_auth_kind.go(oauth 可刷新 / setup_token 长效)。入口:
//   1. 网页授权两步式(完整 OAuth 或 Setup Token,由 mode 决定):
//        POST /accounts/claude/oauth/auth-url      → 返回授权 URL + state(+mode/redirect_uri)
//        POST /accounts/claude/oauth/exchange-code → 用 state+code 换 token 并入库
//      服务端用一个带 TTL 的内存表按 state 暂存 verifier 与交换参数。
//   2. sessionKey 一键换号(claude_setup_token.go):
//        POST /accounts/claude/oauth/exchange-session-key
//   3. 凭据 JSON 导入(单对象 / 数组 / accounts bundle,兼容 CLI 产物与导出文档):
//        POST /accounts/claude/import
//   4. Setup Token 批量粘贴(claude_setup_token.go):
//        POST /accounts/claude/import-setup-tokens
//
// 四条入口最终都汇入 createClaudeAccount 共享查重、指纹、分组与预热流程。

import (
	"sync"
	"time"

	"github.com/codex2api/auth"
	"github.com/codex2api/database"
)

// claudeOAuthPending 暂存一次登录的 state→verifier 及交换参数(带 TTL)。
type claudeOAuthPending struct {
	verifier  string
	createdAt time.Time
	// exchange 记录生成授权 URL 时的 redirect_uri / 令牌形态,交换时必须原样回传。
	exchange auth.ClaudeExchangeOptions
}

var (
	claudeOAuthMu      sync.Mutex
	claudeOAuthPendMap = map[string]claudeOAuthPending{}
)

const claudeOAuthSessionTTL = 15 * time.Minute

// 顺带清理过期项,避免内存无限增长。

// claudeAuthModeReq 是授权类入口共用的「令牌形态」参数:oauth(默认,可刷新)或
// setup_token(长效、仅推理)。
type claudeAuthModeReq struct {
	Mode string `json:"mode"`
}

// parseClaudeAuthMode 归一化 mode 参数;非法值报错。

// GenerateClaudeAuthURL 发起一次 Claude 授权,返回授权 URL 与 state。请求体可选
// {"mode":"setup_token"} 申请长效 Setup Token(仅 user:inference,1 年有效,无 RT)。

type exchangeClaudeCodeReq struct {
	State string `json:"state"`
	Code  string `json:"code"`
	Name  string `json:"name"`
	// ProxyURL 指定固定代理;留空且 UseProxyPool=true 时从代理池自动取一个。
	ProxyURL     string `json:"proxy_url"`
	UseProxyPool bool   `json:"use_proxy_pool"`
	// Timezone 账号绑定的 IANA 时区(如 Asia/Shanghai),用于指纹一致性;空=不指定。
	Timezone string `json:"timezone"`
}

// resolveClaudeLoginProxy 决定本次登录/导入使用并固定到账号的代理:
// 显式 proxy_url 优先;否则若 use_proxy_pool=true 则从代理池轮询取一个。
// 返回的代理会同时用于 OAuth 交换、后续刷新与推理出站,保证 IP 一致(防风控)。

// ExchangeClaudeOAuthCode 用 state+code 换取 token 并把账号写入池子。

type importClaudeTokenReq struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Email        string `json:"email"`
	AccountID    string `json:"account_id"`
	ExpiresAt    string `json:"expires_at"`
	Name         string `json:"name"`
	ProxyURL     string `json:"proxy_url"`
	UseProxyPool bool   `json:"use_proxy_pool"`
	Timezone     string `json:"timezone"`
}

// ImportClaudeToken 直接吃 cmd/claude_login -out 产出的 token JSON 入库。

// Keep the legacy single-document response shape while allowing a portable
// JSON array / {accounts:[...]} bundle to use the same endpoint.

// 未声明过期时刻时:OAuth AT 按 30 分钟(会很快触发刷新),Setup Token 按 1 年。

// RefreshClaudeModels 重新拉取指定 Claude 账号真实可用的模型并落库(动态维护,
// 不用重新导入)。路由 POST /accounts/:id/claude/models。

// 直接更新内存账号的 Models,即时生效(LoadAccountByID 对已存在账号是 no-op)。

// RefreshAllClaudeModels 为所有 Claude 账号重新拉取真实可用模型(定价页"模型目录"用)。
// 路由 POST /accounts/claude/models/refresh。

// refreshAllClaudeModels 逐个 Claude 账号拉取上游模型清单并写回凭据，
// 返回成功/失败账号数；列账号失败时返回 err。

// resolveClaudeModelProxy mirrors the request path's proxy precedence for
// control-plane model discovery: an account-level/managed group proxy wins,
// then the row's persisted proxy is used as a safe fallback when the account
// is not currently present in the runtime store.

func (h *Handler) invalidateClaudeCatalogCaches() {
	if h == nil {
		return
	}
	h.expireAccountListSnapshot(database.UpstreamChannelClaude)
	h.accountAnalysisCacheMu.Lock()
	if h.accountAnalysisCache != nil {
		delete(h.accountAnalysisCache, database.UpstreamChannelClaude)
	}
	h.accountAnalysisCacheMu.Unlock()
}

// insertClaudeAccount 把一份 Claude token 落库并加载进运行时池子(去重按 account_id)。
// timezone 为空时不指定时区。会为该账号生成一套稳定的 Claude Code 指纹并随凭据落库,
// 之后每次上游请求原样套用(见 proxy/claude_upstream.go)。
// claudePlanOrDefault 取 profile 推导的档位,空则回退通用 "claude"。

// createClaudeAccount is the shared insertion path for OAuth and portable
// credential imports. It never writes token values to logs or response bodies.

// 凭据形态:显式声明优先(导入文档 / 交换结果),否则按是否有 RT 推断。声明为
// oauth 却没有 RT 的文档拒绝入库,避免把一个 1 小时就过期的 AT 当成账号。

// Reject an existing credential before consuming its rotating refresh token.

// Setup Token 没有 RT;即使交换响应带了也不保存,避免刷新路径误用。

// 未显式指定时区时,回退到 ClaudeCode 全局默认(系统设置里配置)。

// 生成稳定指纹(UA / x-app / x-stainless-*),存进 custom_headers 供请求期套用。

// API Key 账号没有生成指纹;custom_headers 是运维显式配置的出站请求头
// (issue #647),保留头(鉴权/Content-Type/Accept 等)拒绝入库。

// 对 OAuth/Setup Token 是指纹替换模式;对 API Key 是可选的 Claude Code 客户端
// 身份仿真开关(空=透传)。两种形态都按账号持久化。

// 动态拉取该账号**真实可用**的模型(Anthropic /v1/models),存进 credentials.models;
// 失败不阻断导入(DefaultClaudeModelIDsForAccount 会回退到内置兜底集)。
// API Key 账号的发现请求带同一套自定义头/客户端身份,保证网关看到的请求形态一致。

// A large bundle should not serialize one upstream /v1/models request per
// account. Leave the catalog empty so the normal default Claude model set
// is used; operators can refresh the catalog explicitly after import.

// 查重与插入置于同一临界区，避免并发导入同一账号各插一条（TOCTOU）。
// 复用 antigravity/grok 相同的合并去重锁，跨 provider 一致。

// A refresh token is itself a stable credential identity. Check it even
// when the provider also supplied an account_id; providers may rotate or
// omit that identifier while leaving the same refresh token valid.

// Setup Token 没有 RT 也常常没有 account_id(scope 不含 profile),AT 本身就是
// 唯一身份:同一枚长效令牌不允许重复入库。

// Keep Claude imports on the bounded warmup queue. ProbeUsageSnapshot routes
// this account to Anthropic Messages and never to WHAM/Responses.
