package admin

type imageStudioPublicPricing struct {
	UserBillingMode string  `json:"user_billing_mode"`
	ImageUnitPrice  float64 `json:"image_unit_price,omitempty"`
}

type imageStudioQuotaResponse struct {
	ImagePricing        map[string]imageStudioPublicPricing `json:"image_pricing"`
	QuotaLimit          float64                             `json:"quota_limit"`
	QuotaUsed           float64                             `json:"quota_used"`
	QuotaRemaining      *float64                            `json:"quota_remaining"`
	ExpiresAt           *string                             `json:"expires_at"`
	Status              string                              `json:"status"`
	RefreshAfterSeconds int                                 `json:"refresh_after_seconds"`
}

// GetPortalImageQuota is a read-only credential check, independent of the public
// usage-report switch. Exhausted/expired keys may inspect their own quota, while
// generation routes continue to enforce the existing /v1 authorization rules.

// Read the database row directly: auth-cache snapshots exclude live usage.

// Usage settlement is asynchronous. Let the UI refresh again after a
// flush interval instead of forcing a global log flush on every read.
