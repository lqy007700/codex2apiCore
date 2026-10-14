package admin

// Claude OAuth credential export/import primitives.
//
// Claude credentials are intentionally kept out of the generic Codex export
// endpoint.  This file defines a provider-specific, versioned document that
// can be moved between Codex2API installations without exposing arbitrary
// request headers or instance-local group IDs.

import (
	"fmt"

	"net/http"

	"regexp"

	"strings"

	"github.com/codex2api/auth"
	"github.com/codex2api/database"
)

const (
	claudeCredentialExportVersion    = 1
	claudeCredentialExportMaxBytes   = 8 << 20
	claudeCredentialImportMaxEntries = 500
)

// Account creation can perform a bounded upstream model discovery when an
// old token document has no models. Scale the request budget without making
// a large bundle unbounded.

// claudeGroupRef is portable across installations.  Numeric group IDs are
// deliberately not exported because IDs are instance-local and could bind an
// imported account to an unrelated production group.
type claudeGroupRef struct {
	Name    string `json:"name"`
	Channel string `json:"channel"`
}

// claudeExportEntry is the stable, secret-bearing Claude credential document.
// Operational counters/cooldowns/locks are intentionally omitted; they are
// local runtime state and must be re-established by the destination instance.
type claudeExportEntry struct {
	Type                  string            `json:"type"`
	Version               int               `json:"version"`
	AuthKind              string            `json:"auth_kind"`
	BaseURL               string            `json:"base_url,omitempty"`
	Email                 string            `json:"email,omitempty"`
	Name                  string            `json:"name,omitempty"`
	AccessToken           string            `json:"access_token"`
	RefreshToken          string            `json:"refresh_token"`
	AccountID             string            `json:"account_id,omitempty"`
	ExpiresAt             string            `json:"expires_at,omitempty"`
	PlanType              string            `json:"plan_type,omitempty"`
	Models                []string          `json:"models,omitempty"`
	ProxyURL              string            `json:"proxy_url,omitempty"`
	Timezone              string            `json:"timezone,omitempty"`
	ClaudeFingerprintMode string            `json:"claude_fingerprint_mode,omitempty"`
	FingerprintHeaders    map[string]string `json:"fingerprint_headers,omitempty"`
	// CustomHeaders are the operator-configured outbound headers of an API Key
	// account (never exported for OAuth/Setup Token, whose custom_headers hold the
	// generated fingerprint exposed via FingerprintHeaders instead).
	CustomHeaders map[string]string `json:"custom_headers,omitempty"`
	Tags          []string          `json:"tags,omitempty"`
	GroupRefs     []claudeGroupRef  `json:"group_refs,omitempty"`
	Enabled       bool              `json:"enabled"`

	// exportFileName is only used as a ZIP member name and never serialized.
	exportFileName string `json:"-"`
}

// claudeImportDocument is the validated internal representation accepted by
// the Claude import endpoint.  Enabled is a pointer so legacy documents that
// omit it keep the historical default (enabled=true).
type claudeImportDocument struct {
	Type                  string
	Version               int
	AuthKind              string
	BaseURL               string
	Email                 string
	Name                  string
	AccessToken           string
	RefreshToken          string
	AccountID             string
	ExpiresAt             string
	PlanType              string
	Models                []string
	ProxyURL              string
	UseProxyPool          bool
	Timezone              string
	ClaudeFingerprintMode string
	FingerprintHeaders    map[string]string
	// CustomHeaders is only populated for api_key documents (operator headers);
	// OAuth/Setup Token documents route their identity headers through
	// FingerprintHeaders instead.
	CustomHeaders map[string]string
	Tags          []string
	GroupRefs     []claudeGroupRef
	Enabled       *bool
}

// claudeAccountImportOptions carries metadata that is not part of
// auth.ClaudeTokenData.  It is consumed by the common account creation path.
type claudeAccountImportOptions struct {
	// AuthKind 是凭据形态(oauth / setup_token / api_key);空=按是否有 RT 推断。
	AuthKind string
	BaseURL  string
	Models   []string
	PlanType string
	// FingerprintMode 对 OAuth/Setup Token 是指纹替换模式;对 api_key 是可选的
	// Claude Code 客户端身份仿真开关(空=透传,见 auth.ApplyClaudeAPIKeyIdentityHeaders)。
	FingerprintMode    string
	FingerprintHeaders map[string]string
	// CustomHeaders 仅 api_key 使用:账号级自定义出站请求头(保留头在
	// normalizeClaudeAPIKeyCustomHeaders 中拒绝)。
	CustomHeaders    map[string]string
	Tags             []string
	GroupRefs        []claudeGroupRef
	ResolvedGroupIDs []int64
	SkipModelFetch   bool
	Enabled          *bool
}

// normalizeClaudeAPIKeyCustomHeaders validates operator headers for a Claude
// API Key account: the generic custom_headers rules plus rejection of the
// gateway-owned reserved names (authentication, body framing, Accept).
func normalizeClaudeAPIKeyCustomHeaders(headers map[string]string) (map[string]string, error) {
	normalized, err := normalizeCustomHeaders(headers)
	if err != nil {
		return nil, err
	}
	for name := range normalized {
		if auth.IsClaudeAPIKeyReservedHeader(name) {
			return nil, fmt.Errorf("custom_headers 不能覆盖网关保留的请求头: %s", name)
		}
	}
	return normalized, nil
}

