package admin

import (
	"errors"

	"sync"
	"time"
)

var defaultIntelligenceQueries = []string{
	"LLM jailbreak prompt injection",
	"大模型 破限 提示词",
	"GPT 破甲 提示词",
	"AI 越狱 提示词",
	"中文 prompt injection 绕过",
	"ChatGPT jailbreak prompt",
	"Codex prompt injection jailbreak",
}

var githubPromptSearchBaseURL = "https://api.github.com/search/repositories"

const (
	promptRuleRuntimeSyncInterval       = 5 * time.Second
	legacyIntelligenceMigrationInterval = time.Minute
)

type promptIntelligenceSource struct {
	Provider    string `json:"provider"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	UpdatedAt   string `json:"updated_at"`
}

type promptIntelligenceCandidate struct {
	ID               int64                                 `json:"id,omitempty"`
	Fingerprint      string                                `json:"fingerprint,omitempty"`
	Kind             string                                `json:"kind,omitempty"`
	Name             string                                `json:"name"`
	Pattern          string                                `json:"pattern"`
	Weight           int                                   `json:"weight"`
	Category         string                                `json:"category"`
	Strict           bool                                  `json:"strict"`
	Rationale        string                                `json:"rationale,omitempty"`
	SourceURL        string                                `json:"source_url,omitempty"`
	ChangeType       string                                `json:"change_type,omitempty"`
	LifecycleStatus  string                                `json:"lifecycle_status,omitempty"`
	Source           string                                `json:"source,omitempty"`
	EvidenceCount    int                                   `json:"evidence_count,omitempty"`
	SamplePreview    string                                `json:"sample_preview,omitempty"`
	Protocol         string                                `json:"protocol,omitempty"`
	Provider         string                                `json:"provider,omitempty"`
	Model            string                                `json:"model,omitempty"`
	APIKeyID         int64                                 `json:"api_key_id,omitempty"`
	APIKeyName       string                                `json:"api_key_name,omitempty"`
	AIAnalyzed       bool                                  `json:"ai_analyzed,omitempty"`
	AIAnalysisCount  int                                   `json:"ai_analysis_count,omitempty"`
	AIAnalyzedAt     *time.Time                            `json:"ai_analyzed_at,omitempty"`
	LatestAIAnalysis *promptIntelligenceAIAnalysisResponse `json:"latest_ai_analysis,omitempty"`
	CreatedAt        *time.Time                            `json:"created_at,omitempty"`
	UpdatedAt        *time.Time                            `json:"updated_at,omitempty"`
	LastSeenAt       *time.Time                            `json:"last_seen_at,omitempty"`
}

type promptIntelligenceRun struct {
	StartedAt  time.Time                     `json:"started_at"`
	FinishedAt time.Time                     `json:"finished_at"`
	Queries    []string                      `json:"queries"`
	Sources    []promptIntelligenceSource    `json:"sources"`
	Candidates []promptIntelligenceCandidate `json:"candidates"`
	ModelCalls int                           `json:"model_calls"`
	Staged     int                           `json:"staged"`
	Errors     []string                      `json:"errors"`
}

type promptIntelligenceHistoryResponse struct {
	Runs  []*promptIntelligenceRun `json:"runs"`
	Total int                      `json:"total"`
}

type promptIntelligenceCandidatesResponse struct {
	Candidates []promptIntelligenceCandidate `json:"candidates"`
	Total      int                           `json:"total"`
}

type promptIntelligenceCandidateDraftRequest struct {
	Name      string `json:"name"`
	Pattern   string `json:"pattern"`
	Weight    int    `json:"weight"`
	Category  string `json:"category"`
	Strict    bool   `json:"strict"`
	Rationale string `json:"rationale"`
}

var promptIntelligenceRunMu sync.Mutex

var (
	errPromptIntelligenceCandidateNotFound = errors.New("prompt intelligence candidate not found")
	errPromptIntelligenceCandidateInvalid  = errors.New("prompt intelligence candidate invalid")
)

// startPromptRuleRuntimeSync keeps the one runtime rule engine consistent
// across replicas. Candidate/evidence rows are never read here; only the
// explicitly published custom-pattern snapshot in system_settings is synced.

// 每条上游 CY 证据都挂着一个 CY 记录；把该 CY 的画像主体（人员 / 会话 / Key / IP /
// 上游账号）一并带回，审核证据时能直接看到是哪个用户并跳到画像。

// promptIntelligenceProposalEquivalent compares rule behavior while treating
// an omitted enabled field and an explicit true value as the same active rule.
// This keeps serialization-only differences out of the review queue without
// hiding changes to signal-only or composite matching behavior.

// Auto-added intelligence must describe a complete high-risk behavior, not
// merely a short substring that happens to occur inside a risk sentence.
// This prevents candidates such as `[sS][tT][rR]` from being admitted only
// because "instructions" contains "str".

// migrateLegacyAutomaticIntelligenceRules removes only rules that can be
// proven to have been written by the historical unattended intelligence path.
// Manually created signal-only rules are intentionally retained.
