package proxy

import (
	"sync/atomic"
)

type imageExecutionGate struct{ slots chan struct{} }
type imageExecutionContextKey struct{}

var processImageGate atomic.Pointer[imageExecutionGate]

// Configure once before accepting traffic. Shared by all keys and both direct
// image endpoints and durable workers, including preprocessing/postprocessing.

// Workers reserve per-key concurrency before downloading inputs. Saturation
// means wait, not creation of a failed job.
