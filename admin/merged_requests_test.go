package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/codex2api/database"
	"github.com/gin-gonic/gin"
)

func TestGetMergedUsageLogsValidatesAndAggregates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.New("sqlite", filepath.Join(t.TempDir(), "requests.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for i, status := range []int{429, 200} {
		if err := db.InsertUsageLog(context.Background(), &database.UsageLogInput{RequestID: "merged-handler", Endpoint: "/v1/responses", Model: "test", AttemptIndex: i + 1, IsRetryAttempt: i == 0, StatusCode: status}); err != nil {
			t.Fatal(err)
		}
	}
	db.FlushUsageLogs()
	h := &Handler{db: db}
	router := gin.New()
	router.GET("/usage/requests", h.GetMergedUsageLogs)
	dates := "?start=" + time.Now().Add(-time.Hour).UTC().Format(time.RFC3339) + "&end=" + time.Now().Add(time.Hour).UTC().Format(time.RFC3339) + "&page=1"
	for _, test := range []struct {
		query  string
		status int
	}{{"", 400}, {"?start=no&end=no", 400}, {"?start=2026-10-15T00:00:00Z&end=2026-10-14T00:00:00Z", 400}, {dates + "&account_id=bad", 400}, {dates, 200}} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/usage/requests"+test.query, nil))
		if recorder.Code != test.status {
			t.Fatalf("%s: %d %s", test.query, recorder.Code, recorder.Body.String())
		}
		if test.status == 200 {
			var page database.MergedUsagePage
			if err := json.Unmarshal(recorder.Body.Bytes(), &page); err != nil {
				t.Fatal(err)
			}
			if page.Total != 1 || len(page.Logs) != 1 || page.Logs[0].RetryCount != 1 || page.Logs[0].StatusCode != 200 {
				t.Fatalf("not merged: %+v", page)
			}
		}
	}
}
