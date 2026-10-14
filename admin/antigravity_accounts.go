package admin

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"strconv"
	"strings"

	"time"

	"github.com/codex2api/auth"
	"github.com/codex2api/database"
	"github.com/codex2api/proxy"

	"github.com/gin-gonic/gin"
)

const (
	antigravityImportMaxAccounts        = 500
	antigravitySyncConcurrency          = 4
	antigravityImportedVerifiedEmailKey = "antigravity_import_verified_email"
	antigravityAccessDeniedError        = "Antigravity access denied by Google"
	antigravityNoUsableEgressError      = "Antigravity 没有可用代理出口"
	// antigravityManualRefreshTimeout 是管理端单号同步(换票+项目+权限+配额)的上限。
	antigravityManualRefreshTimeout = 90 * time.Second
)

type antigravityImportRequest struct {
	Files          []string        `json:"files"`
	RefreshTokens  []string        `json:"refresh_tokens"`
	ProxyURL       string          `json:"proxy_url"`
	OAuthClientKey string          `json:"oauth_client_key"`
	ClientID       string          `json:"client_id"`
	ClientSecret   string          `json:"client_secret"`
	GroupIDs       json.RawMessage `json:"group_ids"`
	// ImportProxy 决定文件里的 proxy_url 是否注册进代理表并同步代理池。
	//
	// 该渠道的导入一直会采用文件内代理（prepareAntigravityImport 就是这么写的），
	// 只是从不入表：账号绑的是一个代理池不认识的 URL，管理页看不见、也进不了轮转。
	// 开关只控制"是否入表"，关闭时维持既有行为，不改变代理的取用优先级。
	ImportProxy bool `json:"import_proxy"`
}

type addAntigravityAccountRequest struct {
	Name           string          `json:"name"`
	AuthKind       string          `json:"auth_kind"`
	AuthJSON       string          `json:"auth_json"`
	APIKey         string          `json:"api_key"`
	Models         []string        `json:"models"`
	ModelMapping   string          `json:"model_mapping"`
	ProxyURL       string          `json:"proxy_url"`
	OAuthClientKey string          `json:"oauth_client_key"`
	ClientID       string          `json:"client_id"`
	ClientSecret   string          `json:"client_secret"`
	GroupIDs       json.RawMessage `json:"group_ids"`
}

type updateAntigravityAccountRequest struct {
	Name         *string         `json:"name"`
	AuthJSON     string          `json:"auth_json"`
	APIKey       *string         `json:"api_key"`
	Models       []string        `json:"models"`
	ModelMapping *string         `json:"model_mapping"`
	ProxyURL     *string         `json:"proxy_url"`
	GroupIDs     json.RawMessage `json:"group_ids"`
}

// antigravityModelsForPersistence keeps the durable account catalog in the
// provider's raw/wire namespace. Public IDs are accepted by admin forms, but
// must be resolved before the scheduler observes the updated credential.

func antigravityPublishedModels(rawModels []string) []string {
	return proxy.AntigravityPublishedModelIDs(auth.NormalizeAccountModels(rawModels))
}

func antigravityPublishedModelsOrDefault(rawModels []string) []string {
	if len(rawModels) == 0 {
		rawModels = auth.AntigravityDefaultModelIDs()
	}
	return antigravityPublishedModels(rawModels)
}

// antigravityPublishedModelForObservation maps a persisted physical model
// observation back to one of the logical models actually advertised by the
// account. Unlike catalog publication, a single observation naturally names
// only one backing and therefore must not be passed to PublishedModelIDs on
// its own (logical Gemini models require their complete backing set there).

// antigravityPublishedQuota clones a persisted raw quota snapshot into the
// public model namespace. A logical model may be backed by several provider
// wire models, but the admin response exposes exactly one quota entry for it.
func antigravityPublishedQuota(raw auth.AntigravityQuotaSnapshot) auth.AntigravityQuotaSnapshot {
	projected := raw
	projected.CatalogModelIDs = antigravityPublishedModels(auth.AntigravityDiscoveredModels(raw))
	projected.InternalModelIDs = nil
	projected.Models = []auth.AntigravityModelQuota{}
	// Forwarding rules are provider-wire implementation details and are not an
	// admin-facing model contract.
	projected.ModelForwardingRules = nil

	rawModels := make([]string, 0, len(raw.Models))
	byWireID := make(map[string]auth.AntigravityModelQuota, len(raw.Models))
	for _, model := range raw.Models {
		wireID := strings.ToLower(strings.TrimSpace(model.ModelID))
		if wireID == "" {
			continue
		}
		rawModels = append(rawModels, wireID)
		if _, exists := byWireID[wireID]; !exists {
			byWireID[wireID] = model
		}
	}
	for _, publicID := range antigravityPublishedModels(rawModels) {
		// Prefer the logical model's default effort backing so the quota shown in
		// the account UI matches an effort-less request. If that backing was not
		// returned by the provider, fall back to any available backing for the
		// same logical model instead of dropping it from the quota projection.
		wireIDs := proxy.AntigravityWireModelIDs(publicID)
		if defaultWireID, ok := proxy.AntigravityWireModelID(publicID); ok {
			wireIDs = append([]string{defaultWireID}, wireIDs...)
		}
		// Discovered upstream models are published under their actual ID, so the
		// public ID is also the wire ID.
		if len(wireIDs) == 0 {
			wireIDs = []string{publicID}
		}
		var (
			model auth.AntigravityModelQuota
			found bool
		)
		for _, wireID := range wireIDs {
			model, found = byWireID[strings.ToLower(strings.TrimSpace(wireID))]
			if found {
				break
			}
		}
		if !found {
			continue
		}
		model.ModelID = publicID
		model.DisplayName = publicID
		projected.Models = append(projected.Models, model)
	}
	return projected
}

