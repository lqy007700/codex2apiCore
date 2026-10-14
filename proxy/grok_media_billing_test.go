package proxy

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"

	"testing"
	"time"

	"github.com/codex2api/auth"
	"github.com/codex2api/cache"
	"github.com/codex2api/database"
	"github.com/gin-gonic/gin"
)

func newGrokMediaBillingHandler(t *testing.T, upstreamURL string) (*Handler, *database.DB, int64) {
	t.Helper()
	db, err := database.New("sqlite", filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	keyID, err := db.InsertAPIKey(context.Background(), "media", "sk-media-billing-test")
	if err != nil {
		t.Fatal(err)
	}
	store := auth.NewStore(nil, nil, &database.SystemSettings{MaxConcurrency: 2})
	t.Cleanup(store.Stop)
	store.AddAccount(&auth.Account{DBID: 1, UpstreamType: auth.UpstreamGrok, APIKey: "xai-test", BaseURL: upstreamURL})
	handler := NewHandler(store, db, nil, nil)
	handler.SetRuntimeCache(cache.NewMemory(1))
	return handler, db, keyID
}

func grokMediaBillingContext(method, target string, body []byte, keyID int64) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, target, bytes.NewReader(body))
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	ctx.Set(contextAPIKeyID, keyID)
	return ctx, rec
}

func grokMediaUsageLogs(t *testing.T, db *database.DB, since time.Time) []*database.UsageLog {
	t.Helper()
	db.FlushUsageLogs()
	logs, err := db.ListUsageLogsByTimeRange(context.Background(), since.Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return logs
}

// 视频任务在状态查询首次看到 done 时结算一次:按上游秒数计成本,用户按秒计费,
// 重复轮询不重复扣费;提交行金额为 0。

// 失败任务不计费,但留一条失败记录;上游以非 2xx 返回 failed 状态体同样结算。

// 生图按张计上游成本,上游自报成本优先;张数不再冒充输出 token。

func TestParseXAIImaginePricing(t *testing.T) {
	body := []byte(`
### Text API Pricing

| Model | Context | Input / 1M tokens | Cached input / 1M tokens | Output / 1M tokens |
| --- | --- | --- | --- | --- |
| grok-4.7 (< 200k prompt tokens) | 500k | $2.00 | $0.50 | $6.00 |

### Imagine Pricing

| Model | Cost |
| --- | --- |
| grok-imagine-image-quality | $0.05 / image |
| grok-imagine-image | $0.02 / image |
| grok-imagine-video-1.5 | $0.080 / sec |
| grok-imagine-video | $0.50 / image |
`)
	got, err := ParseXAIOfficialPricingMarkdown(body)
	if err != nil {
		t.Fatal(err)
	}
	if got["grok-imagine-image"].MediaUnitCost != 0.02 || got["grok-imagine-image-quality"].MediaUnitCost != 0.05 || got["grok-imagine-video-1.5"].MediaUnitCost != 0.08 {
		t.Fatalf("imagine pricing = %+v", got)
	}
	if _, ok := got["grok-imagine-video"]; ok {
		t.Fatal("unit mismatch row must be skipped")
	}
	if got["grok-4.7"].Input != 2 {
		t.Fatal("text pricing lost")
	}
}

func approxFloat(a, b float64) bool {
	d := a - b
	return d < 1e-9 && d > -1e-9
}
