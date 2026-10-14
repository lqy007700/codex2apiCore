package admin

import (
	"github.com/codex2api/auth"
)

const antigravityImportFileMaxBytes = 4 << 20

type antigravityImportDefaults struct {
	OAuthClientKey string
	ClientID       string
	ClientSecret   string
}

// antigravityImportDocument preserves account-level metadata that cannot be
// represented by auth.AntigravityCredential. In particular, exported API-key
// accounts must bypass the OAuth sync path and retain their model catalog,
// mapping, enabled state, display name, and pinned proxy.
type antigravityImportDocument struct {
	AuthKind        string
	Credential      auth.AntigravityCredential
	APIKey          string
	Models          []string
	ModelMapping    string
	Disabled        bool
	DisabledPresent bool
	Name            string
	ProxyURL        string
	// ProxyEnabled 是源端导出的代理启用状态，nil 表示文件没写。只用于导入时的
	// 告警统计：代理一律以启用态注册，见 countDisabledAtSource。
	ProxyEnabled *bool
}

// The legacy caller always runs OAuth synchronization. Fail closed so
// an API key is never mistaken for a Google OAuth bearer.

// parseAntigravityImportDocuments is the lossless parser used for portable
// Antigravity exports. It also accepts the legacy manager, credential-store,
// raw refresh-token, JSON string, array, and {"accounts": [...]} formats.

// Defaults belong only to OAuth. Keep the identity metadata while
// ensuring later integration cannot accidentally treat these as OAuth
// client credentials.