type claudeImportResultItem struct {
	ID       int64    `json:"id,omitempty"`
	Email    string   `json:"email,omitempty"`
	OK       bool     `json:"ok"`
	Error    string   `json:"error,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	status   int      `json:"-"`
}

type claudeAccountCreateError struct {
	Status  int
	Message string
}

type claudeAccountCreateResult struct {
	ID       int64
	Email    string
	Warnings []string
}

func (e *claudeAccountCreateError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

var claudeExportUnsafeFileChars = regexp.MustCompile(`[^A-Za-z0-9@._-]`)

// normalizeClaudeFingerprintHeaders accepts Claude Code identity headers and
// the account's device-ID metadata stored alongside them. The device ID is
// deliberately not a ClaudeIdentityHeaderNames entry and is never sent as an
// HTTP header. Authorization, Cookie, API keys, and arbitrary custom headers
// must never cross an export boundary.

// prepareClaudeTimezoneCredentialUpdate keeps only approved tracing headers
// while replacing the generated Claude Code identity headers. Timezone is an
// account-level fingerprint boundary, so the credential and runtime header
// snapshot must be updated together instead of only changing a display field.

func prepareClaudeTimezoneCredentialUpdateWithHeaders(row *database.AccountRow, timezone string, updates map[string]interface{}, requestedHeaders map[string]string) (bool, error) {
	if row == nil || updates == nil {
		return false, nil
	}
	if !strings.EqualFold(strings.TrimSpace(row.Platform), "anthropic") &&
		!strings.EqualFold(strings.TrimSpace(row.GetCredential("upstream_type")), auth.UpstreamClaude) {
		return false, nil
	}
	if claudeAuthKindForRow(row, true) == auth.ClaudeAuthKindAPIKey {
		return false, nil
	}
	timezone = strings.TrimSpace(timezone)
	if err := validateAccountTimezone(timezone); err != nil {
		return false, err
	}
	identity := make(map[string]struct{}, len(auth.ClaudeIdentityHeaderNames))
	for _, name := range auth.ClaudeIdentityHeaderNames {
		identity[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	baseHeaders := row.GetCredentialStringMap("custom_headers")
	if requestedHeaders != nil {
		baseHeaders = requestedHeaders
	}
	// Keys are canonicalized up front: the generated fingerprint says
	// "X-Stainless-OS" while previously persisted headers come back as
	// "X-Stainless-Os", and normalizeCustomHeaders treats a case-only clash
	// with different values as an error. Without this a same-timezone save
	// failed whenever the freshly rolled OS differed from the stored one.
	// 先统一成规范大小写：指纹生成的是 X-Stainless-OS，落库后读回是 X-Stainless-Os，
	// 否则同时区保存时随机到不同 OS 就会触发"大小写重复且值冲突"。
	merged := make(map[string]string)
	keepIdentity := requestedHeaders == nil && strings.EqualFold(strings.TrimSpace(row.GetCredential("timezone")), timezone)
	for name, value := range auth.GenerateClaudeFingerprint(timezone).Headers() {
		merged[http.CanonicalHeaderKey(strings.TrimSpace(name))] = value
	}
	// An explicit header patch may omit account metadata. Keep the stored
	// device ID unless the operator also supplies a replacement for it.
	for name, value := range row.GetCredentialStringMap("custom_headers") {
		if strings.EqualFold(strings.TrimSpace(name), auth.ClaudeDeviceIDCredentialKey) {
			merged[http.CanonicalHeaderKey(auth.ClaudeDeviceIDCredentialKey)] = value
		}
	}
	for name, value := range baseHeaders {
		lowerName := strings.ToLower(strings.TrimSpace(name))
		canonicalName := http.CanonicalHeaderKey(strings.TrimSpace(name))
		if _, isIdentity := identity[lowerName]; isIdentity {
			// Keep a complete existing fingerprint stable when the operator
			// saves the same timezone again; a timezone change (or explicit
			// header patch) intentionally rotates the identity snapshot.
			if keepIdentity {
				merged[canonicalName] = value
			}
			continue
		}
		// Rotating the platform fingerprint must not rotate or erase the
		// account's persistent device identity.
		if lowerName == auth.ClaudeDeviceIDCredentialKey || isClaudeSafeOperationalHeader(name) {
			merged[canonicalName] = value
		}
	}
	normalized, err := normalizeCustomHeaders(merged)
	if err != nil {
		return false, err
	}
	updates["custom_headers"] = normalized
	updates["timezone"] = timezone
	return true, nil
}

// Only a small, explicit set of tracing headers may survive a Claude
// fingerprint rebuild.  This prevents historical Authorization/Cookie/API-key
// values (or arbitrary operator headers) from being copied into a credential
// update merely because they happened to be present in custom_headers.
func isClaudeSafeOperationalHeader(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "traceparent", "tracestate", "x-request-id", "x-client-request-id", "x-correlation-id", "x-trace-id":
		return true
	default:
		return false
	}
}

func claudeExportFingerprintHeaders(headers map[string]string) map[string]string {
	allowed := make(map[string]struct{}, len(auth.ClaudeIdentityHeaderNames))
	for _, name := range auth.ClaudeIdentityHeaderNames {
		allowed[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	allowed[auth.ClaudeDeviceIDCredentialKey] = struct{}{}
	out := make(map[string]string)
	for name, value := range headers {
		if _, ok := allowed[strings.ToLower(strings.TrimSpace(name))]; !ok {
			continue
		}
		if strings.TrimSpace(value) == "" {
			continue
		}
		out[http.CanonicalHeaderKey(strings.TrimSpace(name))] = strings.TrimSpace(value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// 一个没有 RT 的「OAuth」行只剩即将过期的 AT,导出到别处也无法续期。

// API Key accounts keep claude_fingerprint_mode (client identity emulation)
// and carry their operator headers as custom_headers; reserved names are
// dropped defensively so an export can always be re-imported.

// resolveClaudeGroupRefs maps portable references to this instance's IDs. A
// non-Claude or missing reference is reported in missing rather than guessed.

// claudeImportWire is intentionally permissive about unknown future fields so
// a newer exporter can still be consumed by an older gateway. Validation below
// rejects unsupported provider/auth shapes and unsafe values.
type claudeImportWire struct {
	Type                  string                      `json:"type"`
	UpstreamType          string                      `json:"upstream_type"`
	Version               int                         `json:"version"`
	AuthKind              string                      `json:"auth_kind"`
	BaseURL               string                      `json:"base_url"`
	Email                 string                      `json:"email"`
	Name                  string                      `json:"name"`
	AccessToken           string                      `json:"access_token"`
	RefreshToken          string                      `json:"refresh_token"`
	AccountID             string                      `json:"account_id"`
	ExpiresAt             string                      `json:"expires_at"`
	PlanType              string                      `json:"plan_type"`
	Models                []string                    `json:"models"`
	ProxyURL              string                      `json:"proxy_url"`
	UseProxyPool          bool                        `json:"use_proxy_pool"`
	Timezone              string                      `json:"timezone"`
	ClaudeFingerprintMode string                      `json:"claude_fingerprint_mode"`
	FingerprintHeaders    map[string]string           `json:"fingerprint_headers"`
	CustomHeaders         map[string]string           `json:"custom_headers"`
	Tags                  []string                    `json:"tags"`
	GroupRefs             []claudeGroupRef            `json:"group_refs"`
	Groups                []claudeGroupRef            `json:"groups"`
	Enabled               *bool                       `json:"enabled"`
	Credentials           *claudeImportCredentialWire `json:"credentials"`
	APIKey                string                      `json:"api_key"`
}

type claudeImportCredentialWire struct {
	Type                  string            `json:"type"`
	UpstreamType          string            `json:"upstream_type"`
	Version               int               `json:"version"`
	AuthKind              string            `json:"auth_kind"`
	BaseURL               string            `json:"base_url,omitempty"`
	Email                 string            `json:"email"`
	Name                  string            `json:"name"`
	AccessToken           string            `json:"access_token"`
	RefreshToken          string            `json:"refresh_token"`
	AccountID             string            `json:"account_id"`
	ExpiresAt             string            `json:"expires_at"`
	PlanType              string            `json:"plan_type"`
	Models                []string          `json:"models"`
	ProxyURL              string            `json:"proxy_url"`
	UseProxyPool          bool              `json:"use_proxy_pool"`
	Timezone              string            `json:"timezone"`
	ClaudeFingerprintMode string            `json:"claude_fingerprint_mode"`
	FingerprintHeaders    map[string]string `json:"fingerprint_headers"`
	CustomHeaders         map[string]string `json:"custom_headers"`
	Tags                  []string          `json:"tags"`
	GroupRefs             []claudeGroupRef  `json:"group_refs"`
	Groups                []claudeGroupRef  `json:"groups"`
	Enabled               *bool             `json:"enabled"`
	APIKey                string            `json:"api_key"`
}

// 形态推断:未声明 auth_kind 时,有 RT 视为 oauth(AT 可缺省,入库前用 RT 刷出);
// 无 RT 仅当 AT 形如官方 Setup Token(sk-ant-oat01-)才视为 setup_token,否则拒绝——
// 避免把一个 1 小时就过期的裸 OAuth AT 误当长效令牌。

// API Key documents carry arbitrary operator headers (custom_headers; a
// legacy fingerprint_headers key is accepted as an alias) instead of a
// restricted identity fingerprint.

// claudeExportAPIKeyCustomHeaders trims an API Key account's stored
// custom_headers for export/import round trips: empty names/values and
// gateway-reserved headers are removed rather than failing the whole export.

// ExportClaudeAccounts downloads one JSON credential document or a ZIP with
// one document per account. The endpoint is admin-authenticated by the route
// group and deliberately uses secret download headers.

// Record only aggregate, non-secret audit metadata. Never include an email,
// account ID, token, proxy URL, or serialized response body.
