package admin

import (
	"net/http"
	"net/http/httptest"

	"testing"

	"github.com/codex2api/proxy"
	"github.com/gin-gonic/gin"
)

// newExternalImageJobRouter wires the external job route the same way the
// production server does, so admission checks run in their real order.

// occupyAPIKeyConcurrency takes the key's concurrency slot the way an already
// running job would.
func occupyAPIKeyConcurrency(t *testing.T, imageProxy *proxy.Handler, apiKeyValue string) func() {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/jobs", nil)
	ctx.Request.Header.Set("Authorization", "Bearer "+apiKeyValue)
	imageProxy.APIKeyAuthMiddleware()(ctx)
	if ctx.IsAborted() {
		t.Fatalf("API key preflight failed: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	release, ok := imageProxy.AcquireAPIKeyConcurrency(ctx)
	if !ok || release == nil {
		t.Fatal("failed to occupy the API-key concurrency slot")
	}
	return release
}

// A saturated key must be refused at enqueue. Accepting the request and letting
// the background job fail instead would turn MaxConcurrency into a generator of
// failed job rows rather than a limit.

// The enqueue slot covers the whole job, so the in-process upstream calls must
// inherit it. Taking a second slot for the same key would make a job with
// MaxConcurrency=1 deadlock against itself and fail every time.

// The job runs without upstream accounts and therefore fails, but it must
// not fail because it could not obtain its own concurrency slot.

// A partially degraded batch still succeeded. Callers polling the external job
// API key off error_message to detect failure, so the notice must not appear
// there.

// A genuinely failed job keeps reporting through error_message.

// Unset and out-of-range counts must still produce a usable deadline.

// Cropping stays available, but only when an operator asks for it: the default
// must match the local backend, which never crops.
