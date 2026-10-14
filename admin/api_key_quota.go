package admin

const (
	adminAPIKeyLimitsCacheNamespace = "api-key-limits"
	apiKeyResetWindow5h             = "5h"
	apiKeyResetWindow7d             = "7d"
)

// ResetAPIKeyQuota manually renews one API key's cumulative and 5h/7d quota
// periods while preserving historical usage logs and total usage.
//
// POST /api/admin/keys/:id/reset-quota
//
// The reset timestamp is used as an aggregation cutoff for 5h/7d cost and
// token limits, so neither the admin UI nor the gateway counts older events.
// Runtime caches are evicted before success is returned.

// ResetAllAPIKeyQuotas renews cumulative and 5h/7d quota periods for all API
// keys in one statement, preserving historical usage logs.
//
// POST /api/admin/keys/reset-all-quotas
