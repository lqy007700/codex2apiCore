package admin

// Reserved gateway-owned headers can't be smuggled in as custom headers.

// API Key documents accept arbitrary operator headers (issue #647), unlike
// OAuth documents whose fingerprint_headers stay identity-only.

// TestClaudeAPIKeyCustomHeadersAndIdentityLifecycle covers issue #647 end to
// end on the admin side: import persists custom_headers + client identity mode,
// the detail response and export round trip carry them, the scheduler update
// endpoint validates reserved names, and model discovery presents them.

// Scheduler update: reserved names rejected, valid headers + mode applied live.
