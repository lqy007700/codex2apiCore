package admin

import (
	"context"

	"log"

	"github.com/codex2api/auth"
	"github.com/codex2api/database"
)

// Claude / Antigravity 渠道的连通性测试配置：账号页"测试连接"按钮与批量测试
// 默认使用的探测模型和测活内容。全局 test_model / test_content 是 Codex 语义，
// 这两个渠道的模型目录完全不同，需要各自的默认值；留空时模型按账号目录自动选，
// 内容沿用全局测活内容。

const (
	channelTestChannelAntigravity = "antigravity"
	channelTestChannelClaude      = "claude"
)

type channelTestSettingsResponse struct {
	Antigravity database.ChannelTestSettings `json:"antigravity"`
	Claude      database.ChannelTestSettings `json:"claude"`
	// 渠道留空内容时实际生效的全局测活内容，前端用作占位提示。
	DefaultTestContent string `json:"default_test_content"`
	// 渠道并发为 0 时实际生效的全局批量测试并发。
	DefaultTestConcurrency int `json:"default_test_concurrency"`
	// 设置页下拉可选的模型：取自当前号池里该渠道账号目录的并集（Claude 常是带日期的
	// 具体 ID，Antigravity 是对外发布的固定档位），没有账号时回落到默认集。
	ModelChoices map[string][]string `json:"model_choices"`
}

// channelTestModelChoices 汇总各渠道账号目录里可作为测试模型的候选。

// channelTestConfig 返回当前生效的渠道测试配置。首次访问从数据库加载并缓存，
// 之后由 PUT 同步刷新；无数据库（单测 / 纯内存）时返回零值。
func (h *Handler) channelTestConfig(ctx context.Context) database.ChannelTestConfig {
	if h == nil {
		return database.ChannelTestConfig{}
	}
	if cached := h.channelTestCfg.Load(); cached != nil {
		return *cached
	}
	if h.db == nil {
		return database.ChannelTestConfig{}
	}
	cfg, err := h.db.LoadChannelTestConfig(ctx)
	if err != nil {
		log.Printf("读取渠道连通性测试设置失败，按默认处理: %v", err)
		return database.ChannelTestConfig{}
	}
	h.channelTestCfg.Store(&cfg)
	return cfg
}

// channelTestSettingsForAccount 取账号所属渠道的测试配置；Codex/Grok 等渠道返回零值。
func (h *Handler) channelTestSettingsForAccount(ctx context.Context, account *auth.Account) database.ChannelTestSettings {
	if account == nil {
		return database.ChannelTestSettings{}
	}
	cfg := h.channelTestConfig(ctx)
	switch {
	case account.IsAntigravityAPI():
		return cfg.Antigravity
	case account.IsClaudeOAuth():
		return cfg.Claude
	}
	return database.ChannelTestSettings{}
}

// connectionTestContentForAccount 返回该账号测连应发送的用户输入（已做多行抽取与
// 变量展开）：渠道自定义内容优先，留空沿用全局测活内容。
func (h *Handler) connectionTestContentForAccount(ctx context.Context, account *auth.Account) string {
	content := ""
	if settings := h.channelTestSettingsForAccount(ctx, account); settings.TestContent != "" {
		content = settings.TestContent
	} else {
		content = auth.DefaultTestContent
		if h != nil && h.store != nil {
			content = h.store.GetTestContent()
		}
	}
	return auth.RenderTestContent(content)
}

// GetChannelTestSettings 返回 Claude / Antigravity 的连通性测试配置。
// GET /api/admin/settings/channel-tests

// UpdateChannelTestSettings 保存渠道连通性测试配置。请求体只带要改的渠道即可，
// 未出现的渠道保持原值；模型只做基本校验（不能是生图模型），是否在账号目录里
// 由测连时按账号判定。
// PUT /api/admin/settings/channel-tests  {"antigravity":{"test_model":"...","test_content":"..."}}

// normalizeChannelTestSettings 校验单渠道配置：模型不能是生图模型，Claude 模型必须
// 以 claude- 开头；内容为空表示沿用全局，否则受全局同一长度上限约束。

// batchTestConcurrency 决定一次批量测试的并发：整批账号同属 Claude 或 Antigravity
// 且该渠道配置了并发时用渠道值，否则（混合批次、Codex/Grok、未配置）沿用全局。
func (h *Handler) batchTestConcurrency(ctx context.Context, accounts []*auth.Account) int {
	global := 0
	if h != nil && h.store != nil {
		global = h.store.GetTestConcurrency()
	}
	if global <= 0 {
		global = 1
	}
	if len(accounts) == 0 {
		return global
	}
	channel := ""
	for _, account := range accounts {
		var current string
		switch {
		case account == nil:
			return global
		case account.IsAntigravityAPI():
			current = channelTestChannelAntigravity
		case account.IsClaudeOAuth():
			current = channelTestChannelClaude
		default:
			return global
		}
		if channel == "" {
			channel = current
		} else if channel != current {
			return global
		}
	}
	configured := 0
	cfg := h.channelTestConfig(ctx)
	switch channel {
	case channelTestChannelAntigravity:
		configured = cfg.Antigravity.TestConcurrency
	case channelTestChannelClaude:
		configured = cfg.Claude.TestConcurrency
	}
	if configured > 0 {
		return configured
	}
	return global
}

type channelTestSettingsError struct {
	channel string
	message string
}

func (e *channelTestSettingsError) Error() string {
	return e.channel + ": " + e.message
}
