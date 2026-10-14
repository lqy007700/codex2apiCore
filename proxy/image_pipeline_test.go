package proxy

import (
	"net/http"

	"strings"

	"github.com/tidwall/gjson"
)

// Completion includes cleanup; a buffered send before deferred Close
// lets the assertion race the final worker's file removal.

// Ten retained input copies alone would exceed 80 MiB. Allow transport/GC
// metadata headroom but reject payload retention across the waiting phase.

func largePipelineInput() ([]byte, error) {
	return []byte(`{"model":"gpt-image-2","prompt":"pipeline memory test","images":[{"image_url":"data:image/png;base64,` + strings.Repeat("A", 8<<20) + `"}]}`), nil
}
func validatePipelineUpload(r *http.Request) bool {
	b := readUpstreamRequestBody(r)
	return len(gjson.GetBytes(b, "input.0.content.1.image_url").String()) == (8<<20)+len("data:image/png;base64,")
}
