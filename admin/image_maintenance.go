package admin

type imageRetention struct{ assets, jobs int }

// StartImageMaintenance schedules bounded expiration and orphan-file cleanup.

// Delay initial work until startup/migrations have completed. Work is bounded
// and retried hourly; no vacuum or full-table rewrite on the request path.

// Keep the row if storage deletion fails, so another pass can retry it.
