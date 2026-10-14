package admin

import (
	"context"
	"encoding/json"

	"fmt"
	"log"

	"regexp"

	"strings"
	"time"

	"github.com/codex2api/database"
	"github.com/codex2api/proxy"
)

const (
	inviteDefaultMaxEmails        = 10
	inviteUpperMaxEmails          = 50
	inviteRecipientCheckMaxEmails = 200
)

const (
	// v1:响应结构变更时升版，让 Redis 里旧部署写入的条目自然失效。
	inviteEligibilityCacheNamespace = "admin:codex-invite-eligibility:v1"
	inviteTrackingCacheNamespace    = "admin:codex-invite-tracking:v1"

	// 两级 TTL 分工不同：运行态缓存只挡秒级重复（切号来回点、多个管理员同时
	// 开着页面），快照 TTL 才决定多久真正回一次上游。上游这两个端点挂在
	// Cloudflare bot 管理后面，请求越密越容易被丢进 managed challenge，
	// 少打一次就是少一次被挑战的机会。
	inviteRuntimeCacheTTL = 60 * time.Second
	// 资格是月度累计配额，只会被「本网关发邀请」和「用户自己在官网发」改动，
	// 前者发生时会主动失效，所以窗口可以给得长一些。
	inviteEligibilitySnapshotTTL = 15 * time.Minute
	// 已发记录的兑换状态由受邀人行为驱动，网关完全不可知，只能靠短窗口兜。
	inviteTrackingSnapshotTTL = 5 * time.Minute
)

// inviteCachedPayload 是运行态缓存里的条目。ObservedAt/ExpiresAt 跟着数据一起存，
// 让 Redis 条目和数据库快照携带同一组元信息——否则从两条路径读出的「更新于」
// 会不一致。ExpiresAt 是数据的有效期（分钟级），与 Redis 键自身的 TTL（60s）无关。
type inviteCachedPayload struct {
	HTTPStatus int             `json:"http_status"`
	ObservedAt time.Time       `json:"observed_at"`
	ExpiresAt  time.Time       `json:"expires_at"`
	Payload    json.RawMessage `json:"payload"`
}

