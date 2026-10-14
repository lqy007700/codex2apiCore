package admin

import (
	"errors"

	"regexp"

	"time"
)

const (
	promptIntelligenceAIProviderReview    = "review"
	promptIntelligenceAIProviderPool      = "account_pool"
	promptIdentityUpdateModeSuggest       = "suggest"
	promptIdentityUpdateModeGuardedAuto   = "guarded_auto"
	promptIdentityManagedStart            = "[LEARNED SAFETY GUIDANCE — MANAGED]"
	promptIdentityManagedEnd              = "[/LEARNED SAFETY GUIDANCE — MANAGED]"
	promptIdentityAutoMinConfidence       = 0.95
	promptIdentityAutoMinUpstreamEvidence = 3
	promptIntelligenceAIResponseLimit     = 128 * 1024
	promptIntelligenceAIAnalysisTimeout   = 60 * time.Second
	promptIntelligenceAIAnalysisWorkers   = 3
)

var errPromptIntelligenceRequiresChatModel = errors.New("CY 归因需要支持 Chat Completions 的分析模型")

type promptIntelligenceAIAnalysisRequest struct {
	Provider           string `json:"provider"`
	Model              string `json:"model"`
	APIKeyID           int64  `json:"api_key_id"`
	IdentityUpdateMode string `json:"identity_update_mode"`
}

type promptIntelligenceAIRule struct {
	Name      string `json:"name"`
	Pattern   string `json:"pattern"`
	Weight    int    `json:"weight"`
	Category  string `json:"category"`
	Strict    bool   `json:"strict"`
	Rationale string `json:"rationale"`
}

type promptIntelligenceAIIdentityPatch struct {
	Clauses   []string `json:"clauses"`
	Rationale string   `json:"rationale"`
}

type promptIntelligenceAIDecision struct {
	Decision      string                             `json:"decision"`
	Confidence    float64                            `json:"confidence"`
	Reason        string                             `json:"reason"`
	Rule          *promptIntelligenceAIRule          `json:"rule,omitempty"`
	IdentityPatch *promptIntelligenceAIIdentityPatch `json:"identity_patch,omitempty"`
}

type promptIntelligenceAIAnalysisMetadata struct {
	Version                 int                          `json:"version"`
	Provider                string                       `json:"provider"`
	Model                   string                       `json:"model"`
	APIKeyID                int64                        `json:"api_key_id,omitempty"`
	APIKeyName              string                       `json:"api_key_name,omitempty"`
	ReviewSystemPromptHash  string                       `json:"review_system_prompt_hash"`
	UpstreamEvidenceCount   int                          `json:"upstream_evidence_count"`
	LearnableEvidenceCount  int                          `json:"learnable_evidence_count"`
	EvidenceBasis           string                       `json:"evidence_basis,omitempty"`
	Result                  promptIntelligenceAIDecision `json:"result"`
	RuleValidationError     string                       `json:"rule_validation_error,omitempty"`
	IdentityValidationError string                       `json:"identity_validation_error,omitempty"`
	RawOutputPreview        string                       `json:"raw_output_preview,omitempty"`
}

