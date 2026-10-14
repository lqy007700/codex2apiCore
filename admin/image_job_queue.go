package admin

import (
	"fmt"

	"os"

	"strconv"
	"strings"
	"sync"
	"time"
)

const queueInputPrefix = "queue-input:"
const queueInputBudget = 32 << 20
const queueLeaseDuration = 2 * time.Minute
const queueHeadScanLimit = 64
const queueStaleScanLimit = 32
const defaultMaxQueuedPerKey = 1000
const defaultMaxQueueWait = 24 * time.Hour

type imageJobQueue struct {
	intake          chan struct{}
	wg              sync.WaitGroup
	pipeline        bool
	maxQueuedPerKey int           // 0 disables the per-key backlog cap
	maxQueueWait    time.Duration // 0 disables failing jobs that never started
}

// ImageJobWorkerCount reads the opt-in worker limit; zero preserves legacy admission.
func ImageJobWorkerCount() (int, error) {
	raw := strings.TrimSpace(os.Getenv("IMAGE_JOB_WORKERS"))
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 || n > 32 {
		return 0, fmt.Errorf("IMAGE_JOB_WORKERS must be 0..32")
	}
	return n, nil
}

// StartImageJobQueue starts a bounded durable dispatcher before HTTP traffic.

// Body decoding is bounded too, including clients uploading data URLs. URL
// submissions wait here without downloading anything and do not hold a worker.

// URLs are validated without fetching. Uploaded data URLs are spooled to disk
// so neither the database nor the queue retains their Base64 payload.

// Creation receipts are intentionally small; polling/detail endpoints
// retain their existing contracts.

// Take the key slot before claiming: a key saturated by its own requests
// must leave its jobs queued instead of parking a worker on them.

// failStaleQueuedImages fails jobs that never started within maxQueueWait.
// They never reached upstream, so retrying them is safe.

// release is the API-key slot taken before the claim; nil when the key is
// unusable and the job is about to fail.
