package admin

// officialPricingSyncSlot ensures a manual sync and the background poller never
// fetch/write pricing at the same time. Waiting is context-aware, so shutdown and
// cancelled HTTP requests do not get stuck behind a slow official site.
var officialPricingSyncSlot = func() chan struct{} {
	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	return ch
}()

type officialPricingSyncConfigResponse struct {
	Enabled         bool    `json:"enabled"`
	IntervalMinutes int     `json:"interval_minutes"`
	IncludeOpenAI   bool    `json:"include_openai"`
	IncludeGrok     bool    `json:"include_grok"`
	IncludeClaude   bool    `json:"include_claude"`
	LastAttemptAt   *string `json:"last_attempt_at,omitempty"`
	LastSuccessAt   *string `json:"last_success_at,omitempty"`
	LastError       string  `json:"last_error,omitempty"`
	LastWarning     string  `json:"last_warning,omitempty"`
}

type updateOfficialPricingSyncConfigRequest struct {
	Enabled         bool `json:"enabled"`
	IntervalMinutes int  `json:"interval_minutes"`
	IncludeOpenAI   bool `json:"include_openai"`
	IncludeGrok     bool `json:"include_grok"`
	IncludeClaude   bool `json:"include_claude"`
}

type syncOfficialPricingRequest struct {
	IncludeOpenAI *bool `json:"include_openai"`
	IncludeGrok   *bool `json:"include_grok"`
	IncludeClaude *bool `json:"include_claude"`
}

// StartOfficialPricingSync starts the optional official price poller. It is off
// by default and performs no network access until the administrator enables it.
