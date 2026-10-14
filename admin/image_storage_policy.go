package admin

const (
	imageStorageLegacy                 = "legacy"
	imageStorageTemporary              = "temporary"
	imageStorageDeleteAfterRead        = "delete_after_read"
	defaultImageRetentionSeconds int64 = 2 * 60 * 60
	maxImageRetentionSeconds     int64 = 365 * 24 * 60 * 60
)

// Delete storage first, retaining an expired row on failure so maintenance can
// retry. Revocation also prevents cached/thumbnail reads while deletion retries.
