package admin

import (
	"github.com/codex2api/proxy"
)

// Antigravity 渠道设置：
//   - 模型重定向：下游请求不带思考强度后缀的逻辑模型（gemini-3.8-flash）时，自动按
//     配置落到某个固定档位（gemini-3.8-flash-high）。默认只补位缺失的
//     reasoning.effort，开启覆盖后连显式 effort 也按重定向走。
//   - 思考内容下发：开启后 OAuth 账号的 Gemini thought 片段作为 reasoning 输出下发。

type antigravitySettingsResponse struct {
	ModelRedirects          map[string]string                 `json:"model_redirects"`
	RedirectOverridesEffort bool                              `json:"redirect_overrides_effort"`
	ExposeThoughts          bool                              `json:"expose_thoughts"`
	Choices                 []proxy.AntigravityRedirectChoice `json:"choices"`
}

// GetAntigravitySettings 返回当前 Antigravity 渠道设置与可选的重定向档位。
// GET /api/admin/settings/antigravity

// UpdateAntigravitySettings 保存 Antigravity 渠道设置。只更新请求里出现的字段；
// model_redirects 整体替换，值为空的条目表示取消该模型的重定向。
// PUT /api/admin/settings/antigravity
// {"model_redirects":{"gemini-3.8-flash":"gemini-3.8-flash-high"},"redirect_overrides_effort":false,"expose_thoughts":true}
