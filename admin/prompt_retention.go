package admin

import (
	"time"

	"github.com/codex2api/database"
)

// Prompt 审核日志保留：定时按天数分批清理过期日志（跳过 CY 关联行），
// 手动「清空日志」也走同一条分批通道，不再受请求超时限制。

const (
	promptLogRetentionCheckInterval = time.Hour
	promptLogRetentionRunTimeout    = 30 * time.Minute
	promptLogPurgeBatchSize         = database.DefaultPromptLogPurgeBatch
	promptLogPurgeBatchPause        = 200 * time.Millisecond
)

// promptLogPurgeRunning 保证定时清理与手动清理不会同时跑（两者都是长时间分批删除）。
var promptLogPurgeRunning int32

type promptLogRetentionResponse struct {
	RetentionDays      int     `json:"retention_days"`
	Running            bool    `json:"running"`
	LastRunAt          *string `json:"last_run_at,omitempty"`
	LastDeletedLogs    int64   `json:"last_deleted_logs"`
	LastDeletedEvents  int64   `json:"last_deleted_events"`
	LastDeletedSources int64   `json:"last_deleted_sources"`
	LastDurationMs     int64   `json:"last_duration_ms"`
	LastError          string  `json:"last_error,omitempty"`
}

// GetPromptLogRetention 返回保留天数与上次清理统计（GET /api/admin/prompt-filter/retention）。

type updatePromptLogRetentionRequest struct {
	RetentionDays int `json:"retention_days"`
}

// UpdatePromptLogRetention 设置保留天数（0 = 关闭自动清理，最大 365）。

// RunPromptLogRetentionNow 立即在后台按当前保留天数清理一次；已有清理在跑时返回 409。

// startPromptLogPurge 在后台启动一次分批清理；占用中返回 false。

// runPromptLogRetention 执行一次保留清理并记录结果。

// StartPromptLogRetention 启动每小时一次的保留清理；保留天数为 0 时不做任何事。

// 启动后稍等再跑第一轮，避免与其他启动任务抢写锁。
