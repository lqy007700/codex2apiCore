package admin

// 「AI 生成规则草案」：基于候选的上游 CY 证据让模型写出一条窄正则规则草案，
// 只返回给前端预填表单，不落库；管理员审核后再走原有的保存草案 → 发布流程。
// 校验沿用 validatePromptIntelligenceAIRule，不通过的草案也照样返回并附上原因，
// 由人决定是否修改后保存。

type promptIntelligenceDraftSuggestRequest struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	APIKeyID int64  `json:"api_key_id"`
}

type promptIntelligenceDraftSuggestResponse struct {
	Provider        string                   `json:"provider"`
	Model           string                   `json:"model"`
	EvidenceBasis   string                   `json:"evidence_basis"`
	Confidence      float64                  `json:"confidence"`
	Reason          string                   `json:"reason"`
	Rule            promptIntelligenceAIRule `json:"rule"`
	ValidationError string                   `json:"validation_error,omitempty"`
	// EvidenceMatched / EvidenceTotal：草案正则在本候选可学习证据上的命中数，
	// 0 命中说明规则没有抓住它所依据的行为，前端提示但不阻止人工修改后保存。
	EvidenceMatched int `json:"evidence_matched"`
	EvidenceTotal   int `json:"evidence_total"`
}

// SuggestPromptIntelligenceCandidateDraft POST /api/admin/prompt-filter/intelligence/candidates/:id/draft/suggest

// 校验口径与人工保存草案一致（validateIntelligenceCandidate：权重范围 + 正则安全审计），
// 不套用自动入库的"完整匹配内置样句"门槛——那条门槛会拒绝所有针对具体行为的窄正则。

// buildPromptIntelligenceAIRuleDraftIdentity 在 Review 身份上叠加"只产出规则草案"的任务扩展。

// countPromptIntelligenceDraftEvidenceMatches 用草案正则（按网关的大小写不敏感方式编译）
// 逐条测试可学习证据的 Prompt 文本或上下文段，返回命中数与总数。
