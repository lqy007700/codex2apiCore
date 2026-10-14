package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"testing"

	"github.com/codex2api/auth"

	"github.com/gin-gonic/gin"
)

const codexUsageRefreshFixture = `{"plan_type":"plus","rate_limit":{"primary_window":{"used_percent":23,"limit_window_seconds":18000,"reset_after_seconds":3600},"secondary_window":{"used_percent":57,"limit_window_seconds":604800,"reset_after_seconds":86400}}}`

func callCodexUsageRefresh(h *Handler, ctx context.Context, stream bool) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/accounts/batch-refresh-usage?stream=%t", stream), nil).WithContext(ctx)
	h.BatchRefreshCodexUsage(c)
	return w
}

func TestBatchRefreshCodexUsageEmptyPoolAndBusyAccount(t *testing.T) {
	store := auth.NewStore(nil, nil, nil)
	defer store.Stop()
	h := &Handler{store: store}
	response := callCodexUsageRefresh(h, context.Background(), false)
	var result batchOperationEvent
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || response.Code != http.StatusOK || result.Total != 0 || result.Type != "complete" {
		t.Fatalf("empty batch: %d %s", response.Code, response.Body.String())
	}
	account := &auth.Account{DBID: 1, AccessToken: "codex-token", Status: auth.StatusReady}
	store.AddAccount(account)
	account.TryBeginUsageProbe()
	defer account.FinishUsageProbe()
	response = callCodexUsageRefresh(h, context.Background(), false)
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Total != 1 || result.Failed != 1 || result.Success != 0 {
		t.Fatalf("busy account: %s", response.Body.String())
	}
}
