package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/codex2api/database"
	"github.com/gin-gonic/gin"
)

func TestGetUsageRequestChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.New("sqlite", filepath.Join(t.TempDir(), "chain.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.InsertUsageLog(context.Background(), &database.UsageLogInput{RequestID: "handler-request", Endpoint: "/v1/responses", Model: "test", StatusCode: 200, AttemptIndex: 1}); err != nil {
		t.Fatal(err)
	}
	db.FlushUsageLogs()
	logs, err := db.ListRecentUsageLogs(context.Background(), 1)
	if err != nil || len(logs) != 1 {
		t.Fatalf("seed logs: %+v %v", logs, err)
	}
	h := &Handler{db: db}
	router := gin.New()
	router.GET("/usage/logs/:id/chain", h.GetUsageRequestChain)
	for _, test := range []struct {
		id     string
		status int
	}{{"bad", 400}, {"0", 400}, {"-1", 400}, {"9223372036854775808", 400}, {"999999", 404}, {"1", 200}} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/usage/logs/"+test.id+"/chain", nil))
		if recorder.Code != test.status {
			t.Fatalf("id %s: %d %s", test.id, recorder.Code, recorder.Body.String())
		}
		if test.status == 200 {
			var chain database.UsageRequestChain
			if err := json.Unmarshal(recorder.Body.Bytes(), &chain); err != nil {
				t.Fatal(err)
			}
			if chain.RequestID != "handler-request" || len(chain.Records) != 1 {
				t.Fatalf("unexpected payload: %+v", chain)
			}
		}
	}
}
