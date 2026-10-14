package admin

import (
	"encoding/json"

	"time"

	"github.com/codex2api/database"

	"github.com/codex2api/security/promptfilter"
)

const promptFilterAuditContextMaxRunes = 2000

type promptFilterLogsResponse struct {
	Logs     []*database.PromptFilterLog `json:"logs"`
	Total    int                         `json:"total"`
	Page     int                         `json:"page"`
	PageSize int                         `json:"page_size"`
}

type promptPolicyIncidentsResponse struct {
	Incidents []*database.PromptPolicyIncident `json:"incidents"`
	Total     int                              `json:"total"`
	Page      int                              `json:"page"`
	PageSize  int                              `json:"page_size"`
}

type promptPolicyIncidentDetailResponse struct {
	Incident  *database.PromptPolicyIncident        `json:"incident"`
	Matches   json.RawMessage                       `json:"matches"`
	Candidate *database.PromptRuleCandidate         `json:"candidate,omitempty"`
	Evidence  *database.PromptRuleCandidateEvidence `json:"evidence,omitempty"`
	// RiskSubjects 是该 CY 关联的画像主体（人员 / 会话 / Key / IP / 上游账号），
	// 让归因时能直接看到并跳到对应的人员画像。
	RiskSubjects []database.PromptRiskIncidentSubject `json:"risk_subjects"`
}

type promptPolicyAuditQueueHealth struct {
	Enqueued      uint64 `json:"enqueued"`
	Completed     uint64 `json:"completed"`
	DroppedHigh   uint64 `json:"dropped_high"`
	DroppedLow    uint64 `json:"dropped_low"`
	Failed        uint64 `json:"failed"`
	PendingHigh   int    `json:"pending_high"`
	PendingLow    int    `json:"pending_low"`
	RetainedBytes int64  `json:"retained_bytes"`
}

type promptPolicyAuditHealthResponse struct {
	OK                      bool                             `json:"ok"`
	Status                  string                           `json:"status"`
	StorageReady            bool                             `json:"storage_ready"`
	PromptFilterEnabled     bool                             `json:"prompt_filter_enabled"`
	ReviewEnabled           bool                             `json:"review_enabled"`
	ReviewFailClosed        bool                             `json:"review_fail_closed"`
	ReviewPool              promptfilter.ReviewKeyPoolHealth `json:"review_pool"`
	ConversationLockEnabled bool                             `json:"conversation_lock_enabled"`
	IncidentCount           int                              `json:"incident_count"`
	LatestIncidentID        string                           `json:"latest_incident_id,omitempty"`
	LatestIncidentAt        *time.Time                       `json:"latest_incident_at,omitempty"`
	Queue                   promptPolicyAuditQueueHealth     `json:"queue"`
}

type promptFilterTestRequest struct {
	Text     string `json:"text"`
	Endpoint string `json:"endpoint"`
	Model    string `json:"model"`
}

type promptFilterTestResponse struct {
	Verdict  promptfilter.Verdict     `json:"verdict"`
	Decision promptfilter.Decision    `json:"decision"`
	Protocol promptfilter.Protocol    `json:"protocol"`
	Provider promptfilter.ModelFamily `json:"provider"`
	Endpoint string                   `json:"endpoint"`
	Model    string                   `json:"model"`
}

type promptReviewTestRequest struct {
	Text                 string             `json:"text"`
	APIKey               string             `json:"api_key"`
	BaseURL              string             `json:"base_url"`
	Model                string             `json:"model"`
	RequestMode          string             `json:"request_mode"`
	SystemPrompt         string             `json:"system_prompt"`
	UserPromptTemplate   string             `json:"user_prompt_template"`
	PayloadTemplate      string             `json:"payload_template"`
	ConfidenceThreshold  float64            `json:"confidence_threshold"`
	ModerationThresholds map[string]float64 `json:"moderation_thresholds"`
	TimeoutSeconds       int                `json:"timeout_seconds"`
	MaxConcurrent        int                `json:"max_concurrent"`
	MaxTextLength        int                `json:"max_text_length"`
	TestAllKeys          bool               `json:"test_all_keys"`
}

type promptReviewKeyTestResult struct {
	KeyIndex             int                `json:"key_index"`
	KeyID                string             `json:"key_id,omitempty"`
	KeyMasked            string             `json:"key_masked,omitempty"`
	OK                   bool               `json:"ok"`
	Endpoint             string             `json:"endpoint,omitempty"`
	Model                string             `json:"model,omitempty"`
	Flagged              bool               `json:"flagged"`
	Confidence           float64            `json:"confidence"`
	Reason               string             `json:"reason,omitempty"`
	HighestCategory      string             `json:"highest_category,omitempty"`
	DecisionCategory     string             `json:"decision_category,omitempty"`
	DecisionScore        float64            `json:"decision_score"`
	DecisionThreshold    float64            `json:"decision_threshold"`
	CategoryScores       map[string]float64 `json:"category_scores,omitempty"`
	ModerationThresholds map[string]float64 `json:"moderation_thresholds,omitempty"`
	LatencyMS            int64              `json:"latency_ms"`
	Error                string             `json:"error,omitempty"`
}