func antigravityPublishedQuotaJSON(raw string) json.RawMessage {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var quota auth.AntigravityQuotaSnapshot
	if json.Unmarshal([]byte(raw), &quota) != nil {
		return nil
	}
	encoded, err := json.Marshal(antigravityPublishedQuota(quota))
	if err != nil {
		return nil
	}
	return json.RawMessage(encoded)
}

type antigravityImportItem struct {
	Index    int    `json:"index"`
	SubIndex int    `json:"sub_index,omitempty"`
	ID       int64  `json:"id,omitempty"`
	Email    string `json:"email,omitempty"`
	OK       bool   `json:"ok"`
	Synced   bool   `json:"synced"`
	Warning  string `json:"warning,omitempty"`
	Error    string `json:"error,omitempty"`
}

type antigravityRefreshRequest struct {
	IDs []int64 `json:"ids"`
}

type antigravityRefreshItem struct {
	ID      int64  `json:"id"`
	Email   string `json:"email,omitempty"`
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	Warning string `json:"warning,omitempty"`
	Error   string `json:"error,omitempty"`
}

type antigravitySyncJob struct {
	Index       int // compact worker/result slot
	SourceIndex int // original input file/token index (1-based)
	SubIndex    int // credential ordinal within the original input (1-based)
	Credential  auth.AntigravityCredential
	ProxyURL    string
	AccountID   int64
}

type antigravitySyncJobResult struct {
	Job    antigravitySyncJob
	Result auth.AntigravitySyncResult
	Err    error
}

type parsedAntigravityCredential struct {
	Document    antigravityImportDocument
	Credential  auth.AntigravityCredential
	SourceIndex int
	SubIndex    int
}

type preparedAntigravityImport struct {
	Parsed            parsedAntigravityCredential
	Document          antigravityImportDocument
	Name              string
	ProxyURL          string
	EffectiveProxyURL string
	Models            []string
	ModelsPresent     bool
	ModelMapping      string
	ValidationError   string
}

// parseAntigravityImportInputs keeps the response index tied to the original
// input item. A single export may contain multiple accounts, so the optional
// sub-index disambiguates those credentials without renumbering later files
// when an earlier file fails to parse.

func antigravityRowFamilyID(row *database.AccountRow) string {
	if row == nil {
		return ""
	}
	// The dedicated column is the canonical fencing value. The JSON field is
	// retained for compatibility with older readers only.
	if familyID := strings.TrimSpace(row.CredentialFamilyID); familyID != "" {
		return familyID
	}
	return strings.TrimSpace(row.GetCredential("credential_family_id"))
}

func findAntigravityDuplicateAccountID(rows []*database.AccountRow, familyID, email, profileID string, excludeID int64) int64 {
	familyID = strings.TrimSpace(familyID)
	email = strings.ToLower(strings.TrimSpace(email))
	profileID = strings.TrimSpace(profileID)
	for _, row := range rows {
		if row == nil || row.ID == excludeID {
			continue
		}
		if familyID != "" && antigravityRowFamilyID(row) == familyID {
			return row.ID
		}
		if profileID != "" {
			// Once a verified Google subject is available, an email match alone
			// must not block a distinct subject. Subject/family are authoritative.
			if strings.TrimSpace(row.GetCredential("account_id")) == profileID {
				return row.ID
			}
			continue
		}
		if email != "" && strings.TrimSpace(row.GetCredential("account_id")) == "" && strings.EqualFold(strings.TrimSpace(row.GetCredential("email")), email) {
			return row.ID
		}
	}
	return 0
}

func appendAntigravityWarning(current, next string) string {
	current = strings.TrimSpace(current)
	next = strings.TrimSpace(next)
	if current == "" {
		return next
	}
	if next == "" {
		return current
	}
	return current + "; " + next
}

func antigravityAuthoritativeProfile(profile auth.AntigravityProfile) bool {
	return profile.VerifiedEmail && strings.TrimSpace(profile.ID) != "" && strings.TrimSpace(profile.Email) != ""
}

func antigravityAccessDeniedReason(result auth.AntigravitySyncResult) string {
	if result.Quota.Forbidden {
		return "Google quota API denied access"
	}
	if result.EntitlementsObserved && !result.Entitlements.Allowed {
		if reason := strings.TrimSpace(result.Entitlements.Reason); reason != "" {
			return reason
		}
		return "Google permissions denied access"
	}
	return ""
}

