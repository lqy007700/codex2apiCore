package admin

import (
	"time"
)

const promptRiskTrustSyncInterval = 30 * time.Second
const promptRiskTrustAdaptiveReconcileInterval = 5 * time.Minute

type promptRiskTrustPolicyRequest struct {
	DurationHours int    `json:"duration_hours"`
	RiskThreshold int    `json:"risk_threshold"`
	Reason        string `json:"reason"`
}