type promptReviewAPIKeyDescriptor struct {
	ID     string `json:"id"`
	Index  int    `json:"index"`
	Masked string `json:"masked"`
}

type promptReviewAPIKeysResponse struct {
	Items []promptReviewAPIKeyDescriptor `json:"items"`
	Count int                            `json:"count"`
}

type promptReviewProfileResponse struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	BaseURL       string    `json:"base_url"`
	Model         string    `json:"model"`
	RequestMode   string    `json:"request_mode"`
	TimeoutSecond int       `json:"timeout_seconds"`
	KeyCount      int       `json:"key_count"`
	Active        bool      `json:"active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type promptReviewProfilesResponse struct {
	Profiles []promptReviewProfileResponse `json:"profiles"`
}

type promptReviewProfileRequest struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	BaseURL       string `json:"base_url"`
	Model         string `json:"model"`
	RequestMode   string `json:"request_mode"`
	APIKey        string `json:"api_key"`
	AdapterJSON   string `json:"adapter_json"`
	TimeoutSecond int    `json:"timeout_seconds"`
}

// 留空表示保存当前已生效的 Key，避免用户为了保存新档案而
// 必须再次粘贴现有 DeepSeek Key。

// maskPromptReviewAPIKey 按 rune 切片(字节切会截断多字节字符),且只在
// 前后缀不可能重叠、多数字符仍被遮蔽时(≥12 字符)才展示明文片段。

type promptReviewTestResponse struct {
	OK                   bool                        `json:"ok"`
	Endpoint             string                      `json:"endpoint"`
	Model                string                      `json:"model"`
	Flagged              bool                        `json:"flagged"`
	Confidence           float64                     `json:"confidence"`
	ConfidenceThreshold  float64                     `json:"confidence_threshold"`
	Reason               string                      `json:"reason,omitempty"`
	HighestCategory      string                      `json:"highest_category,omitempty"`
	DecisionCategory     string                      `json:"decision_category,omitempty"`
	DecisionScore        float64                     `json:"decision_score"`
	DecisionThreshold    float64                     `json:"decision_threshold"`
	CategoryScores       map[string]float64          `json:"category_scores,omitempty"`
	ModerationThresholds map[string]float64          `json:"moderation_thresholds,omitempty"`
	LatencyMS            int64                       `json:"latency_ms"`
	KeyCount             int                         `json:"key_count,omitempty"`
	Results              []promptReviewKeyTestResult `json:"results,omitempty"`
}

type promptFilterRulePatternTestRequest struct {
	Pattern string `json:"pattern"`
	Text    string `json:"text"`
}

type promptFilterRulePatternTestResponse struct {
	Matched bool   `json:"matched"`
	Error   string `json:"error,omitempty"`
}

type promptFilterRuleItem struct {
	Name     string `json:"name"`
	Pattern  string `json:"pattern"`
	Weight   int    `json:"weight"`
	Category string `json:"category,omitempty"`
	Strict   bool   `json:"strict,omitempty"`
	Enabled  bool   `json:"enabled"`
	Builtin  bool   `json:"builtin"`
}

type promptFilterRulesResponse struct {
	BuiltinPatterns  []promptFilterRuleItem       `json:"builtin_patterns"`
	CustomPatterns   []promptfilter.PatternConfig `json:"custom_patterns"`
	DisabledPatterns []string                     `json:"disabled_patterns"`
}

// 手动清空改为后台分批删除：几十万行日志在一条 DELETE + 10s 请求超时下清不完，
// 现在立即返回并在后台以 5000 行/批清理；与 CY 关联的日志一律跳过。

// 主动关闭的功能不算故障（面板上已有独立的开关状态展示）；只有真实异常才降级：
// 审计队列丢高优/写入失败，或审查已启用但 key 池空配置/全冷却。

// MatchPromptFilterLog 按时间/端点/APIKey 找到与某次请求最接近的一条提示词过滤日志，
// 用于「使用统计」里点击 cyber_policy 报错时查看触发的完整请求内容。
// GET /api/prompt-filter/logs/match?at=<RFC3339>&endpoint=&api_key_id=&source=

type promptReviewModelsRequest struct {
	BaseURL        string `json:"base_url"`
	APIKey         string `json:"api_key"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

// ListPromptReviewModels 从审查供应商拉取 OpenAI 兼容的模型列表,供前端选择审查模型。
// base_url/api_key 允许携带表单草稿值覆盖已存配置,便于保存前先拉列表。

// storedRaw 保持数据库原值不做 trim:CAS 按原值精确比较,存量值若带
// 换行/制表符,trim 后将永远匹配不上而恒 409。
