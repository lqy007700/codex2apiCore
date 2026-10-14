package admin

type channelMonitorConfigRequest struct {
	Enabled         bool   `json:"enabled"`
	IntervalMinutes int    `json:"interval_minutes"`
	Model           string `json:"model"`
}

type channelMonitorConfigResponse struct {
	AccountID       int64    `json:"account_id"`
	Enabled         bool     `json:"enabled"`
	IntervalMinutes int      `json:"interval_minutes"`
	Model           string   `json:"model"`
	AvailableModels []string `json:"available_models"`
	LastCheckedAt   *string  `json:"last_checked_at,omitempty"`
	NextCheckAt     *string  `json:"next_check_at,omitempty"`
}

type channelMonitorCheckResponse struct {
	Status       string `json:"status"`
	HTTPStatus   int    `json:"http_status,omitempty"`
	LatencyMS    int64  `json:"latency_ms"`
	FirstTokenMS int64  `json:"first_token_ms"`
	CheckedAt    string `json:"checked_at"`
}

type channelMonitorBillingResponse struct {
	Status       string         `json:"status"`
	Data         map[string]any `json:"data,omitempty"`
	Message      string         `json:"message,omitempty"`
	HTTPStatus   int            `json:"http_status,omitempty"`
	CheckedAt    *string        `json:"checked_at,omitempty"`
	SuccessAt    *string        `json:"success_at,omitempty"`
	NextCheckAt  *string        `json:"next_check_at,omitempty"`
	FailureCount int            `json:"failure_count,omitempty"`
}

type channelMonitorCardResponse struct {
	AccountID       int64                         `json:"account_id"`
	Name            string                        `json:"name"`
	BaseURL         string                        `json:"base_url"`
	Model           string                        `json:"model"`
	IntervalMinutes int                           `json:"interval_minutes"`
	Status          string                        `json:"status"`
	HTTPStatus      int                           `json:"http_status,omitempty"`
	LatencyMS       int64                         `json:"latency_ms"`
	FirstTokenMS    int64                         `json:"first_token_ms"`
	Message         string                        `json:"message,omitempty"`
	LastCheckedAt   *string                       `json:"last_checked_at,omitempty"`
	NextCheckAt     *string                       `json:"next_check_at,omitempty"`
	Availability7d  *float64                      `json:"availability_7d,omitempty"`
	Checks7d        int64                         `json:"checks_7d"`
	Billing         channelMonitorBillingResponse `json:"billing"`
	RecentChecks    []channelMonitorCheckResponse `json:"recent_checks"`
}

type channelMonitorListResponse struct {
	Items       []channelMonitorCardResponse `json:"items"`
	GeneratedAt string                       `json:"generated_at"`
}

type channelMonitorBillingRateItemResponse struct {
	AccountID int64                         `json:"account_id"`
	Billing   channelMonitorBillingResponse `json:"billing"`
}

type channelMonitorBillingRatesResponse struct {
	Items       []channelMonitorBillingRateItemResponse `json:"items"`
	GeneratedAt string                                  `json:"generated_at"`
}

// GetChannelMonitorConfig returns defaults without creating a database row.
// This keeps unopened Responses accounts completely inert.

// ListChannelMonitorBillingRates is the lightweight account-list projection.
// It deliberately omits health history so rendering a balance column does not
// turn into one history query per monitored account.
