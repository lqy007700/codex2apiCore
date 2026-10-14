package admin

// TestPromptIntelligenceLearningEvidenceQualityGate 验证预览回退只服务于
// 无质量裁决的存量行:insufficient 隔离与 context_only 的出处标注不可被
// SamplePreview 升格绕过。

// TestCountPromptIntelligenceDirectEvidenceDeduplicatesReplays 验证受控自动
// 应用的证据门槛按独立 Prompt 指纹计数,重放同一文本不能凑数。

// 同一段上下文重放 → 只算一条

// 线上 AI 归因（context_only CY）实际给出的两条建议，之前被误判为"通用行为指令"。

// 第二次线上归因给出的英文否定式

// 无安全领域词
// 有判定词但无安全领域词
// 有领域词但无明确判定
// 受保护内容
