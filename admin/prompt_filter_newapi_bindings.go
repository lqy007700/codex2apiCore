package admin

import (
	"time"
)

const (
	defaultPromptFilterBindingSecretGraceSeconds = int64(300)
	minGeneratedBindingSecretGraceSeconds        = int64(60)
	maxPromptFilterBindingSecretGraceSeconds     = int64(86400)
	promptFilterBindingMutationTimeout           = 5 * time.Second
)

type promptFilterNewAPIBindingCreateRequest struct {
	APIKeyID              int64  `json:"api_key_id"`
	PlatformCode          string `json:"platform_code"`
	PlatformName          string `json:"platform_name"`
	Enabled               *bool  `json:"enabled"`
	RequireSignedIdentity *bool  `json:"require_signed_identity"`
	PromptFilterScope     string `json:"prompt_filter_scope"`
}

type promptFilterNewAPIBindingUpdateRequest struct {
	PlatformCode          *string `json:"platform_code"`
	PlatformName          *string `json:"platform_name"`
	Enabled               *bool   `json:"enabled"`
	RequireSignedIdentity *bool   `json:"require_signed_identity"`
	PromptFilterScope     *string `json:"prompt_filter_scope"`
}

type promptFilterNewAPIBindingSecretRequest struct {
	Secret       string `json:"secret"`
	GraceSeconds *int64 `json:"grace_seconds"`
}

type promptFilterNewAPIBindingResponse struct {
	APIKeyID                int64      `json:"api_key_id"`
	PlatformCode            string     `json:"platform_code"`
	PlatformName            string     `json:"platform_name"`
	Enabled                 bool       `json:"enabled"`
	RequireSignedIdentity   bool       `json:"require_signed_identity"`
	PromptFilterScope       string     `json:"prompt_filter_scope"`
	SecretConfigured        bool       `json:"secret_configured"`
	SecretMasked            string     `json:"secret_masked"`
	PreviousSecretActive    bool       `json:"previous_secret_active"`
	PreviousSecretExpiresAt *time.Time `json:"previous_secret_expires_at,omitempty"`
	UpdatedAt               time.Time  `json:"updated_at"`
	Secret                  string     `json:"secret,omitempty"`
}