// applyAntigravityAccessFence projects authoritative Google access facts onto
// the existing persistent account-status hard fence. A failed sync may only
// add a fence when it observed an explicit denial; only a fully successful
// sync is allowed to clear an earlier denial.
func (h *Handler) applyAntigravityAccessFence(ctx context.Context, id int64, result auth.AntigravitySyncResult, syncSucceeded bool) (bool, error) {
	if h == nil || h.db == nil || id <= 0 {
		return false, nil
	}
	if reason := antigravityAccessDeniedReason(result); reason != "" {
		return h.db.SetOwnedAccountError(ctx, id, antigravityAccessDeniedError, antigravityAccessDeniedError+": "+reason)
	}
	if syncSucceeded {
		return h.db.ClearOwnedAccountError(ctx, id, antigravityAccessDeniedError)
	}
	return false, nil
}

func (h *Handler) removeAntigravityRuntimeAccount(id int64) {
	if h != nil && h.store != nil && id > 0 {
		h.store.RemoveAccount(id)
	}
}

func (h *Handler) reloadAntigravityRuntimeAccount(ctx context.Context, id int64) error {
	if h.store == nil || id <= 0 {
		return nil
	}
	h.store.RemoveAccount(id)
	if err := h.store.LoadAccountByID(ctx, id); err != nil {
		// LoadAccountByID should publish only after a complete build, but remove
		// again defensively so future loader changes cannot leave a partial or
		// stale account dispatchable after a failed control-plane mutation.
		h.store.RemoveAccount(id)
		return err
	}
	return nil
}

// reconcileAntigravityRuntimeAfterCAS publishes a durable concurrent/ambiguous
// winner only when its generation demonstrably advanced beyond the caller's
// snapshot. Re-reading the same generation after provider token progress would
// revive the potentially consumed refresh token, so that case is removed.
func (h *Handler) reconcileAntigravityRuntimeAfterCAS(ctx context.Context, id, expectedGeneration int64) (bool, error) {
	current, err := h.db.GetAccountByID(ctx, id)
	if err != nil {
		h.removeAntigravityRuntimeAccount(id)
		return false, err
	}
	if current.CredentialGeneration <= expectedGeneration {
		h.removeAntigravityRuntimeAccount(id)
		return false, nil
	}
	if err := h.reloadAntigravityRuntimeAccount(ctx, id); err != nil {
		h.removeAntigravityRuntimeAccount(id)
		return false, err
	}
	return true, nil
}

// resolveAntigravityControlPlaneProxy applies the same account > group > pool
// > global > direct policy as inference. New accounts use DBID 0 with their
// requested proxy and target groups; existing callers pass the persisted ID
// and memberships loaded from the database.
func (h *Handler) resolveAntigravityControlPlaneProxy(accountID int64, accountProxyURL string, groupIDs []int64) (string, error) {
	accountProxyURL = strings.TrimSpace(accountProxyURL)
	if h == nil || h.store == nil {
		return accountProxyURL, nil
	}
	candidate := &auth.Account{
		DBID: accountID, ProxyURL: accountProxyURL,
		GroupIDs: append([]int64(nil), groupIDs...),
	}
	resolvedProxyURL, usable := h.store.ResolveUsableProxyForAccount(candidate)
	if !usable {
		return "", errors.New(antigravityNoUsableEgressError)
	}
	return strings.TrimSpace(resolvedProxyURL), nil
}

// AddAntigravityAccount imports one credential document and immediately syncs
// its Google identity, permissions, and quota snapshot.

type batchUpdateAntigravityModelsRequest struct {
	IDs    []int64  `json:"ids"`
	Models []string `json:"models"`
}

// UpdateAntigravityAccount updates editable metadata and optionally replaces
// the OAuth credential. Credential progress is fenced even when a later sync
// stage fails, so a rotated refresh token is not discarded.

// Google may already have consumed/rotated the source refresh token.
// Without a durable progress write, the old runtime credential is unsafe.

// Every provider observation below belongs to the old Google key. Clear
// it in the same identity CAS so neither runtime nor admin state can
// inherit stale quota, permission, catalog, or capability proof.

// Even model/mapping-only edits are generation-fenced. Advancing the
// generation is conservative but prevents a stale editor from overwriting
// a concurrent key rotation while retaining its newer canonical family.

// RefreshAntigravityQuota currently performs the same authenticated sync as a
// credential refresh because quota, permissions, and project metadata share it.

// BatchImportAntigravityAccounts imports portable Antigravity exports, manager
// exports, raw refresh tokens, and credential-store JSON. Enabled accounts are
// published only after persistence completes; disabled imports remain absent
// from the runtime pool.

// 代理注册必须早于任何账号落库，顺序理由见 registerImportedProxyBindings。
// 规范化结果写回 document，让账号绑定的 URL 与代理表里的那条完全一致——两者
// 只要差一个尾斜杠，账号就会被判为绑了一个不在池里的托管代理而不可调度。

func antigravityCredentialFamilyID(credential auth.AntigravityCredential, profileID string) string {
	kind := "refresh"
	value := strings.TrimSpace(credential.RefreshToken)
	if strings.TrimSpace(profileID) != "" {
		kind = "google-sub"
		value = strings.TrimSpace(profileID)
	}
	if value == "" {
		value = strings.TrimSpace(credential.Email)
		kind = "email"
	}
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("antigravity-credential-v1\x00" + kind + "\x00" + value))
	return "ag_" + hex.EncodeToString(sum[:])
}

