package proxy

import (
	"testing"

	"github.com/codex2api/security/promptfilter"
)

func TestTerminalModelBypassRequiresCleanReviewAndRequestedModelMatch(t *testing.T) {
	cfg := promptfilter.DefaultConfig()
	cfg.Advanced.Enforcement.TerminalBypassModels = []string{"codex-auto-review", "custom-audit"}
	base := promptfilter.Verdict{
		Action:            promptfilter.ActionBlock,
		Reviewed:          true,
		TerminalStrictHit: true,
	}

	clean, bypassed := applyPromptGuardTerminalModelBypass(base, promptfilter.RequestEnvelope{RequestedModel: "CODEX-AUTO-REVIEW", EffectiveModel: "gpt-5.6-sol"}, cfg)
	if !bypassed || clean.Action != promptfilter.ActionAllow || clean.TerminalStrictHit || clean.TerminalCategoryHit {
		t.Fatalf("clean exempt review was not allowed: bypassed=%t verdict=%+v", bypassed, clean)
	}

	for name, verdict := range map[string]promptfilter.Verdict{
		"flagged":      func() promptfilter.Verdict { v := base; v.ReviewFlagged = true; return v }(),
		"error":        func() promptfilter.Verdict { v := base; v.ReviewError = "review unavailable"; return v }(),
		"not_reviewed": func() promptfilter.Verdict { v := base; v.Reviewed = false; return v }(),
	} {
		t.Run(name, func(t *testing.T) {
			got, bypassed := applyPromptGuardTerminalModelBypass(verdict, promptfilter.RequestEnvelope{RequestedModel: "codex-auto-review"}, cfg)
			if bypassed || got.Action != promptfilter.ActionBlock || !got.TerminalStrictHit {
				t.Fatalf("unsafe review state bypassed terminal block: bypassed=%t verdict=%+v", bypassed, got)
			}
		})
	}

	mappedOnly, bypassed := applyPromptGuardTerminalModelBypass(base, promptfilter.RequestEnvelope{RequestedModel: "gpt-5.6-sol", EffectiveModel: "codex-auto-review"}, cfg)
	if bypassed || mappedOnly.Action != promptfilter.ActionBlock {
		t.Fatalf("effective-model alias bypassed requested-model boundary: bypassed=%t verdict=%+v", bypassed, mappedOnly)
	}
}

func TestFinalizePromptGuardDecisionRetainsTerminalLocalBlockAfterReviewClear(t *testing.T) {
	decision := promptfilter.Decision{
		Enabled:        true,
		Mode:           promptfilter.GuardModeEnforce,
		Action:         promptfilter.ActionBlock,
		Terminal:       true,
		StrikeEligible: true,
		PrimaryOrigin:  promptfilter.OriginCurrentUser,
	}
	verdict := promptfilter.Verdict{
		Action:            promptfilter.ActionBlock,
		Reviewed:          true,
		ReviewFlagged:     false,
		TerminalStrictHit: true,
		SensitiveIntent:   true,
	}

	got := finalizePromptGuardDecision(decision, verdict)
	if got.Action != promptfilter.ActionBlock || !got.Terminal || !got.StrikeEligible {
		t.Fatalf("terminal local block lost after review clear: %+v", got)
	}
}
