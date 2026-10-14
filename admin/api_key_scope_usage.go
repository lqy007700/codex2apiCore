package admin

// apiKeyScopeUsageItem 是某个 API Key 在一条 scope 限额上的当前用量（issue #439）。
// 只返回该条 scope 真正配了上限的窗口，前端据此渲染进度条。
type apiKeyScopeUsageItem struct {
	ScopeType   string                       `json:"scope_type"`
	ScopeID     int64                        `json:"scope_id"`
	ScopeName   string                       `json:"scope_name"`
	ScopeExists bool                         `json:"scope_exists"`
	OnExhausted string                       `json:"on_exhausted"`
	Windows     []apiKeyScopeUsageWindowItem `json:"windows"`
	// Cumulative 是累计额度（不随时间回落，需手动重置）的当前状态。
	Cumulative *apiKeyScopeCumulativeItem `json:"cumulative,omitempty"`
	// Skips 是这条预算被判定耗尽的运行态统计(进程内，重启清零)。预算耗尽后请求会静默
	// 落到其它账号，这个计数是唯一能直观回答「这条预算真的在生效吗」的信号。
	Skips *apiKeyScopeSkipItem `json:"skips,omitempty"`
}

type apiKeyScopeCumulativeItem struct {
	UsedCost      float64 `json:"used_cost"`
	UsedTokens    int64   `json:"used_tokens"`
	UsedRequests  int64   `json:"used_requests"`
	QuotaCost     float64 `json:"quota_cost,omitempty"`
	QuotaTokens   int64   `json:"quota_tokens,omitempty"`
	QuotaRequests int64   `json:"quota_requests,omitempty"`
	ResetCount    int     `json:"reset_count"`
	LastResetAt   string  `json:"last_reset_at,omitempty"`
	Exhausted     bool    `json:"exhausted"`
}

type apiKeyScopeSkipItem struct {
	Requests    int64  `json:"requests"`
	FirstAt     string `json:"first_at"`
	LastAt      string `json:"last_at"`
	LastMessage string `json:"last_message"`
}

type apiKeyScopeUsageWindowItem struct {
	Window       string  `json:"window"`
	Requests     int64   `json:"requests"`
	Tokens       int64   `json:"tokens"`
	UserBilled   float64 `json:"user_billed"`
	CostLimit    float64 `json:"cost_limit,omitempty"`
	TokenLimit   int64   `json:"token_limit,omitempty"`
	RequestLimit int64   `json:"request_limit,omitempty"`
	Exhausted    bool    `json:"exhausted"`
}

// GetAPIKeyScopeUsage 返回某 API Key 各条分组/账号维度限额的当前用量。
// GET /api/admin/keys/:id/scope-usage
//
// 口径与网关判定完全一致（同一套按账号拆分的窗口聚合 + 当前分组成员关系折算），
// 唯一差别是这里不叠加网关进程内的增量事件，因此可能比实际用量少最多一个批量落库周期。

// Four conditional aggregates share one 30-day index scan. This avoids the
// previous 5h/1d/7d/30d repeated scans when opening a Key with scope limits.

// 账号展示名由前端用自己的账号列表解析，这里只判断账号是否还在池中。

type resetAPIKeyScopeQuotaReq struct {
	ScopeType string `json:"scope_type"`
	ScopeID   int64  `json:"scope_id"`
}

// ResetAPIKeyScopeQuota 把某条 scope 的累计额度清零并记一次重置。
// POST /api/admin/keys/:id/scope-quota/reset
//
// 累计额度不随时间回落（这正是它与滑动窗口的区别），所以必须有这个显式入口，
// 否则用完就永久卡死。语义对齐 Key 级额度的 reset_quota。

// 重置后立刻重新观察命中统计，否则历史计数会让人误以为预算还在被拦。

// apiKeyScopeSummaryItem 是列表页用的 scope 预算概览:只给"最紧的那个窗口"的占比,
// 详情留给编辑抽屉。
type apiKeyScopeSummaryItem struct {
	ScopeType    string  `json:"scope_type"`
	ScopeID      int64   `json:"scope_id"`
	ScopeName    string  `json:"scope_name"`
	OnExhausted  string  `json:"on_exhausted"`
	Window       string  `json:"window"`
	Metric       string  `json:"metric"`
	Ratio        float64 `json:"ratio"`
	Exhausted    bool    `json:"exhausted"`
	SkipRequests int64   `json:"skip_requests,omitempty"`
}

// GetAPIKeysScopeSummary 一次返回全部（配了 scope 预算的）Key 的预算概览。
// GET /api/admin/keys-scope-summary
//
// 每个窗口只做一次跨 Key 的聚合查询，避免列表页按 Key 逐个查导致 N×窗口 的放大。

// 只查真的被用到的窗口。

// 概览只保留「用得最满」的那个窗口。占比 >= 1 与 CheckWindow 的耗尽判定等价
// （两者都是 used >= limit），所以不必再单独跑一遍判定。

// 累计额度也要参与「最紧」的比较，否则只配累计额度的 Key 在列表里
// 会显示成 0%。

// scopeCumulativeRatio 返回累计额度里"用得最满"的口径占比与口径名。

// scopeWindowRatio 返回某窗口内"用得最满"的口径占比（0~1+）与该口径名。

// aggregateScopeUsage 把按账号拆分的用量折算到一条 scope 上。

// collectAccountGroups 把聚合结果里出现的账号当前所属分组累积到 out。已从账号池
// 移除的账号解析不到分组，其历史用量在分组维度上被忽略。

// accountGroupNameIndex 返回分组 ID → 名称。查询失败时返回空表（前端退回显示 ID）。