func antigravityCredentialProvisionalFamilyID(credential auth.AntigravityCredential) string {
	if strings.TrimSpace(credential.RefreshToken) != "" {
		return antigravityCredentialFamilyID(credential, "")
	}
	value := strings.TrimSpace(credential.AccessToken)
	if value != "" {
		sum := sha256.Sum256([]byte("antigravity-provisional-v1\x00access\x00" + value))
		return "ag_" + hex.EncodeToString(sum[:])
	}
	return antigravityCredentialFamilyID(credential, "")
}

// This is an imported claim only. The canonical verified_email field is
// populated exclusively from live Google userinfo below.

// Use the credential that will actually be inserted when Sync returned
// an empty result (for example, a token endpoint failure). The helper
// deliberately quarantines unverified credentials and clears any
// identity-bound fields that could not be re-observed.

func antigravitySnapshotSource(row *database.AccountRow, result auth.AntigravitySyncResult) *database.AccountRow {
	if row == nil {
		return nil
	}
	previousID := strings.TrimSpace(row.GetCredential("account_id"))
	nextID := strings.TrimSpace(result.Profile.ID)
	if previousID != "" {
		if nextID == "" || previousID != nextID {
			return nil
		}
		return row
	}
	previousEmail := strings.TrimSpace(row.GetCredential("email"))
	nextEmail := strings.TrimSpace(result.Profile.Email)
	if previousEmail != "" {
		if nextEmail == "" || !strings.EqualFold(previousEmail, nextEmail) {
			return nil
		}
	}
	return row
}

