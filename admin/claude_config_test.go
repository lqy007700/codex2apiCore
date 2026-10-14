package admin

// Soft-delete the row directly in the DB (bypassing the store), so the
// fingerprint persist inside SyncClaudeCLIVersion hits sql.ErrNoRows
// while the in-memory Store still thinks the account is live.

// Explicit 0 must persist as 0 (follow global), not be re-defaulted to 120.
