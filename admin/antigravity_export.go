package admin

import (
	"regexp"

	"github.com/gin-gonic/gin"
)

// antigravityExportEntry is a portable, secret-bearing credential document.
// It is intentionally returned only by the admin-only download endpoint.
type antigravityExportEntry struct {
	Type           string   `json:"type"`
	Version        int      `json:"version"`
	AuthKind       string   `json:"auth_kind"`
	Email          string   `json:"email,omitempty"`
	Name           string   `json:"name,omitempty"`
	AccessToken    string   `json:"access_token,omitempty"`
	RefreshToken   string   `json:"refresh_token,omitempty"`
	IDToken        string   `json:"id_token,omitempty"`
	ProjectID      string   `json:"project_id,omitempty"`
	OAuthClientKey string   `json:"oauth_client_key,omitempty"`
	ClientID       string   `json:"client_id,omitempty"`
	ClientSecret   string   `json:"client_secret,omitempty"`
	Scope          string   `json:"scope,omitempty"`
	ExpiresAt      string   `json:"expires_at,omitempty"`
	APIKey         string   `json:"api_key,omitempty"`
	Models         []string `json:"models,omitempty"`
	ModelMapping   string   `json:"model_mapping,omitempty"`
	ProxyURL       string   `json:"proxy_url,omitempty"`
	ProxyLabel     string   `json:"proxy_label,omitempty"`
	ProxyEnabled   *bool    `json:"proxy_enabled,omitempty"`
	Disabled       bool     `json:"disabled"`

	exportFileName string
}

// antigravityAccountRowToExportEntry converts an account row into an export
// entry. Unlike the CPA and Grok exports, proxies defaults to enabled for this
// channel: it has always emitted proxy_url, and dropping it would silently
// break the round-trip of existing backups.

var antigravityExportUnsafeFileChars = regexp.MustCompile(`[^A-Za-z0-9@._-]`)

// writeSecretResponseHeaders marks a response as secret-bearing so it is never
// cached or content-sniffed. Split from writeSecretDownloadHeaders because the
// CPA export endpoints return an inline JSON array the frontend names itself —
// they need the cache guards without the download disposition.
func writeSecretResponseHeaders(c *gin.Context) {
	c.Header("Cache-Control", "no-store, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("X-Content-Type-Options", "nosniff")
}

// ExportAntigravityAccounts downloads Antigravity credentials. format=json (the
// default) returns one JSON file: a single credential document for one account
// or a JSON array for several, both accepted by the importer as-is. format=zip
// keeps the one-file-per-account archive. The response contains live secrets
// and must never be cached, embedded, or logged.