func antigravityCredentialUpdates(result auth.AntigravitySyncResult, previous *database.AccountRow) (map[string]any, error) {
	originalPrevious := previous
	trustedPrevious := previous != nil && antigravityRowHasVerifiedIdentity(previous)
	sameIdentityPrevious := trustedPrevious && antigravitySnapshotSource(previous, result) != nil
	if !sameIdentityPrevious {
		previous = nil
	}
	quotaSnapshot := result.Quota
	if previous != nil && (!result.QuotaGroupsObserved || !result.AICreditsObserved) {
		var previousQuota auth.AntigravityQuotaSnapshot
		if raw := strings.TrimSpace(previous.GetCredential("antigravity_quota")); raw != "" && json.Unmarshal([]byte(raw), &previousQuota) == nil {
			if !result.QuotaGroupsObserved {
				quotaSnapshot.Groups = previousQuota.Groups
			}
			if !result.AICreditsObserved {
				quotaSnapshot.AICredits = previousQuota.AICredits
			}
		}
	}
	quota, err := json.Marshal(quotaSnapshot)
	if err != nil {
		return nil, err
	}
	models := auth.AntigravityDiscoveredModels(quotaSnapshot)
	updates := map[string]any{
		"upstream_type": auth.UpstreamAntigravity,
		"access_token":  result.Credential.AccessToken, "refresh_token": result.Credential.RefreshToken,
		"id_token": result.Credential.IDToken, "email": result.Profile.Email,
		"name": result.Profile.Name, "avatar_url": result.Profile.Picture,
		"verified_email": result.Profile.VerifiedEmail,
		"account_id":     result.Profile.ID, "project_id": "",
		"models":           models,
		"oauth_client_key": result.Credential.OAuthClientKey, "oauth_scope": result.Credential.Scope,
		"antigravity_client_id": result.Credential.ClientID, "antigravity_client_secret": result.Credential.ClientSecret,
		"antigravity_quota":      string(quota),
		"antigravity_sync_error": "", "antigravity_sync_warning": result.Warning,
		"antigravity_last_synced_at":       result.Quota.UpdatedAt.UTC().Format(time.RFC3339),
		"antigravity_last_sync_attempt_at": time.Now().UTC().Format(time.RFC3339),
	}
	if result.Credential.VerifiedEmailPresent {
		// Preserve the imported claim separately from the live, authoritative
		// userinfo result. This metadata is never projected as verified_email.
		updates[antigravityImportedVerifiedEmailKey] = result.Credential.VerifiedEmail
	}
	if sameIdentityPrevious {
		projectID := result.Credential.ProjectID
		if !result.EntitlementsObserved && strings.TrimSpace(projectID) == "" {
			projectID = previous.GetCredential("project_id")
		}
		updates["project_id"] = projectID
	}
	if originalPrevious != nil && !sameIdentityPrevious {
		updates["id_token"] = ""
	}
	if !sameIdentityPrevious && !result.EntitlementsObserved {
		// loadProject is the authority for the project/tier. A quota response
		// obtained while that authority is unavailable may have used a stale
		// project carried by the imported token, so do not publish it under a
		// newly discovered subject.
		updates["models"] = []string{}
		updates["antigravity_quota"] = ""
		updates["antigravity_last_synced_at"] = ""
	}
	if result.EntitlementsObserved {
		entitlements, marshalErr := json.Marshal(result.Entitlements)
		if marshalErr != nil {
			return nil, marshalErr
		}
		updates["project_id"] = result.Entitlements.ProjectID
		updates["plan_type"] = result.Entitlements.EffectiveTier
		updates["antigravity_permissions"] = string(entitlements)
		updates["antigravity_entitlements"] = string(entitlements)
	} else if previous == nil {
		// Identity replacement uses a merge-CAS to preserve unrelated account
		// metadata. Clear identity-bound snapshots when the new permissions are unknown.
		updates["plan_type"] = ""
		updates["antigravity_permissions"] = ""
		updates["antigravity_entitlements"] = ""
	}
	if previous == nil && !result.EntitlementsObserved {
		updates["project_id"] = ""
	}
	updates["expires_at"] = ""
	if !result.Credential.ExpiresAt.IsZero() {
		updates["expires_at"] = result.Credential.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return updates, nil
}

func antigravityCredentialFailureUpdates(result auth.AntigravitySyncResult, syncErr error) map[string]any {
	updates := map[string]any{
		"upstream_type":                    auth.UpstreamAntigravity,
		"antigravity_sync_error":           antigravityErrorString(syncErr),
		"antigravity_sync_warning":         result.Warning,
		"antigravity_last_sync_attempt_at": time.Now().UTC().Format(time.RFC3339),
	}
	credential := result.Credential
	if credential.VerifiedEmailPresent {
		updates[antigravityImportedVerifiedEmailKey] = credential.VerifiedEmail
	}
	for key, value := range map[string]string{
		"access_token": credential.AccessToken, "refresh_token": credential.RefreshToken,
		"id_token": credential.IDToken, "oauth_client_key": credential.OAuthClientKey,
		"oauth_scope": credential.Scope, "antigravity_client_id": credential.ClientID,
		"antigravity_client_secret": credential.ClientSecret,
	} {
		if strings.TrimSpace(value) != "" {
			updates[key] = value
		}
	}
	if !credential.ExpiresAt.IsZero() {
		updates["expires_at"] = credential.ExpiresAt.UTC().Format(time.RFC3339)
	}
	if antigravityAuthoritativeProfile(result.Profile) {
		updates["email"] = result.Profile.Email
		updates["name"] = result.Profile.Name
		updates["avatar_url"] = result.Profile.Picture
		updates["verified_email"] = result.Profile.VerifiedEmail
		updates["account_id"] = result.Profile.ID
		if result.EntitlementsObserved {
			updates["project_id"] = credential.ProjectID
		} else {
			updates["project_id"] = ""
		}
	}
	if result.EntitlementsObserved {
		if encoded, err := json.Marshal(result.Entitlements); err == nil {
			updates["plan_type"] = result.Entitlements.EffectiveTier
			updates["antigravity_permissions"] = string(encoded)
			updates["antigravity_entitlements"] = string(encoded)
		}
	}
	return updates
}

func antigravityErrorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (h *Handler) RefreshAntigravityAccount(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(c, http.StatusBadRequest, "无效的账号 ID")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), antigravityManualRefreshTimeout)
	defer cancel()
	item := h.refreshAntigravityAccount(ctx, id)
	if !item.OK {
		status := http.StatusBadGateway
		if item.Error == "账号不存在" {
			status = http.StatusNotFound
		}
		writeError(c, status, item.Error)
		return
	}
	c.JSON(http.StatusOK, item)
}

// isAntigravityAccountID 判断账号是否为 Antigravity 账号。禁用的 Antigravity 账号
// 不在运行时池里，运行时查不到时回落到数据库行。
func (h *Handler) isAntigravityAccountID(ctx context.Context, id int64) bool {
	if h == nil || id <= 0 {
		return false
	}
	if h.store != nil {
		if account := h.store.FindByID(id); account != nil {
			return account.IsAntigravityAPI()
		}
	}
	if h.db == nil {
		return false
	}
	row, err := h.db.GetAccountByID(ctx, id)
	if err != nil || row == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(row.GetCredential("upstream_type")), auth.UpstreamAntigravity)
}

// syncAntigravityAccountByID 是通用刷新入口(单号刷新、批量刷新)里的 Antigravity
// 分支：走控制面完整同步，失败原因原样作为错误返回。
func (h *Handler) syncAntigravityAccountByID(ctx context.Context, id int64) error {
	syncCtx, cancel := context.WithTimeout(ctx, antigravityManualRefreshTimeout)
	defer cancel()
	item := h.refreshAntigravityAccount(syncCtx, id)
	if item.OK {
		return nil
	}
	if message := strings.TrimSpace(item.Error); message != "" {
		return errors.New(message)
	}
	return errors.New("Antigravity 同步失败")
}

