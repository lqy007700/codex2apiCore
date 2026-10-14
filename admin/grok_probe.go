package admin

import (
	"errors"

	"time"
)

const (
	// grokProbeRunGuard 单轮探测的整体超时兜底,避免异常账号把整轮卡死。
	grokProbeRunGuard           = 15 * time.Minute
	grokMaintenancePollInterval = time.Second
	grokMaintenanceLease        = 20 * time.Minute
	// Control-plane work has its own capacity; generation probes use another queue.
	grokMaintenanceBatchSize = 64
)

var errGrokMaintenanceProjectionIncomplete = errors.New("grok maintenance projection remains incomplete")

// StartGrokStatusProbe starts two independent maintenance loops:
//   - an always-on due_at queue for control-plane facts, model catalogs, and
//     native capabilities. The queue is leased across replicas and therefore
//     does not rescan every Grok account every 30 seconds;
//   - the historical inference connectivity probe, still governed by the
//     operator's GrokProbeEnabled switch and configured interval.
//
// This separation is security-relevant: disabling generation probes must not
// let a live plan, allow_access gate, balance, or catalog remain stale forever.

// One startup seed repairs rows created by older versions. New/changed Grok
// accounts are enqueued by database triggers thereafter.

// Generation/connectivity remains optional and retains the coarser settings
// cadence. Keeping it in a separate loop prevents a disabled probe switch or
// a slow generation attempt from suppressing freshness maintenance.

// 本地投影可能落后于触发器入队(账号由其它副本刚创建)。只有数据库
// 确认账号已删/非 Grok 才删任务,否则报错走退避重试等投影跟上。

// 运行时暂停≠删号:改期重试。数据库层 enabled=0 的任务删除由触发器
// 负责;触发器已删行时 CompleteMaintenanceJob 返回 ErrNoRows,忽略。

// Persisted upstream failures deliberately expire immediately. Treating
// that state as a successful one-second reschedule would create a tight
// retry loop during an outage. Returning an error keeps the generic job's
// attempt count and activates the bounded exponential backoff.

// Capability expiry must never block successful control-plane maintenance.

// grokImportProbeSlots bounds read-only post-import control-plane/catalog
// synchronization so a large archive cannot fan out thousands of requests.
var grokImportProbeSlots = make(chan struct{}, 4)

// triggerGrokUsageProbe keeps its historical import-call-site name. It first
// performs the fenced control-plane/catalog synchronization and then schedules
// the required three minimal native capability probes. The latter are bounded
// globally and serialised per account by runGrokCapabilityProbe.

// refreshStaleGrokControlPlane keeps user/settings/billing/catalog snapshots at
// their declared freshness without consuming account reservations or dispatch
// counters. It is intentionally independent from GrokProbeEnabled.

// runGrokStatusProbe performs only the optional generation connectivity check.
