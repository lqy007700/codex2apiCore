package admin

import (
	"context"
	"encoding/json"
	"fmt"

	"regexp"
	"slices"

	"strings"

	"github.com/codex2api/auth"
	"github.com/codex2api/proxy"
)

const qualityTestPromptLimit = 16000

// A separate document avoids inheriting the admin page's script-src policy (as
// srcdoc/blob documents do). Do not loosen the admin CSP or grant same-origin.

type qualityTestRequest struct {
	Model           string `json:"model"`
	Prompt          string `json:"prompt"`
	ReasoningEffort string `json:"reasoning_effort"`
	// PromptID references the custom preset the prompt came from; PresetKey names a
	// built-in one. PresetName is the client's display name, kept as a fallback only.
	PromptID   int64  `json:"prompt_id,omitempty"`
	PresetKey  string `json:"preset_key,omitempty"`
	PresetName string `json:"preset_name,omitempty"`
	// TimeoutMinutes is the run's hard limit; 0 selects the default.
	TimeoutMinutes int `json:"timeout_minutes,omitempty"`
}

var builtinPresetKey = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)

// resolveQualityTestPreset snapshots the preset a run came from. Custom presets
// take their name from the database; a vanished preset degrades to "hand-typed".

type qualityTestOptions struct {
	Models           []string `json:"models"`
	ReasoningEfforts []string `json:"reasoning_efforts"`
}

// QualityTest runs one administrator-selected account, without scheduler fallback.
// It shares the connection test's transport, cancellation, diagnostics and quota handling.

func (h *Handler) qualityTestOptionsForAccount(ctx context.Context, account *auth.Account) qualityTestOptions {
	options := qualityTestOptions{Models: []string{}, ReasoningEfforts: []string{"", "none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra"}}
	var candidates []string
	switch {
	case account.IsClaudeOAuth():
		candidates = claudeProbeModelIDs(account)
		options.ReasoningEfforts = []string{"", "low", "medium", "high", "max"}
	case account.IsAntigravityAPI():
		candidates = antigravityConnectionTestModels(account)
		// Public Antigravity model IDs already encode a fixed thinking tier.
		options.ReasoningEfforts = []string{""}
	case account.IsRelayStyle():
		candidates = account.OpenAIResponsesModels()
		if account.IsGrokAPI() {
			options.ReasoningEfforts = []string{"", "low", "medium", "high"}
			if len(candidates) == 0 {
				candidates = defaultGrokConnectionTestModels(account)
			}
		}
	default:
		candidates = account.CodexModels()
		if len(candidates) == 0 {
			candidates = proxy.TextTestModelIDs(ctx, h.db)
		}
	}
	for _, model := range candidates {
		model = strings.TrimSpace(model)
		// Effort aliases are proxy routing shortcuts, not upstream model IDs.
		if strings.EqualFold(model, "codex-auto-review") || !isTextConnectionModel(model) || strings.ContainsAny(model, "()") || slices.Contains(options.Models, model) {
			continue
		}
		options.Models = append(options.Models, model)
	}
	return options
}

func (h *Handler) validateQualityTestForAccount(ctx context.Context, account *auth.Account, req qualityTestRequest) error {
	options := h.qualityTestOptionsForAccount(ctx, account)
	if !slices.Contains(options.Models, req.Model) {
		return fmt.Errorf("该账号不支持测试模型: %s", req.Model)
	}
	if !slices.Contains(options.ReasoningEfforts, req.ReasoningEffort) {
		return fmt.Errorf("该渠道不支持所选思考强度")
	}
	return nil
}

func buildQualityTestPayload(account *auth.Account, model string, req qualityTestRequest, securityCfg auth.ClaudeSecurityConfig) ([]byte, error) {
	const instructions = "Return a complete, self-contained HTML document for the user's request. Include all SVG, CSS and JavaScript inline. Do not use external resources. Return only HTML, without Markdown fences or explanations."
	body := map[string]any{"model": model, "stream": true}
	if account.IsClaudeOAuth() {
		maxTokens := int64(32768)
		if securityCfg.MaxOutputTokens > 0 && securityCfg.MaxOutputTokens < maxTokens {
			maxTokens = securityCfg.MaxOutputTokens
		}
		body["max_tokens"] = maxTokens
		body["system"] = instructions
		body["messages"] = []map[string]any{{"role": "user", "content": req.Prompt}}
		if req.ReasoningEffort != "" {
			body["thinking"] = map[string]any{"type": "adaptive"}
			body["output_config"] = map[string]any{"effort": req.ReasoningEffort}
		}
	} else {
		body["store"] = false
		body["instructions"] = instructions
		body["input"] = []map[string]any{{"role": "user", "content": []map[string]any{{"type": "input_text", "text": req.Prompt}}}}
		if req.ReasoningEffort != "" {
			body["reasoning"] = map[string]any{"effort": req.ReasoningEffort}
		}
	}
	return json.Marshal(body)
}