func (h *Handler) refreshAntigravityAccount(ctx context.Context, id int64) antigravityRefreshItem {
	item := antigravityRefreshItem{ID: id}
	row, err := h.db.GetAccountByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			item.Error = "账号不存在"
		} else {
			item.Error = err.Error()
		}
		return item
	}
	if !strings.EqualFold(strings.TrimSpace(row.GetCredential("upstream_type")), auth.UpstreamAntigravity) {
		item.Error = "仅 Antigravity 账号支持该操作"
		return item
	}
	credential := antigravityCredentialFromRow(row)
	// Let loadCodeAssist establish the project for the refreshed principal. A
	// project carried from the prior subject can otherwise be sent to quota
	// after a token rotation and produce an ambiguous snapshot.
	credential.ProjectID = ""
	credential.IDToken = ""
	preSyncFamilyID := antigravityRowFamilyID(row)
	if antigravityRowHasVerifiedIdentity(row) {
		// A known subject cannot safely remain the family for a token whose
		// replacement subject has not been reverified. Move to the pre-refresh
		// refresh-token lineage as the provisional family instead.
		preSyncFamilyID = antigravityCredentialProvisionalFamilyID(credential)
	}
	if preSyncFamilyID == "" {
		preSyncFamilyID = antigravityCredentialProvisionalFamilyID(credential)
	}
	groupIDs, err := h.db.GetAccountGroupIDs(ctx, row.ID)
	if err != nil {
		item.Error = err.Error()
		return item
	}
	proxyURL, err := h.resolveAntigravityControlPlaneProxy(row.ID, row.ProxyURL, groupIDs)
	if err != nil {
		item.Error = err.Error()
		return item
	}
	client, err := auth.NewAntigravityClient(proxyURL)
	if err != nil {
		item.Error = err.Error()
		return item
	}
	result, err := client.Sync(ctx, credential)
	if err == nil && !antigravityAuthoritativeProfile(result.Profile) {
		err = errors.New("Antigravity sync did not return a verified Google subject")
	}
	if err != nil {
		item.Email = strings.TrimSpace(result.Profile.Email)
		if item.Email == "" {
			item.Email = row.GetCredential("email")
		}
		item.Error = err.Error()
		providerProgress := antigravityFailureHasProgress(row, result)
		persistOutcome := h.persistFailedAntigravitySync(ctx, row, result, err, preSyncFamilyID)
		item.Warning = appendAntigravityWarning(item.Warning, persistOutcome.Warning)
		var fenceErr error
		if persistOutcome.ProgressPersisted || (!providerProgress && persistOutcome.Mutated) {
			h.invalidateAccountSnapshotCaches()
			_, fenceErr = h.applyAntigravityAccessFence(ctx, id, result, false)
		}
		if reason := antigravityAccessDeniedReason(result); reason != "" {
			item.Warning = appendAntigravityWarning(item.Warning, antigravityAccessDeniedError+": "+reason)
		}
		if fenceErr != nil {
			item.Warning = appendAntigravityWarning(item.Warning, "访问状态持久化失败: "+fenceErr.Error())
			h.removeAntigravityRuntimeAccount(id)
		} else if persistOutcome.ProgressPersisted || (!providerProgress && persistOutcome.Mutated) {
			if reloadErr := h.reloadAntigravityRuntimeAccount(ctx, id); reloadErr != nil {
				log.Printf("保存失败同步结果后重载 Antigravity 账号 %d 运行时状态失败: %v", id, reloadErr)
				item.Warning = appendAntigravityWarning(item.Warning, "运行时重载失败: "+reloadErr.Error())
			}
		} else if persistOutcome.ConcurrentWinner {
			if _, reconcileErr := h.reconcileAntigravityRuntimeAfterCAS(ctx, id, row.CredentialGeneration); reconcileErr != nil {
				item.Warning = appendAntigravityWarning(item.Warning, "运行时重载失败: "+reconcileErr.Error())
			}
		} else if providerProgress {
			h.removeAntigravityRuntimeAccount(id)
		}
		item.Error = appendAntigravityWarning(item.Error, item.Warning)
		return item
	}
	updates, err := antigravityCredentialUpdates(result, row)
	if err != nil {
		item.Error = err.Error()
		return item
	}
	familyID := antigravityCredentialFamilyID(result.Credential, result.Profile.ID)
	h.mergeDuplicateMu.Lock()
	rows, listErr := h.db.ListActiveByChannel(ctx, database.UpstreamChannelAntigravity)
	if listErr == nil {
		if duplicateID := findAntigravityDuplicateAccountID(rows, familyID, result.Profile.Email, result.Profile.ID, id); duplicateID > 0 {
			h.mergeDuplicateMu.Unlock()
			item.Error = fmt.Sprintf("Antigravity credential identity already exists (id=%d)", duplicateID)
			return item
		}
	}
	if listErr != nil {
		h.mergeDuplicateMu.Unlock()
		item.Error = listErr.Error()
		return item
	}
	var applied bool
	if familyID != "" && antigravityRowFamilyID(row) != familyID {
		_, applied, err = h.db.ReplaceAccountCredentialsCAS(ctx, id, row.CredentialGeneration, familyID, updates)
	} else {
		_, applied, err = h.db.UpdateAccountCredentialsCAS(ctx, id, row.CredentialGeneration, updates)
	}
	h.mergeDuplicateMu.Unlock()
	if err != nil {
		item.Error = err.Error()
		if _, reconcileErr := h.reconcileAntigravityRuntimeAfterCAS(ctx, id, row.CredentialGeneration); reconcileErr != nil {
			log.Printf("Antigravity 手工刷新 CAS 错误后无法确认 durable state (account=%d): %v", id, reconcileErr)
			item.Warning = appendAntigravityWarning(item.Warning, "运行时重载失败: "+reconcileErr.Error())
			item.Error = appendAntigravityWarning(item.Error, item.Warning)
		}
		return item
	}
	if !applied {
		item.Error = "账号凭据已被其他操作更新，请重试"
		if _, reconcileErr := h.reconcileAntigravityRuntimeAfterCAS(ctx, id, row.CredentialGeneration); reconcileErr != nil {
			log.Printf("Antigravity 手工刷新 CAS 冲突后重载 durable winner 失败 (account=%d): %v", id, reconcileErr)
			item.Warning = appendAntigravityWarning(item.Warning, "运行时重载失败: "+reconcileErr.Error())
			item.Error = appendAntigravityWarning(item.Error, item.Warning)
		}
		return item
	}
	if strings.TrimSpace(result.Profile.Name) != "" {
		_ = h.db.UpdateAccountName(ctx, id, result.Profile.Name)
	}
	if _, fenceErr := h.applyAntigravityAccessFence(ctx, id, result, true); fenceErr != nil {
		h.removeAntigravityRuntimeAccount(id)
		item.Error = "持久化 Antigravity 访问状态失败: " + fenceErr.Error()
		return item
	}
	if reason := antigravityAccessDeniedReason(result); reason != "" {
		item.Warning = appendAntigravityWarning(item.Warning, antigravityAccessDeniedError+": "+reason)
	}
	if err := h.reloadAntigravityRuntimeAccount(ctx, id); err != nil {
		log.Printf("刷新后重载 Antigravity 账号 %d 运行时状态失败: %v", id, err)
		item.Warning = appendAntigravityWarning(item.Warning, "运行时重载失败: "+err.Error())
	}
	h.db.InsertAccountEventAsync(id, "updated", "antigravity_refresh")
	item.Email = result.Profile.Email
	item.Warning = appendAntigravityWarning(result.Warning, item.Warning)
	item.Message = "Antigravity 账号信息与额度已刷新"
	item.OK = true
	return item
}

