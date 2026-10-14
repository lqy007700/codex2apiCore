package admin

import (
	"fmt"

	"strings"

	"github.com/codex2api/database"
)

// Rule IDs identify persistent counters. Allowing a schedule or pattern to change
// under the same ID could split a week's budget or silently reuse another budget.
func normalizeAdminAPIKeyModelRequestLimits(in, existing []database.APIKeyModelRequestLimit) ([]database.APIKeyModelRequestLimit, error) {
	byID := make(map[string]database.APIKeyModelRequestLimit, len(existing))
	for _, rule := range existing {
		byID[rule.ID] = rule
	}
	for _, rule := range in {
		if id := strings.TrimSpace(rule.ID); id != "" {
			if _, ok := byID[id]; !ok {
				return nil, fmt.Errorf("limits.model_request_limits: 未知规则 ID %q；新增规则请省略 id，由服务端生成", id)
			}
		}
	}
	clean, err := database.NormalizeAPIKeyModelRequestLimits(in)
	if err != nil {
		return nil, err
	}
	for _, rule := range clean {
		old, ok := byID[rule.ID]
		if !ok {
			continue
		}
		if rule.Model != old.Model || rule.Window != old.Window || rule.Timezone != old.Timezone || rule.ResetWeekday != old.ResetWeekday || rule.ResetTime != old.ResetTime {
			return nil, fmt.Errorf("limits.model_request_limits: 规则 %q 的模型与重置时间不可直接修改；请删除旧规则并新增规则。修改次数上限时请保留原 id", rule.ID)
		}
	}
	return clean, nil
}

// GetAPIKeyModelRequestUsage reads the same durable counters used at dispatch.
// GET /api/admin/keys/:id/model-request-usage
