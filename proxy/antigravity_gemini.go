package proxy

import (
	"bytes"

	"encoding/json"
	"errors"
	"fmt"
	"io"

	"strings"
)

// ExecuteAntigravityGeminiRequest forwards a native Gemini generateContent request
// through the Cloud Code v1internal Antigravity adapter and unwraps the upstream
// envelope back into Gemini API shape.

// ExecuteAntigravityGeminiCountTokensRequest forwards a native Gemini countTokens
// request through Cloud Code v1internal:countTokens.

// Strip client scaffolding before the session id is derived from the
// contents, so the id stays stable across turns.

// antigravityNormalizeNativeGeminiResponseSchema 把 generationConfig.responseJsonSchema
// (及 snake_case 的 response_json_schema)归一成 responseSchema。Antigravity 后端不认
// 前者,原样透传时结构化输出会静默不生效。responseJsonSchema 是标准 JSON Schema,
// 可能带 $defs/$ref 与 responseSchema(OpenAPI 子集)不认的关键字,搬过去时套用与
// Responses→Antigravity 相同的清洗;客户端已显式给了 responseSchema 则以它为准,
// 只删掉过时键。

const antigravityGeminiSkipThoughtSignature = "skip_thought_signature_validator"

// antigravityNormalizeNativeGeminiContents neutralizes client scaffolding in
// native Gemini contents in place. Parts are rewritten rather than dropped so
// the envelope keeps the exact shape the caller sent; an emptied text part
// becomes an empty string, which is a shape this codebase already produces
// elsewhere.

type antigravityGeminiToolCallGroup struct {
	responsesNeeded int
	callNames       []string
}

func unwrapAntigravityNativeGeminiChunk(body []byte) []byte {
	var envelope map[string]any
	if json.Unmarshal(body, &envelope) != nil {
		return body
	}
	if response, ok := envelope["response"].(map[string]any); ok {
		out, err := json.Marshal(restoreAntigravityNativeGeminiUsage(response))
		if err != nil {
			return body
		}
		return out
	}
	out, err := json.Marshal(restoreAntigravityNativeGeminiUsage(envelope))
	if err != nil {
		return body
	}
	return out
}

func restoreAntigravityNativeGeminiUsage(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	if usage, ok := value["cpaUsageMetadata"].(map[string]any); ok {
		value["usageMetadata"] = usage
		delete(value, "cpaUsageMetadata")
	}
	return value
}

type antigravityNativeGeminiSSEBody struct {
	source         io.ReadCloser
	events         *antigravitySSEReader
	queue          bytes.Buffer
	reverseNameMap map[string]string
	terminalErr    error
	sawTerminal    bool
	// jsonMode reads every event through antigravityRewriteJSONModeFrame;
	// jsonModeToolCalled is its cross-event state.
	jsonMode           bool
	jsonModeToolCalled bool
}

func (b *antigravityNativeGeminiSSEBody) Close() error {
	if b == nil || b.source == nil {
		return nil
	}
	return b.source.Close()
}

func (b *antigravityNativeGeminiSSEBody) enqueue(chunk []byte) {
	if len(chunk) == 0 {
		return
	}
	b.queue.WriteString("data: ")
	b.queue.Write(chunk)
	b.queue.WriteString("\n\n")
}

func (b *antigravityNativeGeminiSSEBody) observe(payload map[string]any) {
	switch strings.ToUpper(geminiFinishReason(payload)) {
	case "", "NONE", "FINISH_REASON_UNSPECIFIED":
	default:
		b.sawTerminal = true
	}
	if feedback, ok := payload["promptFeedback"].(map[string]any); ok {
		reason, _ := feedback["blockReason"].(string)
		switch strings.ToUpper(strings.TrimSpace(reason)) {
		case "", "NONE", "BLOCK_REASON_UNSPECIFIED":
		default:
			b.sawTerminal = true
		}
	}
}

func (b *antigravityNativeGeminiSSEBody) endError(err error) error {
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if b.sawTerminal {
		return io.EOF
	}
	return fmt.Errorf("antigravity Gemini stream ended before a terminal response: %w", io.ErrUnexpectedEOF)
}

// The final event is drained before Read reports the end of the stream,
// including when the stream has no trailing blank line.
func (b *antigravityNativeGeminiSSEBody) readEvent() ([]byte, error) {
	return b.events.next()
}

func antigravityNativeGeminiStreamPayload(data []byte) (map[string]any, error) {
	payload, err := decodeAntigravityStreamEnvelope(data)
	if err != nil {
		return nil, fmt.Errorf("decode antigravity Gemini stream event: %w", err)
	}
	for {
		if upstreamError, ok := payload["error"]; ok {
			return nil, newAntigravityStreamError(upstreamError)
		}
		response, ok := payload["response"].(map[string]any)
		if !ok {
			return payload, nil
		}
		payload = response
	}
}

func (b *antigravityNativeGeminiSSEBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for b.queue.Len() == 0 {
		if b.terminalErr != nil {
			return 0, b.terminalErr
		}
		data, readErr := b.readEvent()
		trimmed := bytes.TrimSpace(data)
		if len(trimmed) == 0 {
			if readErr != nil {
				b.terminalErr = b.endError(readErr)
			}
			continue
		}
		if bytes.Equal(trimmed, []byte("[DONE]")) {
			b.terminalErr = b.endError(readErr)
			continue
		}
		if b.jsonMode {
			trimmed, b.jsonModeToolCalled = antigravityRewriteJSONModeFrame(trimmed, b.jsonModeToolCalled)
		}
		payload, err := antigravityNativeGeminiStreamPayload(trimmed)
		if err != nil {
			b.terminalErr = err
			if readErr != nil && !errors.Is(readErr, io.EOF) {
				b.terminalErr = readErr
			}
			continue
		}
		chunk := unwrapAntigravityNativeGeminiChunk(trimmed)
		chunk = antigravityRestoreNativeGeminiResponseNames(chunk, b.reverseNameMap)
		b.observe(payload)
		b.enqueue(chunk)
		if readErr != nil {
			b.terminalErr = b.endError(readErr)
		}
	}
	return b.queue.Read(p)
}