type antigravityFailedSyncPersistOutcome struct {
	Mutated           bool
	ProgressPersisted bool
	ConcurrentWinner  bool
	PersistenceFailed bool
	Warning           string
}

func (h *Handler) persistFailedAntigravitySync(ctx context.Context, row *database.AccountRow, result auth.AntigravitySyncResult, syncErr error, fallbackFamilyID string) antigravityFailedSyncPersistOutcome {
	if row == nil {
		return antigravityFailedSyncPersistOutcome{PersistenceFailed: true, Warning: "Antigravity source account is unavailable"}
	}
	basicUpdates := antigravityCredentialFailureUpdates(result, syncErr)
	if !antigravityFailureHasProgress(row, result) {
		mutated, err := h.db.MergeAccountCredentialsForGeneration(ctx, row.ID, row.CredentialGeneration, basicUpdates)
		if err != nil {
			return antigravityFailedSyncPersistOutcome{PersistenceFailed: true, Warning: "failed to record Antigravity sync error: " + err.Error()}
		}
		return antigravityFailedSyncPersistOutcome{Mutated: mutated, ConcurrentWinner: !mutated}
	}

	failureUpdates, familyID := antigravityFailureCredentialUpdates(row, result, syncErr, fallbackFamilyID)
	h.mergeDuplicateMu.Lock()
	defer h.mergeDuplicateMu.Unlock()
	rows, err := h.db.ListActiveByChannel(ctx, database.UpstreamChannelAntigravity)
	if err != nil {
		return antigravityFailedSyncPersistOutcome{PersistenceFailed: true, Warning: "failed to check Antigravity identity uniqueness: " + err.Error()}
	}
	if duplicateID := findAntigravityDuplicateAccountID(rows, familyID, result.Profile.Email, result.Profile.ID, row.ID); duplicateID > 0 {
		// A verified subject already owned by another row must not be silently
		// moved. Record the failure on the source and leave both identities
		// untouched; the process mutex serializes this check/write decision.
		warning := fmt.Sprintf("refreshed Google identity already belongs to account %d", duplicateID)
		// Do not merge rotated tokens or observed profile fields here: those
		// belong to the duplicate target. Persist only the source-row failure
		// marker and attempt metadata, preserving its prior credential lineage.
		duplicateFailure := map[string]any{
			"upstream_type":                    auth.UpstreamAntigravity,
			"antigravity_sync_error":           antigravityErrorString(syncErr),
			"antigravity_sync_warning":         result.Warning,
			"antigravity_last_sync_attempt_at": time.Now().UTC().Format(time.RFC3339),
		}
		mutated, mergeErr := h.db.MergeAccountCredentialsForGeneration(ctx, row.ID, row.CredentialGeneration, duplicateFailure)
		if mergeErr != nil {
			warning = appendAntigravityWarning(warning, "failed to record Antigravity sync error on source account: "+mergeErr.Error())
		}
		return antigravityFailedSyncPersistOutcome{Mutated: mutated, PersistenceFailed: mergeErr != nil, Warning: warning}
	}
	applied, persistErr := h.persistAntigravityFailureCredential(ctx, row, familyID, failureUpdates)
	if persistErr != nil {
		return antigravityFailedSyncPersistOutcome{PersistenceFailed: true, Warning: "failed to preserve refreshed Antigravity credential: " + persistErr.Error()}
	}
	if !applied {
		return antigravityFailedSyncPersistOutcome{ConcurrentWinner: true, Warning: "credential changed concurrently; refreshed token was not persisted"}
	}
	return antigravityFailedSyncPersistOutcome{Mutated: true, ProgressPersisted: true}
}