// inviteCacheMeta 随响应下发，让前端能显示数据是何时取回的、是不是缓存。
type inviteCacheMeta struct {
	Source     string     `json:"source"` // upstream / runtime / snapshot
	ObservedAt *time.Time `json:"observed_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

// inviteForceRefresh 判断是否绕过缓存直连上游。前端的手动刷新按钮与发送后的
// 重新拉取都会带上这个参数。

// 缓存作用域：同一份结果的完整标识。归一化后的参数才进 scope，否则「不传 limit」
// 与「传 100」会各存一份内容完全相同的条目。
func inviteEligibilityScope(programID, entrypoint string) string {
	return programID + "|" + entrypoint
}

func inviteTrackingScope(programID, period string, limit int) string {
	return fmt.Sprintf("%s|%s|%d", programID, period, limit)
}

// readInviteCache 依次尝试运行态缓存与数据库快照，命中则把结果解进 dest。
// 未命中返回 nil。任何一层出错都只记日志并继续回上游——缓存不该让功能不可用。
func (h *Handler) readInviteCache(ctx context.Context, namespace, kind string, accountID, generation int64, scope string, dest any) *inviteCacheMeta {
	now := time.Now()

	key := inviteCacheKey(accountID, generation, scope)
	var cached inviteCachedPayload
	if h.getRuntimeJSON(ctx, namespace, key, &cached) &&
		len(cached.Payload) > 0 && now.Before(cached.ExpiresAt) {
		if err := json.Unmarshal(cached.Payload, dest); err == nil {
			return &inviteCacheMeta{Source: "runtime", ObservedAt: &cached.ObservedAt, ExpiresAt: &cached.ExpiresAt}
		}
	}

	if h.db == nil {
		return nil
	}
	snap, err := h.db.GetCodexInviteSnapshot(ctx, accountID, kind, scope)
	if err != nil {
		log.Printf("读取邀请快照失败: account=%d kind=%s err=%v", accountID, kind, err)
		return nil
	}
	// 凭据代数不同说明账号重新授权过，旧快照描述的是另一份身份的资格。
	if snap == nil || snap.Expired(now) || snap.CredentialGeneration != generation {
		return nil
	}
	if err := json.Unmarshal(snap.Payload, dest); err != nil {
		return nil
	}
	// 回填运行态缓存，让同一份快照在接下来的 60 秒里不再回库。
	h.setRuntimeJSON(ctx, namespace, key, inviteCachedPayload{
		HTTPStatus: snap.HTTPStatus,
		ObservedAt: snap.ObservedAt,
		ExpiresAt:  snap.ExpiresAt,
		Payload:    snap.Payload,
	}, inviteRuntimeCacheTTL)
	return &inviteCacheMeta{Source: "snapshot", ObservedAt: &snap.ObservedAt, ExpiresAt: &snap.ExpiresAt}
}

// writeInviteCache 落一次成功观测。只在上游给出业务结论时调用——失败绝不入缓存，
// 见 database.CodexInviteSnapshot 的说明。
func (h *Handler) writeInviteCache(ctx context.Context, namespace, kind string, accountID, generation int64, scope string, httpStatus int, ttl time.Duration, value any) {
	payload, err := json.Marshal(value)
	if err != nil {
		log.Printf("编码邀请缓存失败: account=%d kind=%s err=%v", accountID, kind, err)
		return
	}
	now := time.Now()
	expiresAt := now.Add(ttl)

	h.setRuntimeJSON(ctx, namespace, inviteCacheKey(accountID, generation, scope), inviteCachedPayload{
		HTTPStatus: httpStatus,
		ObservedAt: now,
		ExpiresAt:  expiresAt,
		Payload:    payload,
	}, inviteRuntimeCacheTTL)

	if h.db == nil {
		return
	}
	if err := h.db.UpsertCodexInviteSnapshot(ctx, &database.CodexInviteSnapshot{
		AccountID:            accountID,
		Kind:                 kind,
		Scope:                scope,
		CredentialGeneration: generation,
		HTTPStatus:           httpStatus,
		Payload:              payload,
		ObservedAt:           now,
		ExpiresAt:            expiresAt,
	}); err != nil {
		log.Printf("写入邀请快照失败: account=%d kind=%s err=%v", accountID, kind, err)
	}
}

func inviteCacheKey(accountID, generation int64, scope string) string {
	return fmt.Sprintf("%d|%d|%s", accountID, generation, scope)
}

// invalidateInviteCache 在配额确实被改动后清缓存（目前只有「发送成功」一种情形）。
//
// 快照按账号整体删除，这是权威的那一层。运行态缓存的键含参数组合，无法枚举，
// 只删默认组合——非默认参数只可能由手工调 API 产生，最多陈旧一个 runtime TTL；
// 前端在发送后会带 refresh=1 重新拉取，走不到缓存。

var inviteEmailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
var inviteEmailSplitter = regexp.MustCompile(`[,;\r\n\t ]+`)

type inviteRequest struct {
	Emails     []string `json:"emails"`
	EmailsText string   `json:"emails_text"`
	// 上游已把旧的单字段 referral_key 拆成 program_id + entrypoint，留空走默认计划。
	ProgramID  string `json:"program_id"`
	Entrypoint string `json:"entrypoint"`
	ProxyURL   string `json:"proxy_url"`
	MaxEmails  int    `json:"max_emails"`
}

type inviteRecipientCheckRequest struct {
	Emails []string `json:"emails"`
}

// CheckInviteRecipients 批量查询收件邮箱是否已经被本网关预占或邀请过。
// POST /api/accounts/invite/recipients/check

// SendInvite 通过指定账号向 ChatGPT 推荐邀请端点发送邀请邮件。
// POST /api/accounts/:id/invite

// 不能在没有持久账本的情况下先发邮件：上游成功后将无法阻止再次邀请。

// 网络错误不能证明上游没有受理。为保证同一邮箱至多发送一次，保守标记为
// unknown，等待 tracking 或管理员核对，而不是自动释放后允许立即重试。

// 挑战页明确没有进入邀请业务端点，可以安全释放，保留输入供用户稍后重试。

// 上游明确给出业务拒绝时本次没有成功发送。若原因明确说明收件人已收到
// 邀请，则把对应邮箱记为 known_invited；其余预占释放，允许修正后重试。

// 上游已经给出收件人级证据，但本地没能精确固化时不能解锁重试；
// 把本批剩余预占转 unknown，宁可人工核对也不冒发第二封的风险。

// 只要请求真到过上游，配额就可能已经动过——收件人级被拒也可能已扣掉发送次数。
// 唯一确定没动的是被 Cloudflare 挡在门外的那次。宁可多回一次上游，也不能
// 让页面继续显示发送前的剩余次数。

// 常见：无 referral 资格的账号返回 403。透传上游响应供前端展示。

// 邮件已经发出，绝不能把成功响应改成可重试的失败。预占行仍会阻止重复发送。

// rememberInviteTrackingRecipients 把上游 tracking 证明存在的历史邀请并入全局账本。
// 无论邀请已兑换、过期还是可重发，产品规则都是「一个邮箱只邀请一次」，因此都保留。
func (h *Handler) rememberInviteTrackingRecipients(ctx context.Context, accountID int64, programID, entrypoint string, result *proxy.CodexInviteTracking) {
	if h.db == nil || result == nil || !result.OK || result.Challenged || len(result.Items) == 0 {
		return
	}
	evidence := make([]database.CodexInviteRecipientEvidence, 0, len(result.Items))
	for _, item := range result.Items {
		if strings.TrimSpace(item.Email) == "" {
			continue
		}
		invitedAt := time.Time{}
		if item.CreatedAt != "" {
			invitedAt, _ = time.Parse(time.RFC3339Nano, item.CreatedAt)
		}
		evidence = append(evidence, database.CodexInviteRecipientEvidence{
			Email:                   item.Email,
			ReferralID:              item.ReferralID,
			InviteURL:               item.InviteURL,
			UpstreamRecipientStatus: item.Status,
			InvitedAt:               invitedAt,
		})
	}
	if len(evidence) == 0 {
		return
	}
	_, err := h.db.UpsertCodexInviteRecipientsFromTracking(ctx, accountID, programID, entrypoint, result.StatusCode, evidence)
	if err != nil {
		log.Printf("回填邀请邮箱账本失败: account=%d err=%v", accountID, err)
	}
}

// GetInviteEligibility 查询指定账号的推荐邀请资格与剩余配额。
// GET /api/accounts/:id/invite/eligibility

// 只有上游给出业务结论时才入缓存。挑战页借用 403、与「无资格」同码，
// 缓存它等于把一次 Cloudflare 拦截固化成十五分钟的「这个号不能发邀请」。
// 200 + should_show=false 是真结论，照常缓存。

// 上游非 2xx 不算网关错误，透传供前端展示（如无资格账号的 403）。

// GetInviteTracking 查询指定账号已发出的邀请及兑换状态。
// GET /api/accounts/:id/invite/tracking

// resolveInviteAccount 解析路径上的账号 ID、校验凭证并解出代理。
// 校验失败时已写入错误响应，返回 ok=false。

// collectInviteEmails 合并 emails[] 与文本来源，去重（忽略大小写）、校验格式、夹紧上限。
