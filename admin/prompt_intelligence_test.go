package admin

// A local publish can move the Store to R2 while another replica moves the
// database back to the earlier R1 snapshot before the next poll. Sync must
// compare against the current Store, not a goroutine-local lastRaw cache.

// A later administrator restore of the exact historical signal rule is a
// deliberate manual action. The durable migration evidence must prevent a
// future restart from deleting it again.

// During a blue-green rollout an older instance may still run the removed
// AutoAdd path. A newer legacy write log is a new migration generation and
// must remove the automatically reintroduced rule again.

// Once that newer generation is completed, a subsequent restore without a
// newer legacy log is again treated as an intentional administrator action.

// Simulate a process stopping after candidate/evidence staging but before
// the legacy runtime rule was removed. This evidence is not completion.