func (h *Handler) persistAntigravityFailureCredential(ctx context.Context, row *database.AccountRow, familyID string, updates map[string]any) (bool, error) {
	if row == nil {
		return false, sql.ErrNoRows
	}
	if familyID == "" || antigravityRowFamilyID(row) != familyID {
		_, applied, err := h.db.ReplaceAccountCredentialsCAS(ctx, row.ID, row.CredentialGeneration, familyID, updates)
		return applied, err
	}
	_, applied, err := h.db.UpdateAccountCredentialsCAS(ctx, row.ID, row.CredentialGeneration, updates)
	return applied, err
}

func antigravityFailureCredentialUpdates(row *database.AccountRow, result auth.AntigravitySyncResult, syncErr error, fallbackFamilyID string) (map[string]any, string) {
	updates := antigravityCredentialFailureUpdates(result, syncErr)
	profileObserved := antigravityAuthoritativeProfile(result.Profile)
	familyID := strings.TrimSpace(fallbackFamilyID)
	if familyID == "" {
		familyID = antigravityCredentialProvisionalFamilyID(result.Credential)
	}
	trustedSameIdentity := false
	if profileObserved {
		familyID = antigravityCredentialFamilyID(result.Credential, result.Profile.ID)
		trustedSameIdentity = antigravitySnapshotSource(row, result) != nil && antigravityRowHasVerifiedIdentity(row)
	}
	if trustedSameIdentity {
		if !result.EntitlementsObserved {
			delete(updates, "project_id")
		}
		return updates, familyID
	}

	updates["models"] = []string{}
	updates["antigravity_quota"] = ""
	updates["antigravity_last_synced_at"] = ""
	if !result.EntitlementsObserved || !profileObserved {
		updates["project_id"] = ""
		updates["plan_type"] = ""
		updates["antigravity_permissions"] = ""
		updates["antigravity_entitlements"] = ""
	}
	if !profileObserved {
		// A rotated credential without a reverified subject is quarantined under
		// its provisional credential family. Do not leave a new token paired with
		// the old Google principal or its display metadata.
		updates["account_id"] = ""
		updates["email"] = ""
		updates["name"] = ""
		updates["avatar_url"] = ""
		updates["verified_email"] = false
	}
	// Never carry an old principal's token across a provisional or subject
	// replacement transition.
	updates["id_token"] = ""
	if result.Credential.ExpiresAt.IsZero() {
		updates["expires_at"] = ""
	}
	return updates, familyID
}

func antigravityFailureHasProgress(row *database.AccountRow, result auth.AntigravitySyncResult) bool {
	if row == nil {
		return false
	}
	credential := result.Credential
	if strings.TrimSpace(credential.AccessToken) != "" && credential.AccessToken != row.GetCredential("access_token") {
		return true
	}
	if strings.TrimSpace(credential.RefreshToken) != "" && credential.RefreshToken != row.GetCredential("refresh_token") {
		return true
	}
	if antigravityAuthoritativeProfile(result.Profile) || result.EntitlementsObserved {
		return true
	}
	return false
}

func antigravityRowHasVerifiedIdentity(row *database.AccountRow) bool {
	if row == nil {
		return false
	}
	return strings.TrimSpace(row.GetCredential("account_id")) != "" ||
		(row.GetCredentialBool("verified_email") && strings.TrimSpace(row.GetCredential("email")) != "")
}

func antigravityCredentialFromRow(row *database.AccountRow) auth.AntigravityCredential {
	credential := auth.AntigravityCredential{
		AccessToken: row.GetCredential("access_token"), RefreshToken: row.GetCredential("refresh_token"),
		IDToken: row.GetCredential("id_token"), Email: row.GetCredential("email"), Name: row.GetCredential("name"),
		AvatarURL: row.GetCredential("avatar_url"), ProjectID: row.GetCredential("project_id"),
		OAuthClientKey: row.GetCredential("oauth_client_key"),
		ClientID:       row.GetCredential("antigravity_client_id"), ClientSecret: row.GetCredential("antigravity_client_secret"),
		Scope: row.GetCredential("oauth_scope"),
	}
	if raw := strings.TrimSpace(row.GetCredential("expires_at")); raw != "" {
		credential.ExpiresAt, _ = time.Parse(time.RFC3339, raw)
	}
	if value := row.GetCredentialOptionalBool(antigravityImportedVerifiedEmailKey); value != nil {
		credential.VerifiedEmail = *value
		credential.VerifiedEmailPresent = true
	}
	return credential
}
