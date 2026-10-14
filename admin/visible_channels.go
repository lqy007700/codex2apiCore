package admin

// 管理台可见渠道设置：控制仪表盘与账号管理里展示哪些上游渠道。
// 只影响管理台展示，不影响调度与 API 行为；Codex 作为兜底渠道始终显示。

type visibleChannelsResponse struct {
	Channels []string `json:"channels"`
	All      []string `json:"all"`
	Fallback string   `json:"fallback"`
}

// GetVisibleChannelsSettings 返回当前可见渠道。
// GET /api/admin/settings/visible-channels

// UpdateVisibleChannelsSettings 保存可见渠道；未知渠道被丢弃，兜底渠道自动补回。
// PUT /api/admin/settings/visible-channels  {"channels":["codex","grok"]}