type promptIntelligenceLearningContext struct {
	Origin    string `json:"origin"`
	Role      string `json:"role,omitempty"`
	Text      string `json:"text"`
	Linked    bool   `json:"linked,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	Trust     string `json:"trust,omitempty"`
}

type promptIntelligenceLearningEvidence struct {
	Version       int                                 `json:"version"`
	Quality       string                              `json:"quality"`
	PromptText    string                              `json:"prompt_text,omitempty"`
	Context       []promptIntelligenceLearningContext `json:"context,omitempty"`
	UpstreamError string                              `json:"upstream_error,omitempty"`
	Transport     string                              `json:"transport,omitempty"`
	StatusCode    int                                 `json:"status_code,omitempty"`
	AttemptIndex  int                                 `json:"attempt_index,omitempty"`
	ReviewModel   string                              `json:"review_model,omitempty"`
	ReviewFlagged bool                                `json:"review_flagged,omitempty"`
	ReviewError   string                              `json:"review_error,omitempty"`
}

type promptIdentityUpdateResult struct {
	Mode               string   `json:"mode"`
	Suggested          bool     `json:"suggested"`
	Eligible           bool     `json:"eligible"`
	Applied            bool     `json:"applied"`
	RolledBack         bool     `json:"rolled_back,omitempty"`
	AnalysisEvidenceID int64    `json:"analysis_evidence_id"`
	RevisionEvidenceID int64    `json:"revision_evidence_id,omitempty"`
	Clauses            []string `json:"clauses,omitempty"`
	BlockReason        string   `json:"block_reason,omitempty"`
}

const (
	// promptIntelligenceEvidenceBasisPrompt 表示归因基于至少一条完整用户 Prompt；
	// promptIntelligenceEvidenceBasisContextOnly 表示只有会话 / 工具上下文证据。
	promptIntelligenceEvidenceBasisPrompt      = "prompt"
	promptIntelligenceEvidenceBasisContextOnly = "context_only"
)

type promptIntelligenceAIAnalysisResponse struct {
	AnalysisEvidenceID int64                        `json:"analysis_evidence_id"`
	Provider           string                       `json:"provider"`
	Model              string                       `json:"model"`
	EvidenceBasis      string                       `json:"evidence_basis"`
	Decision           promptIntelligenceAIDecision `json:"decision"`
	RuleCandidate      *promptIntelligenceCandidate `json:"rule_candidate,omitempty"`
	RuleError          string                       `json:"rule_error,omitempty"`
	IdentityUpdate     promptIdentityUpdateResult   `json:"identity_update"`
}

type promptIdentityRevisionMetadata struct {
	Version                int      `json:"version"`
	AnalysisEvidenceID     int64    `json:"analysis_evidence_id"`
	Provider               string   `json:"provider"`
	Model                  string   `json:"model"`
	Confidence             float64  `json:"confidence"`
	Reason                 string   `json:"reason"`
	Mode                   string   `json:"mode"`
	BasePromptHash         string   `json:"base_prompt_hash"`
	PreviousPromptHash     string   `json:"previous_prompt_hash"`
	AppliedPromptHash      string   `json:"applied_prompt_hash"`
	PreviousManagedClauses []string `json:"previous_managed_clauses"`
	AppliedManagedClauses  []string `json:"applied_managed_clauses"`
	RolledBackRevisionID   int64    `json:"rolled_back_revision_id,omitempty"`
}

type promptIntelligenceAICallAttribution struct {
	Provider   string
	Model      string
	APIKeyID   int64
	APIKeyName string
}

type promptIntelligenceCoverageSummary struct {
	EffectiveCoverage    string `json:"effective_coverage"`
	UpstreamEvidence     int    `json:"upstream_evidence_count"`
	LocalBlockCount      int    `json:"local_block_count"`
	LocalAllowCount      int    `json:"local_allow_count"`
	LocalWarnCount       int    `json:"local_warn_count"`
	LocalUnknownCount    int    `json:"local_unknown_count"`
	SignalOnlyMatchCount int    `json:"signal_only_match_count"`
}

// 没有完整用户 Prompt 的 CY（Codex 工具回合：当前用户消息为空，命中来自
// history / tool_output 层）同样允许归因：证据包里保存了 session_context /
// tool_arguments / tool_output 段，按 evidence_basis=context_only 标注后送模型。

// 受控自动应用的证据门槛按独立证据计数：完整 Prompt 与 context_only 上下文证据
// 都算（管理员已选择允许上下文证据触发自动模式）。

// context_only：没有当前用户 Prompt，命中来自 related_context（session_context /
// tool_arguments / tool_output 段）；模型需据此归因并明确说明依据是上下文。

// 采集侧已对这条证据给出质量裁决(包括 insufficient 隔离与
// context_only 的出处标注),预览回退仅适用于裁决产生之前的存量行:
// 把 insufficient 行升格为可学习、或把 context_only 的预览冒充直接
// Prompt,都会绕过采集侧的隔离语义。

// 受控自动应用的证据门槛按去重后的独立 Prompt 计数:同一条攻击文本被
// 重放多次只算一条,否则重复注入即可凑满门槛。

// countPromptIntelligenceAutoEligibleEvidence 是受控自动应用的证据计数：与
// countPromptIntelligenceDirectEvidence 一样按去重后的独立证据计数，但 context_only
// 证据也计入（以其上下文文本指纹去重）；只有 insufficient 被排除。

type promptIntelligenceAIReviewCallResult struct {
	output string
	err    error
}

var (
	promptIdentityForbiddenClause = regexp.MustCompile(`(?i)(sk-[a-z0-9]|authorization\s*:|bearer\s+|cookie\s*:|api[ _-]?key|private[ _-]?key|system\s+prompt|ignore\s+(all\s+)?(previous|prior|system)|override\s+(the\s+)?(instructions|policy)|change\s+(the\s+)?(json|output)|tool\s*call|https?://|<\/?user_input>|\[/?learned safety guidance|系統提示詞|系统提示词|忽略.{0,12}指令|更改.{0,12}輸出|更改.{0,12}输出|api.?密[鑰钥]|訪問令牌|访问令牌|工具調用|工具调用)`) //nolint:lll
	// 安全领域词：条款必须落在内容安全分类范围内（漏洞 / 攻击 / 滥用 / 凭据 / 恶意软件 …），
	// 否则就是通用行为指令。中英文都覆盖常见写法。
	promptIdentityDomainSignal = regexp.MustCompile(`(?i)(rce|exploit|malware|credential|unauthori[sz]ed|account abuse|phishing|ransomware|reverse shell|deepfake|doxx|credible threat|cyber|own system|authorized|defensive|abuse|injection|privilege|backdoor|intrusion|pentest|vulnerab|security|漏洞|攻擊|攻击|惡意軟體|恶意软件|憑據|凭据|未授權|未授权|批量帳號|批量账号|釣魚|钓鱼|勒索|反彈.?shell|反弹.?shell|深度偽造|深度伪造|人肉|暴力威脅|暴力威胁|自有系統|自有系统|防禦|防御|濫用|滥用|入侵|滲透|渗透|木馬|木马|病毒|提權|提权|注入|越權|越权|掃描|扫描|後門|后门|安全)`) //nolint:lll
	// 判定词：条款必须给出明确的分类结论（视为 / 不视为 / 属于正常 / 不按 … 处理 / 误报 …），
	// 而不是泛泛的行为要求。中文表达多样，这里按"结论动词 + 否定形式"两类覆盖。
	promptIdentityDecisionSignal = regexp.MustCompile(`(?i)(harmful|high.?risk|block|flag|classif|consider|treat|benign|safe|allow|legitimate|false.?positive|escalat|constitute|\bis\s+not\b|\bare\s+not\b|\bnot\s+(an?\s+)?(attack|abuse|exploit|malware|threat|violation|misuse)|does\s+not\s+(count|qualify|indicate|amount)|counts?\s+as|qualif(y|ies)\s+as|amounts?\s+to|no\s+weight|carry\s+no\s+weight|shall\s+not|must\s+not|should\s+not|normal\s+development|actual\s+(attack|abuse)|concrete\s+(attack|abuse)|unless.{0,80}(intent|attack|abuse)|without.{0,80}(intent|attack|abuse)|違規|违规|高風險|高风险|攔截|拦截|標記|标记|判定|視為|视为|視作|视作|合規|合规|放行|誤報|误报|不構成|不构成|不算|不是|不屬於|不属于|算作|當作|当作|歸為|归为|認定|认定|計入|计入|升級為|升级为|上升為|上升为|屬於.{0,12}(正常|合法|合規|合规|違規|违规|高風險|高风险|攻擊|攻击|濫用|滥用)|正常(開發|开发|文件|檔案|運維|运维|操作|使用|處理|处理|開發行為|开发行为)|不按.{0,20}(處理|处理)|不(應|应|應當|应当|得|能|要|可)(上升|升級|升级|視為|视为|視作|视作|判定|算作|當作|当作|歸為|归为|認定|认定|計入|计入|攔截|拦截|標記|标记|按.{0,12}處理|按.{0,12}处理)|不(予|做|作)(攔截|拦截|標記|标记|處理|处理)|(需|應|应|應當|应当|必須|必须)(攔截|拦截|標記|标记|阻斷|阻断|拒絕|拒绝)|不計.{0,8}(權重|权重)|除非.{0,30}(攻擊|攻击|濫用|滥用|意圖|意图)|沒有.{0,30}(攻擊|攻击|濫用|滥用|意圖|意图))`) //nolint:lll
)
