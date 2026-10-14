package admin

import (
	"regexp"
	"sort"

	"strings"
	"sync/atomic"

	"github.com/codex2api/auth"
	"github.com/codex2api/database"
	"github.com/codex2api/proxy"
)

// modelVersionNumberRE 提取模型名中的数字段，用于版本排序（如 gpt-5.6-luna → 5,6）。
var modelVersionNumberRE = regexp.MustCompile(`\d+`)

// preferredBillingModelOrder 定价列表优先展示 GPT-6，再展示 GPT-5.6。
var preferredBillingModelOrder = []string{
	"gpt-6-astra",
	"gpt-6-sol",
	"gpt-6-luna",
	"gpt-5.6-sol",
	"gpt-5.6-terra",
	"gpt-5.6-luna",
}

// modelPreferredRank 返回置顶优先级（越小越靠前）；非置顶模型返回 -1。

// 变体后缀 / 思考强度别名：gpt-5.6-sol-high、gpt-5.6-sol(xhigh)

// modelVersionParts 从模型 ID 中取出数字序列，供新→旧排序使用。

// compareModelKeysNewestFirst 比较两个模型键：
// 1) 置顶模型（sol/terra/luna）固定靠前；
// 2) 其余按版本号从新到旧；同版本再按名称字典序。
// 返回 -1 表示 a 应排在 b 前面，1 表示 a 在 b 后面，0 表示相等。

// 同置顶组内按名称稳定排序（sol-high 跟在 sol 后等）。

// 无版本号的模型沉底。

// 公共前缀相同：段更多视为更高版本（5.6 > 5）。

// sortModelKeysNewestFirst 将模型键按版本号从新到旧排序。

// grokBillingModelIDs 返回定价页要展示的 Grok 模型：各 Grok 账号声明白名单的并集，
// 外加默认集（只要存在未声明白名单的 Grok 账号——这类账号按默认集对外放行）。
// 没有 Grok 账号时返回空，纯 Codex 部署的定价页不受影响。
func (h *Handler) grokBillingModelIDs() []string {
	if h == nil || h.store == nil {
		return nil
	}
	ids := h.grokChannelModels()
	// 默认集按凭据类型不同（OAuth 走 CLI 通道、API Key 走公开 API），
	// 两种通道各取一次，不能见到第一个未声明账号就收工。
	oauthCovered, apiKeyCovered := false, false
	for _, account := range h.store.Accounts() {
		if !account.IsGrokAPI() || len(account.GrokModels()) > 0 {
			continue
		}
		isAPIKey := account.GrokAuthKind() == auth.GrokAuthKindAPIKey
		if (isAPIKey && apiKeyCovered) || (!isAPIKey && oauthCovered) {
			continue
		}
		if isAPIKey {
			apiKeyCovered = true
		} else {
			oauthCovered = true
		}
		ids = append(ids, proxy.DefaultGrokModelIDsForAccount(account)...)
		if oauthCovered && apiKeyCovered {
			break
		}
	}
	return ids
}

// grokDefaultDisplayModelIDs 是定价页始终展示的 Grok 内置模型集(即使没有 Grok 账号),
// 与 Codex 内置模型的常显行为对齐。取 OAuth 与 API Key 两套文本默认集的并集(后者为超集),
// 再并入生图/生视频默认集:媒体模型按单位(张/秒)计费,同样需要常显的定价入口。
func grokDefaultDisplayModelIDs() []string {
	ids := make([]string, 0, 16)
	ids = append(ids, auth.GrokOAuthDefaultModelIDs()...)
	ids = append(ids, auth.GrokAPIKeyDefaultModelIDs()...)
	ids = append(ids, auth.GrokImageDefaultModelIDs()...)
	ids = append(ids, auth.GrokVideoDefaultModelIDs()...)
	return ids
}

