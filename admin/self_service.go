package admin

import (
	"context"

	"regexp"

	"sync"
	"time"
)

// ==================== 账号自助添加公开门户 ====================
//
// 面向外部/游客的公开页：填联系邮箱 → 生成 ChatGPT 授权链接 → 登录后回填授权码 →
// 账号以「待审核」状态入库（enabled=false + tag self-service），管理员批准后才进调度池。
// 本期不做人机验证，仅 IP 限流 + 邮箱格式校验。凭证与 token 不回显给提交者。

const (
	selfServiceTag       = "self-service"
	selfServiceSource    = "self-service"
	selfServiceRateLimit = 10        // 单 IP 窗口内最多请求数
	selfServiceRateWin   = time.Hour // 限流窗口
	selfServiceEmailMax  = 254       // 邮箱长度上限
)

var selfServiceEmailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// selfServiceRateLimiter 简单的按 IP 滑动计数限流（内存版，够用即可）。
type selfServiceRateLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

var globalSelfServiceLimiter = &selfServiceRateLimiter{hits: make(map[string][]time.Time)}

// allow 报告某 IP 是否在窗口内仍有配额；有则记一次。now 由调用方传入以便测试。

// PublicAccountPortalPageEnabled 报告账号自助门户是否开启（默认关）。
func (h *Handler) PublicAccountPortalPageEnabled(ctx context.Context) (bool, error) {
	if h == nil || h.db == nil {
		return false, nil
	}
	settings, err := h.db.GetSystemSettings(ctx)
	if err != nil {
		return false, err
	}
	if settings == nil {
		return false, nil
	}
	return settings.PublicAccountPortalPageEnabled, nil
}

// accountPortalMiddleware 门控公开门户 API：开关关闭一律 404；开启则按 IP 限流。

// GenerateAccountPortalAuthURL 生成 ChatGPT 授权链接（公开）。
// POST /api/account-portal/generate-auth-url  body: { contact_email }

// SubmitAccountPortalCode 用回填的授权码兑换并以「待审核」入库（公开）。
// POST /api/account-portal/submit-code  body: { session_id, code, state }

// 已在池中（或已提交待审核）：对提交者返回中性的成功语义，避免探测账号是否存在。

// 不回显 token / refresh_token；仅告知已进入待审核。

// upsertSelfServiceAccount 以「待审核」状态入库：enabled=false + note(联系人) + tag self-service，
// 不加入运行时调度池（管理员批准后再启用）。重复账号返回 errDuplicateOAuthIdentity。

// 待审核：禁用 + 备注 + 打标；不调用 Store.AddAccount，故不进调度池。

// 公开提交不走 /api/admin/accounts* 失效中间件；不主动丢快照的话，
// 账号页 SWR 会继续吐旧列表，待审核面板第一次查询就是 0。
