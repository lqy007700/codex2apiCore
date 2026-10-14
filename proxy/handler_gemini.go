package proxy

import (
	"encoding/json"

	"io"

	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var geminiNativeGenerationMethods = []string{
	"generateContent",
	"streamGenerateContent",
	"countTokens",
}

// GeminiListModels handles GET /v1beta/models for native Gemini clients.

// GeminiGetModel handles GET /v1beta/models/{model} for native Gemini clients.

// GeminiModelsAction handles native Gemini API POST /v1beta/models/*action requests
// and routes Antigravity OAuth accounts through the Cloud Code v1internal adapter.

func geminiInboundEndpoint(stream bool) string {
	if stream {
		return "/v1beta/models:streamGenerateContent"
	}
	return "/v1beta/models:generateContent"
}

type geminiNativeStreamResult struct {
	usage        UsageInfo
	firstTokenMs int
	readErr      error
	writeErr     error
}

// forwardGeminiNativeStream keeps reads and downstream writes distinguishable
// for usage logging. Emit complete SSE events and flush each one so a thought
// does not sit in the HTTP buffer while the upstream prepares its answer.
func forwardGeminiNativeStream(c *gin.Context, body io.Reader, startedAt time.Time) geminiNativeStreamResult {
	var result geminiNativeStreamResult
	var usage geminiNativeUsage
	activateContinuousRetryKeepalive(c.Request.Context())
	result.readErr = readSSEStreamWithContinuousRetryKeepalive(c.Request.Context(), body, func(_ string, data []byte) bool {
		if c.Request.Context().Err() != nil {
			return false
		}
		usage.observe(data)
		if result.firstTokenMs == 0 && !startedAt.IsZero() && geminiNativeHasContent(data) {
			result.firstTokenMs = max(int(time.Since(startedAt).Milliseconds()), 1)
		}
		frame := "data: " + string(data) + "\n\n"
		var written int
		written, result.writeErr = c.Writer.WriteString(frame)
		if result.writeErr == nil && written != len(frame) {
			result.writeErr = io.ErrShortWrite
		}
		if result.writeErr != nil {
			return false
		}
		c.Writer.Flush()
		return true
	})
	result.usage = usage.usageInfo()
	return result
}

// writeGeminiNativeError uses the same native error envelope before and after
// SSE headers are committed. Once streaming has started, the HTTP status cannot
// change. Google SDKs expect a bare JSON error at the end of the stream; an SSE
// data error is silently discarded by the SDK bundled with Gemini CLI 0.61.0.
func writeGeminiNativeError(c *gin.Context, status int, message string) {
	statusName := "INTERNAL"
	switch status {
	case http.StatusBadGateway, http.StatusServiceUnavailable:
		statusName = "UNAVAILABLE"
	case http.StatusGatewayTimeout, http.StatusRequestTimeout:
		statusName = "DEADLINE_EXCEEDED"
	case http.StatusTooManyRequests:
		statusName = "RESOURCE_EXHAUSTED"
	case http.StatusBadRequest:
		statusName = "INVALID_ARGUMENT"
	case http.StatusUnauthorized:
		statusName = "UNAUTHENTICATED"
	case http.StatusForbidden:
		statusName = "PERMISSION_DENIED"
	case http.StatusNotFound:
		statusName = "NOT_FOUND"
	}
	payload := gin.H{"error": gin.H{"code": status, "message": message, "status": statusName}}
	if !c.Writer.Written() {
		c.Header("Content-Type", "application/json; charset=utf-8")
		c.JSON(status, payload)
		return
	}
	if !strings.HasPrefix(c.Writer.Header().Get("Content-Type"), "text/event-stream") {
		return
	}
	data, _ := json.Marshal(payload)
	if _, err := c.Writer.Write(data); err == nil {
		c.Writer.Flush()
	}
}

// Gemini CLI's bundled Google SDK expects data events and cannot skip SSE
// comments. An empty response keeps the connection alive without content.

// Native Gemini forwards the stream directly and does not drain usage
// after a disconnect. Keep this context alive through body consumption,
// while letting a canceled client stop a blocked upstream read immediately.

// The native reader only returns a clean EOF after a real Gemini
// terminal. A successful upstream read still requires delivery to the
// downstream client before this request can be recorded as successful.

// Gemini clients retry RESOURCE_EXHAUSTED/UNAVAILABLE themselves,
// so keep the upstream status instead of a generic 502.
