package admin

import (
	"context"

	"log"

	"github.com/codex2api/proxy"
)

type modelTraceBankStatus struct {
	Active   proxy.ModelTraceBankInfo  `json:"active"`
	Embedded proxy.ModelTraceBankInfo  `json:"embedded"`
	Override *proxy.ModelTraceBankInfo `json:"override,omitempty"`
	// OverrideStale means a release shipped a bank at least as new as the
	// installed override, so the embedded copy is used instead.
	OverrideStale bool   `json:"override_stale,omitempty"`
	OverrideError string `json:"override_error,omitempty"`
}

type modelTraceBankUpdateResponse struct {
	modelTraceBankStatus
	Updated       bool     `json:"updated"`
	Message       string   `json:"message"`
	AddedModels   []string `json:"added_models,omitempty"`
	RemovedModels []string `json:"removed_models,omitempty"`
}

// modelTraceBankNewer reports whether candidate was trained after base. Banks
// without a timestamp never displace one that has it.
func modelTraceBankNewer(candidate, base *proxy.ModelTraceDetector) bool {
	return !candidate.BuiltAt().IsZero() && candidate.BuiltAt().After(base.BuiltAt())
}

// resolveModelTraceBanks loads the embedded bank and any installed override.
// The override is re-read whenever its stored revision changes, so an update
// made on one replica reaches the others on their next detection.
func (h *Handler) resolveModelTraceBanks(ctx context.Context) (active, embedded, override *proxy.ModelTraceDetector, overrideErr error) {
	embedded, err := proxy.NewModelTraceDetector()
	if err != nil {
		return nil, nil, nil, err
	}
	if h == nil || h.db == nil {
		return embedded, embedded, nil, nil
	}
	revision, err := h.db.GetModelTraceBankRevision(ctx)
	if err != nil || revision == "" {
		return embedded, embedded, nil, err
	}
	override = proxy.CachedModelTraceOverride(revision)
	if override == nil {
		stored, err := h.db.GetModelTraceBankOverride(ctx)
		if err != nil || stored == nil {
			return embedded, embedded, nil, err
		}
		override, err = proxy.ParseModelTraceBankOverride(stored.BankJSON, stored.Revision)
		if err != nil {
			return embedded, embedded, nil, err
		}
		proxy.CacheModelTraceOverride(override)
	}
	if !modelTraceBankNewer(override, embedded) {
		return embedded, embedded, override, nil
	}
	return override, embedded, override, nil
}

func (h *Handler) currentModelTraceDetector(ctx context.Context) (*proxy.ModelTraceDetector, error) {
	active, _, _, overrideErr := h.resolveModelTraceBanks(ctx)
	if overrideErr != nil {
		log.Printf("[modeltrace] 已安装的指纹库不可用，回退内置版本: %v", overrideErr)
	}
	if active == nil {
		return nil, overrideErr
	}
	return active, nil
}

func (h *Handler) modelTraceBankStatus(ctx context.Context) (modelTraceBankStatus, error) {
	active, embedded, override, overrideErr := h.resolveModelTraceBanks(ctx)
	if active == nil {
		return modelTraceBankStatus{}, overrideErr
	}
	status := modelTraceBankStatus{Active: active.Info(), Embedded: embedded.Info()}
	if override != nil {
		info := override.Info()
		status.Override = &info
		status.OverrideStale = active != override
	}
	if overrideErr != nil {
		status.OverrideError = overrideErr.Error()
	}
	return status, nil
}

// GetModelTraceBank reports the active fingerprint bank.
//
// GET /api/admin/modeltrace/bank

// UpdateModelTraceBank installs the bank from upstream main when it is newer
// than the active one. Nothing is fetched unless an operator asks for it.
//
// POST /api/admin/modeltrace/bank/update

// The stored revision is both the freshness baseline and the CAS token for
// the save. Falling back to the embedded bank here could downgrade a newer
// installed override, so a read failure aborts the update.

// ResetModelTraceBank removes the installed override and reverts to the bank
// bundled with this release.
//
// DELETE /api/admin/modeltrace/bank
