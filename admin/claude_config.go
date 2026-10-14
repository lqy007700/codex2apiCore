package admin

import (
	"github.com/codex2api/auth"
	"github.com/codex2api/proxy"
)

// claudeGlobalConfigDTO 是 ClaudeCode 全局配置的读写载体(系统设置里的独立模块)。
// 全体 Claude 账号默认遵守;个体账号可在「编辑账号」里覆盖。
type claudeGlobalConfigDTO struct {
	FingerprintMode    string `json:"fingerprint_mode"`     // preserve / force(空=preserve)
	DefaultTimezone    string `json:"default_timezone"`     // 导入 Claude 账号的默认 IANA 时区
	SessionWindowLimit int64  `json:"session_window_limit"` // 默认并发会话窗口数(0=跟随全局)
	auth.ClaudeClientPolicy
	auth.ClaudeSecurityConfig
	CLIVersionSyncEnabled       *bool `json:"cli_version_sync_enabled"`
	CLIVersionSyncIntervalHours int   `json:"cli_version_sync_interval_hours"`
	// FirstTokenTimeoutSeconds：Claude 路径首字超时秒数（缺失=默认 120，0=跟随全局）。
	FirstTokenTimeoutSeconds *int `json:"first_token_timeout_seconds"`
	// StreamKeepaliveEnabled：Claude 流式首字前是否发 SSE 保活注释（缺失=开启）。
	StreamKeepaliveEnabled *bool `json:"stream_keepalive_enabled"`
	// 以下三项只读；PUT 忽略。
	SyncedCLIVersion    string `json:"synced_cli_version"`
	BuiltinCLIVersion   string `json:"builtin_cli_version"`
	EffectiveCLIVersion string `json:"effective_cli_version"`
}

// GetClaudeConfig 返回当前 ClaudeCode 全局配置(取自运行时 Store 访问器)。

// UpdateClaudeConfig 校验并持久化 ClaudeCode 全局配置,同时热更新运行时 Store。

// 热更新运行时 Store,无需重启即生效。

// boolPtr 返回指向给定 bool 值的指针，便于构造「显式布尔字段」的 JSON DTO。

// claudeIntPtr 返回指向给定 int 值的指针，用于「缺失与显式 0 有别」的 JSON DTO 字段。

// claudeCLIVersionSyncResponse 在同步结果之上附加一个可选的 warning 字段：
// 抓取+持久化成功、但指纹回写部分失败时，仍以 200 响应并携带 warning，
// 而不是把整次同步判为失败。
type claudeCLIVersionSyncResponse struct {
	*proxy.ClaudeCLIVersionSyncResult
	Warning string `json:"warning,omitempty"`
}

// SyncClaudeCLIVersion 供设置页「立即同步」调用：拉取最新 Claude Code CLI 版本并回写账号指纹。
// proxy.SyncClaudeCLIVersion 的 err 可能只是抓取成功、持久化成功之后的指纹回写部分失败，
// 因此只在抓取阶段就失败（没有 FetchedVersion）时才判 502；否则仍按 200 返回结果，
// 并把该 err 作为 warning 字段透出，方便前端提示但不阻断已生效的版本同步。
