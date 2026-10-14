package admin

import (
	"encoding/json"

	"strings"
	"testing"
	"time"
)

func antigravityQuotaJSON(t *testing.T, remaining float64) string {
	t.Helper()
	snapshot := map[string]interface{}{
		"models": []map[string]interface{}{{"model_id": "gemini-3-flash", "remaining_fraction": 1}},
		"quota_groups": []map[string]interface{}{{
			"display_name": "Gemini Models",
			"buckets": []map[string]interface{}{
				{"bucket_id": "gemini-5h", "window": "5h", "remaining_fraction": 1},
				{"bucket_id": "gemini-weekly", "window": "weekly", "remaining_fraction": remaining},
			},
		}},
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func readBatchOperationEvents(t *testing.T, body string) []batchOperationEvent {
	t.Helper()
	events := make([]batchOperationEvent, 0)
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event batchOperationEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			t.Fatalf("decode event %q: %v", line, err)
		}
		events = append(events, event)
	}
	return events
}

// 通用批量刷新(含流式进度)对 Antigravity 账号走控制面同步，禁用账号也能刷新；
// 不存在的账号计入失败而不是中断整批。

// 运行时池为空:禁用账号本就不在池里,识别必须回落到数据库行。