// modelPricingRow 是定价管理页每个规范模型的一行：当前生效价 + 来源。
type modelPricingRow struct {
	Model          string                        `json:"model"`
	Channel        string                        `json:"channel"` // codex / grok / antigravity / claude —— 供前端按 provider 分组
	Source         string                        `json:"source"`  // custom / synced / default
	Pricing        database.ModelPricingOverride `json:"pricing"`
	CanonicalModel string                        `json:"canonical_model,omitempty"`
	IsAlias        bool                          `json:"is_alias,omitempty"`
	// MediaUnit 非空表示媒体模型(image=按张、second=按秒),价格看 media_unit_cost
	// 而非 token 单价;media_unit_cost 为 0 即未定价。
	MediaUnit string `json:"media_unit,omitempty"`
}

// claudeChannelModels 返回定价页要展示的 Claude 模型:各 Claude 账号可见模型的并集。
// 没有 Claude 账号时返回空,纯 Codex/其它部署的定价页不受影响。
func (h *Handler) claudeChannelModels() []string {
	if h == nil || h.store == nil {
		return nil
	}
	seen := make(map[string]struct{})
	models := make([]string, 0)
	for _, account := range h.store.Accounts() {
		if account == nil || !account.IsClaudeOAuth() {
			continue
		}
		for _, model := range proxy.DefaultClaudeModelIDsForAccount(account) {
			key := strings.ToLower(strings.TrimSpace(model))
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			models = append(models, model)
		}
	}
	sort.Strings(models)
	return models
}

// claudeAvailableChannelModels returns models from enabled, non-banned Claude
// accounts for request-facing catalogs. Pricing/history still use
// claudeChannelModels so a disabled account cannot make an unusable model
// selectable while its historical cost data remains visible to administrators.
func (h *Handler) claudeAvailableChannelModels() []string {
	if h == nil || h.store == nil {
		return nil
	}
	seen := make(map[string]struct{})
	models := make([]string, 0)
	for _, account := range h.store.Accounts() {
		if account == nil || !account.IsClaudeOAuth() ||
			atomic.LoadInt32(&account.Disabled) != 0 ||
			atomic.LoadInt32(&account.DispatchPaused) != 0 {
			continue
		}
		account.Mu().RLock()
		status := account.Status
		tier := account.HealthTier
		account.Mu().RUnlock()
		if status == auth.StatusError || tier == auth.HealthTierBanned {
			continue
		}
		for _, model := range proxy.DefaultClaudeModelIDsForAccount(account) {
			model = strings.TrimSpace(model)
			key := strings.ToLower(model)
			if key == "" || account.IsModelRateLimited(model) {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			models = append(models, model)
		}
	}
	sort.Strings(models)
	return models
}

func modelPricingManagementKeys(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		key := database.PricingManagementModelKey(id)
		if key == "" || strings.Contains(key, "(") {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

// ListModelPricing 返回各规范模型的当前生效定价与来源，供设置页定价表展示。

// 取当前对外暴露的模型，映射到规范定价键去重（退役模型自然被排除）。

// Grok 模型不在 Codex 注册表里，但同样对外暴露、同样按 token 计费，
// 单独并进来，否则定价页看不到 grok-4.5 这类模型。

// Grok 内置默认模型始终并入,使定价页像 Codex 内置模型一样常显 grok 家族,
// 即使当前没有任何 Grok 账号(官方同步的 grok 采集逻辑不受影响)。

// 每个渠道内按新版本在前排序；渠道之间整体拼接,避免版本号交叉穿插。

// UpdateModelPricingRequest 设置/清除某模型的 custom 定价覆盖。
type UpdateModelPricingRequest struct {
	Model   string                         `json:"model"`
	Reset   bool                           `json:"reset"`   // true 时清除该模型覆盖，回退代码默认
	Pricing *database.ModelPricingOverride `json:"pricing"` // reset=false 时必填
}

// UpdateModelPricing 写入/清除某模型的 custom 定价覆盖。

// SyncModelPricingRequest 可选携带一次性同步 URL（同时保存为默认来源）。
type SyncModelPricingRequest struct {
	URL string `json:"url"`
}

// SyncModelPricing 从 JSON URL 同步定价（synced 覆盖，不动 custom）。

// 字段即准：直接传设置页 URL 字段的当前值（空 → 用内置默认并清空存储来源）。
