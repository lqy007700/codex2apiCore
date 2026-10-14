package proxy

import (
	"sync"

	"github.com/codex2api/database"
	"github.com/gin-gonic/gin"
)

type imageJobBillingContextKey struct{}
type deferredImageUsage struct {
	handler *Handler
	input   *database.UsageLogInput
}
type imageJobBilling struct {
	mu       sync.Mutex
	finished bool
	entries  []deferredImageUsage
}

// DeferImageJobBilling lets Studio settle successful-image fees after saving its
// assets. The finalizer must be deferred, including on failure/panic. Token mode
// keeps its existing immediate logging behavior. The finalizer is idempotent.

func deferImageUsage(c *gin.Context, h *Handler, input *database.UsageLogInput) bool {
	if c == nil || c.Request == nil {
		return false
	}
	b, ok := c.Request.Context().Value(imageJobBillingContextKey{}).(*imageJobBilling)
	if !ok {
		return false
	}
	details := input.UserBillingDetails()
	// Failed attempts have no image fee and can be logged immediately. Retain
	// only successful outputs, keeping memory bounded during long retry loops.
	if details.UserBillingMode != database.UserBillingModePerImage || details.BilledImageCount == 0 {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.finished {
		return false
	}
	b.entries = append(b.entries, deferredImageUsage{handler: h, input: input})
	return true
}
