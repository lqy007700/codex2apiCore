package admin

// Positive jitter never shortens the declared minimum delay.

// Refill free slots without waiting for the slowest account in a batch.
// Claimed rows start immediately, so no local waiting queue consumes leases.

// Drain completions together to avoid a query per account.
