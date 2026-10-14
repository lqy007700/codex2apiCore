package proxy

import (
	"io"
)

const antigravityNativeGeminiThoughtEvent = `{"response":{"candidates":[{"content":{"parts":[{"text":"thinking","thought":true,"thoughtSignature":"signature"}]}}]}}`
const antigravityNativeGeminiStopEvent = `{"response":{"candidates":[{"content":{"parts":[{"text":"answer"}]},"finishReason":"STOP"}]}}`

// Return the error only once, together with the last bytes. A reader must not
// replace it with the EOF from the next Read or discard the accompanying event.
type antigravityNativeGeminiFinalErrorReader struct {
	data []byte
	err  error
}

func (r *antigravityNativeGeminiFinalErrorReader) Read(p []byte) (int, error) {
	if len(r.data) > 0 {
		n := copy(p, r.data)
		r.data = r.data[n:]
		if len(r.data) > 0 {
			return n, nil
		}
		err := r.err
		r.err = nil
		return n, err
	}
	if r.err != nil {
		err := r.err
		r.err = nil
		return 0, err
	}
	return 0, io.EOF
}

// Small reads also ensure a pending error cannot hide queued bytes.
