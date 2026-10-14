package proxy

const (
	// ClaudeReleasesLatestURL 是 anthropics/claude-code 最新正式 release 的 GitHub API 端点。
	ClaudeReleasesLatestURL = "https://api.github.com/repos/anthropics/claude-code/releases/latest"
	// ClaudeNpmDistTagsURL 是 npm 上 @anthropic-ai/claude-code 的 dist-tags 端点（GitHub 失败时回退）。
	ClaudeNpmDistTagsURL = "https://registry.npmjs.org/-/package/@anthropic-ai/claude-code/dist-tags"
)

// 测试接缝；生产代码不要赋值。
var (
	claudeReleasesLatestURLForTest = ""
	claudeNpmDistTagsURLForTest    = ""
)

// SetClaudeVersionSourceURLsForTest 覆盖 GitHub/npm 版本源端点。仅供测试使用；
// 生产代码不要调用。传空串恢复默认端点。

// ClaudeCLIVersionSyncDisabled 报告是否通过 CLAUDE_DISABLE_CLI_VERSION_SYNC 关闭了联网同步。
// 关闭后仍会在启动时用当前生效版本做一次本地指纹回写（不联网）；管理端「立即同步」不受影响。

// ClaudeCLIVersionSyncResult 是一次同步的结果投影。
type ClaudeCLIVersionSyncResult struct {
	FetchedVersion    string `json:"fetched_version"`
	EffectiveVersion  string `json:"effective_version"`
	BuiltinVersion    string `json:"builtin_version"`
	Updated           bool   `json:"updated"`
	AccountsRefreshed int    `json:"accounts_refreshed"`
}

// extractClaudeCLIVersion 接受 "2.1.258" / "v2.1.258"，丢弃预发布后缀；非法返回空串。

// FetchLatestClaudeCLIVersion 先查 GitHub releases/latest，失败再查 npm dist-tags。

// 必须返回接口 nil，而不是 nil 指针

// SyncClaudeCLIVersion 拉取最新版本，高于当前生效版本时持久化并发布，随后回写所有账号指纹。

// StartClaudeCLIVersionSync 启动时先用生效版本做一次本地指纹回写（不联网），
// 然后按 ClaudeConfig 的开关与间隔定时联网同步。
// 本地回写在后台任务内执行，即使 CLAUDE_DISABLE_CLI_VERSION_SYNC 关闭了联网
// 同步也照常运行；只有联网的 runOnce 与定时循环受该开关约束。
