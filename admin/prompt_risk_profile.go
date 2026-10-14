package admin

import (
	"time"

	"github.com/codex2api/database"
)

const promptRiskHistoryGuardrail = "画像只统计本地 warn/block 与上游 CY；影子审计和普通命中不再抬高风险。画像不会单独封禁当前请求，只控制可自动失效的模型复核豁免；达到阈值或再次出现 CY 时立即恢复同步审核。"

const promptRiskProfileListTimeout = 20 * time.Second

type promptRiskProfilesResponse struct {
	Profiles       []*database.PromptRiskProfile `json:"profiles"`
	Total          int                           `json:"total"`
	Page           int                           `json:"page"`
	PageSize       int                           `json:"page_size"`
	ScoringVersion string                        `json:"scoring_version"`
	Guardrail      string                        `json:"guardrail"`
}

type promptRiskProfileDetailResponse struct {
	Profile             *database.PromptRiskProfile           `json:"profile"`
	Events              []*database.PromptRiskEvent           `json:"events"`
	TrustEvents         []*database.PromptRiskTrustEvent      `json:"trust_events"`
	AdaptiveReviewBasis promptRiskAdaptiveReviewBasisResponse `json:"adaptive_review_basis"`
	EventTotal          int                                   `json:"event_total"`
	EventPage           int                                   `json:"event_page"`
	EventPageSize       int                                   `json:"event_page_size"`
	TrustEventTotal     int                                   `json:"trust_event_total"`
	TrustEventPage      int                                   `json:"trust_event_page"`
	TrustEventPageSize  int                                   `json:"trust_event_page_size"`
	ScoringVersion      string                                `json:"scoring_version"`
	Guardrail           string                                `json:"guardrail"`
}

type promptRiskAdaptiveReviewBasisResponse struct {
	Enabled                    bool       `json:"enabled"`
	ReviewEnabled              bool       `json:"review_enabled"`
	Eligible                   bool       `json:"eligible"`
	Decision                   string     `json:"decision"`
	CleanReviewCount           int        `json:"clean_review_count"`
	PositiveEvidenceCount      int        `json:"positive_evidence_count"`
	MinCleanReviews            int        `json:"min_clean_reviews"`
	MinObservationHours        int        `json:"min_observation_hours"`
	ObservationHours           int        `json:"observation_hours"`
	SamplePercent              int        `json:"sample_percent"`
	ForceReviewIntervalMinutes int        `json:"force_review_interval_minutes"`
	TrustDurationHours         int        `json:"trust_duration_hours"`
	RiskThreshold              int        `json:"risk_threshold"`
	FirstCleanAt               *time.Time `json:"first_clean_at,omitempty"`
	LastCleanAt                *time.Time `json:"last_clean_at,omitempty"`
	NextForcedReviewAt         *time.Time `json:"next_forced_review_at,omitempty"`
	ForceReviewDue             bool       `json:"force_review_due"`
}

// 生产画像列表需要聚合近 30 天事件。高流量实例可能包含数十万条记录，
// 5 秒会在 SQLite 正常计算完成前主动取消，表现为稳定的 500。
