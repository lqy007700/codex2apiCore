package proxy

import (
	"encoding/json"

	"regexp"

	"strings"
)

var antigravityFunctionNameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_.:-]`)

func antigravityRestoreGeminiFunctionName(reverse map[string]string, sanitized string) string {
	if sanitized == "" || reverse == nil {
		return sanitized
	}
	if original, ok := reverse[sanitized]; ok {
		return original
	}
	return sanitized
}

func antigravityRestoreNativeGeminiResponseNames(body []byte, reverse map[string]string) []byte {
	if len(reverse) == 0 || len(body) == 0 {
		return body
	}
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return body
	}
	candidates, ok := payload["candidates"].([]any)
	if !ok {
		return body
	}
	for candidateIndex, rawCandidate := range candidates {
		candidate, ok := rawCandidate.(map[string]any)
		if !ok {
			continue
		}
		content, ok := candidate["content"].(map[string]any)
		if !ok {
			continue
		}
		parts, ok := content["parts"].([]any)
		if !ok {
			continue
		}
		for partIndex, rawPart := range parts {
			part, ok := rawPart.(map[string]any)
			if !ok {
				continue
			}
			for _, field := range []string{"functionCall", "function_call", "functionResponse", "function_response"} {
				nested, ok := part[field].(map[string]any)
				if !ok {
					continue
				}
				if name, _ := nested["name"].(string); strings.TrimSpace(name) != "" {
					nested["name"] = antigravityRestoreGeminiFunctionName(reverse, name)
				}
			}
			parts[partIndex] = part
		}
		content["parts"] = parts
		candidate["content"] = content
		candidates[candidateIndex] = candidate
	}
	payload["candidates"] = candidates
	out, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return out
}
