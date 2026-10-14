package admin

import (
	"testing"

	"github.com/codex2api/auth"
)

// resetAntigravityOAuthSettingsEnv 清空环境变量与包级配置态，避免用例间串扰。
func resetAntigravityOAuthSettingsEnv(t *testing.T) {
	t.Helper()
	t.Setenv("ANTIGRAVITY_OAUTH_CLIENTS", "")
	t.Setenv("ANTIGRAVITY_OAUTH_CLIENT_KEY", "")
	auth.SetConfiguredAntigravityOAuth(auth.AntigravityOAuthSettings{})
	t.Cleanup(func() { auth.SetConfiguredAntigravityOAuth(auth.AntigravityOAuthSettings{}) })
}

// 编辑同 key 条目但不重填 secret：client_id 更新、secret 沿用已保存值。

// 热更新可直接支撑授权 URL 构建（原 env 独占路径）。

// 新增条目缺 secret：既没有已保存值可沿用，也不能落库空 secret。
