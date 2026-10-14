package admin

import (
	"errors"

	"time"
)

const (
	channelMonitorProbeTimeout       = 75 * time.Second
	channelMonitorHealthTimeout      = 60 * time.Second
	channelMonitorBillingTimeout     = 10 * time.Second
	channelMonitorBillingInterval    = 30 * time.Minute
	channelMonitorUnsupportedBilling = 4 * time.Hour
	channelMonitorLeaseTTL           = 90 * time.Second
	channelMonitorDegradedThreshold  = 6 * time.Second
	channelMonitorMaxResponseBytes   = 1024 * 1024
	channelMonitorMaxBillingBytes    = 64 * 1024
	channelMonitorDueBatchSize       = 20
)

var errChannelMonitorBusy = errors.New("该渠道正在探测，请稍后再试")

// StartChannelMonitor starts a bounded, due-time driven runner. An account
// lease is acquired for every probe, so multiple application replicas sharing
// Redis do not duplicate billable health calls.

// Add up to 10% deterministic jitter to avoid all cards firing together
// after a restart or bulk enable.

type channelMonitorResponseRead struct {
	Text         string
	FirstTokenMS int64
	Terminal     bool
	Failure      string
}

type channelMonitorBillingWire struct {
	Object                  string   `json:"object"`
	SchemaVersion           int      `json:"schema_version"`
	BillingScope            string   `json:"billing_scope"`
	GroupRateMultiplier     *float64 `json:"group_rate_multiplier"`
	UserRateMultiplier      *float64 `json:"user_rate_multiplier"`
	ResolvedRateMultiplier  *float64 `json:"resolved_rate_multiplier"`
	PeakRateEnabled         *bool    `json:"peak_rate_enabled"`
	PeakStart               *string  `json:"peak_start"`
	PeakEnd                 *string  `json:"peak_end"`
	PeakRateMultiplier      *float64 `json:"peak_rate_multiplier"`
	AppliedPeakMultiplier   *float64 `json:"applied_peak_multiplier"`
	EffectiveRateMultiplier *float64 `json:"effective_rate_multiplier"`
	Timezone                *string  `json:"timezone"`
	ObservedAt              string   `json:"observed_at"`
}
