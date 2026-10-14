package admin

import (
	"context"

	"errors"

	"net/http"

	"time"

	"github.com/codex2api/auth"

	"github.com/codex2api/proxy"
)

const (
	antigravityCapabilityProbeTimeout = 30 * time.Second
	antigravityProbeResponseLimit     = 256 << 10
)

var (
	errAntigravityAccountRequired   = errors.New("only Antigravity accounts support this operation")
	errAntigravityCredentialChanged = errors.New("Antigravity credential changed during the operation; retry")
)

type antigravityCapabilityExecutor func(context.Context, *auth.Account, string, []byte, bool, string) (*http.Response, error)

type antigravityCatalogState struct {
	Models       []string `json:"models"`
	Source       string   `json:"source"`
	Verified     bool     `json:"verified"`
	Synchronized bool     `json:"synchronized"`
	ObservedAt   string   `json:"observed_at,omitempty"`
}

type antigravityIdentityState struct {
	Status        string `json:"status"`
	EmailVerified bool   `json:"email_verified"`
	SubjectKnown  bool   `json:"subject_known"`
	ProjectStatus string `json:"project_status"`
	ProjectID     string `json:"project_id,omitempty"`
}

type antigravityCapabilityObservation struct {
	CredentialGeneration int64     `json:"credential_generation"`
	Protocol             string    `json:"protocol"`
	ModelID              string    `json:"model_id"`
	Status               string    `json:"status"`
	Verified             bool      `json:"verified"`
	HTTPStatus           int       `json:"http_status,omitempty"`
	Source               string    `json:"source"`
	ObservedAt           time.Time `json:"observed_at"`
	ContentType          string    `json:"content_type,omitempty"`
}

type antigravityAccountState struct {
	AccountID            int64                              `json:"account_id"`
	CredentialGeneration int64                              `json:"credential_generation"`
	CredentialKind       string                             `json:"credential_kind"`
	Catalog              antigravityCatalogState            `json:"catalog"`
	Identity             antigravityIdentityState           `json:"identity"`
	Permissions          *auth.AntigravityEntitlements      `json:"permissions,omitempty"`
	Quota                *auth.AntigravityQuotaSnapshot     `json:"quota,omitempty"`
	Capabilities         []antigravityCapabilityObservation `json:"capabilities"`
	LastSyncedAt         string                             `json:"last_synced_at,omitempty"`
	LastSyncAttemptAt    string                             `json:"last_sync_attempt_at,omitempty"`
	LastCapabilityProbe  string                             `json:"last_capability_probe_at,omitempty"`
	Warnings             []string                           `json:"warnings"`
}

type antigravityStateSyncResponse struct {
	Message       string                   `json:"message"`
	State         *antigravityAccountState `json:"state"`
	Remote        bool                     `json:"remote"`
	CatalogSource string                   `json:"catalog_source"`
	Verified      bool                     `json:"verified"`
}

// GetAntigravityAccountState returns sanitized persisted observations only. It
// never performs an upstream request and never serializes credential secrets.

func (h *Handler) runAntigravityRefresh(ctx context.Context, id int64) antigravityRefreshItem {
	if h.antigravitySyncAccount != nil {
		return h.antigravitySyncAccount(ctx, id)
	}
	return h.refreshAntigravityAccount(ctx, id)
}

// SyncAntigravityAccountState refreshes OAuth control-plane facts. API-key
// accounts are deliberately local-only: no remote catalog claim is fabricated.

func (h *Handler) antigravityProbeExecutor() antigravityCapabilityExecutor {
	if h.antigravityCapabilityProbe != nil {
		return h.antigravityCapabilityProbe
	}
	return proxy.ExecuteAntigravityResponsesRequest
}

// ProbeAntigravityAccountCapabilities performs exactly one bounded, non-stream
// generation request against one model. It is never called by state reads or
// sync and is the only path that can mark API-key Interactions as verified.
