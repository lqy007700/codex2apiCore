package proxy

import (
	"encoding/json"
)

// realtimeTextSession adapts the text subset of the OpenAI Realtime protocol
// to the Responses WebSocket transport already supported by Codex2API. Audio
// buffer events are rejected explicitly rather than being silently discarded.
type realtimeTextSession struct {
	Model        string
	Instructions string
	Tools        json.RawMessage
	ToolChoice   json.RawMessage
	Items        []json.RawMessage
	History      []json.RawMessage
	historyBytes int
}

const (
	realtimeTextHistoryMaxItems = 64
	realtimeTextHistoryMaxBytes = 256 << 10
)

// RealtimeWebSocket accepts standard Realtime text events from NewAPI and
// translates each response.create turn into a Responses WebSocket turn.

// Session acknowledgements can echo a large original frame. Retain its
// admission until the write finishes; forwarding obtains its own lease.

// A failed logical turn must not leak pending user items into the
// next response.create. The client can resend the intended input as a
// fresh conversation item after correcting the request.

// Cancellation is a logical turn boundary even though the Responses
// compatibility transport does not forward this event upstream.
