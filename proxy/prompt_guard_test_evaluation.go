package proxy

import (
	"github.com/codex2api/security/promptfilter"
)

// PromptGuardTestEvaluation is the non-persisting result returned to
// administrative rule testing. Request metadata is included so callers can
// verify that endpoint and model selection reached the same policy profile
// resolver used by live V1 requests.
type PromptGuardTestEvaluation struct {
	Verdict  promptfilter.Verdict     `json:"verdict"`
	Decision promptfilter.Decision    `json:"decision"`
	Protocol promptfilter.Protocol    `json:"protocol"`
	Provider promptfilter.ModelFamily `json:"provider"`
	Endpoint string                   `json:"endpoint"`
	Model    string                   `json:"model"`
}

// EvaluatePromptGuardTextForTest evaluates one direct current-user prompt
// through the real GuardPipeline decision path without sending an upstream
// model generation request or persisting local operational state. The evaluation intentionally
// disables session correlation, cumulative risk and signed NewAPI identity;
// it also uses an isolated Handler without a database or runtime cache so it
// cannot write logs, extension breaker state, penalties or sidecar cache data.
// Configured semantic sidecar and secondary review calls remain available so
// the returned action reflects the final live decision as closely as possible;
// those external detector calls may consume their configured service quota.

// Administrative probes must not warm the process-wide exact-segment cache.
// The cache is behavior-preserving, but keeping the probe fully isolated
// makes repeated tests unable to alter live-request cache residency.

// Never reuse the live handler's cache or database. evaluatePromptGuardEnvelope
// itself is the production decision path, while this isolated handler makes
// the administrative probe observational only.
