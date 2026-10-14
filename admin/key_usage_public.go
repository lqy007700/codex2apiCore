package admin

import (
	"github.com/codex2api/database"
)

const (
	publicAPIKeyUsageDefaultLogPageSize = 25
	publicAPIKeyUsageMaxLogPageSize     = 100
)

type publicAPIKeyUsageResponse struct {
	Key               publicAPIKeyUsageKey               `json:"key"`
	Range             publicAPIKeyUsageRange             `json:"range"`
	Usage             *database.APIKeySelfUsageReport    `json:"usage"`
	ModelRequestUsage []database.APIKeyModelRequestUsage `json:"model_request_usage"`
}

type publicAPIKeyUsageKey struct {
	Name        string                `json:"name"`
	Key         string                `json:"key"`
	QuotaLimit  float64               `json:"quota_limit"`
	QuotaUsed   float64               `json:"quota_used"`
	TotalUsed   float64               `json:"total_used"`
	ResetCount  int                   `json:"reset_count"`
	LastResetAt *string               `json:"last_reset_at,omitempty"`
	ExpiresAt   *string               `json:"expires_at,omitempty"`
	Limits      database.APIKeyLimits `json:"limits"`
	Status      string                `json:"status"`
	CreatedAt   string                `json:"created_at"`
}

type publicAPIKeyUsageRange struct {
	Name  string  `json:"name"`
	Start *string `json:"start,omitempty"`
	End   string  `json:"end"`
}

// 停用是管理员吊销凭据，与代理链路一致直接拒绝；过期/额度用尽仍可读，
// 让持有者看到状态与历史用量，明白调用为何失败。

const publicAPIKeyUsageMaxFilterLen = 128
