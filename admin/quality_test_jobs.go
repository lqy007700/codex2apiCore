package admin

import (
	"net/http"

	"time"
)

// Called before accepting HTTP requests. Shutdown cancels work before closing stores.

func (h *Handler) WaitQualityTests() { h.qualityTestWG.Wait() }

// Admission is short and detached: navigating away after submitting cannot leave
// a committed record without its runner. Database uniqueness arbitrates all replicas.

// "default" selects runs that used the model default (stored as "").

// preset=none | builtin:<key> | custom:<id>

// The existing account-test pipeline writes SSE to this in-process event sink.
// No client socket, request credentials, or gin request context is retained by the task.
type qualityJobWriter struct {
	header http.Header
	status int
	emit   func(testEvent)
}

func (w *qualityJobWriter) Header() http.Header { return w.header }

func (w *qualityJobWriter) Flush() {}

// interrupted tracks whether the latest error event was a transport/stream break.

// Diagnostics time from the attempt start; after a retry keep the job-relative value.

// A break caused by our own cancellation is not an upstream interruption.

// qualityTestMaxRetries bounds automatic re-attempts. Only a break before any
// output is retried: once output exists a retry would regenerate (and bill) it all.
const qualityTestMaxRetries = 1

var qualityTestRetryDelay = 2 * time.Second
