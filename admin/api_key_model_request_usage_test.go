package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/codex2api/database"
	"github.com/gin-gonic/gin"
)

func callModelRequestKeyMutation(t *testing.T, h *Handler, method string, id int64, body any) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(id)}}
	c.Request = httptest.NewRequest(method, "/api/admin/keys", strings.NewReader(string(encoded)))
	c.Request.Header.Set("Content-Type", "application/json")
	if method == http.MethodPost {
		h.CreateAPIKey(c)
	} else {
		h.UpdateAPIKey(c)
	}
	return rec
}

// Renaming without supplying limits preserves existing rules.

// Return generated IDs immediately, so reopening an editor before its list
// refresh cannot accidentally submit a newly added rule as another new rule.

func TestAPIKeyModelRequestRulesRejectInvalidAndChangedRules(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newTestAdminDB(t)
	h := &Handler{db: db}
	for name, raw := range map[string]map[string]any{
		"empty model":    {"model": "", "max_requests": 50},
		"zero limit":     {"model": "gpt-6*", "max_requests": 0},
		"negative limit": {"model": "gpt-6*", "max_requests": -1},
		"window":         {"model": "gpt-6*", "max_requests": 50, "window": "month"},
		"timezone":       {"model": "gpt-6*", "max_requests": 50, "timezone": "Not/AZone"},
		"weekday":        {"model": "gpt-6*", "max_requests": 50, "reset_weekday": 8},
		"time":           {"model": "gpt-6*", "max_requests": 50, "reset_time": "25:00"},
		"supplied ID":    {"id": "mr_other", "model": "gpt-6*", "max_requests": 50},
	} {
		t.Run(name, func(t *testing.T) {
			rec := callModelRequestKeyMutation(t, h, http.MethodPost, 0, map[string]any{
				"name": "Invalid budget", "limits": map[string]any{"model_request_limits": []any{raw}},
			})
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("invalid rule accepted: %d %s", rec.Code, rec.Body.String())
			}
		})
	}
	id, err := db.InsertAPIKeyWithOptions(context.Background(), database.APIKeyInput{
		Name: "Immutable", Key: "sk-model-budget-immutable-1234567890",
		Limits: database.APIKeyLimits{ModelRequestLimits: []database.APIKeyModelRequestLimit{{Model: "gpt-6*", MaxRequests: 50}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	row, err := db.GetAPIKeyByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	original := row.Limits.ModelRequestLimits[0]
	for name, mutate := range map[string]func(*database.APIKeyModelRequestLimit){
		"model":      func(r *database.APIKeyModelRequestLimit) { r.Model = "gpt-5*" },
		"timezone":   func(r *database.APIKeyModelRequestLimit) { r.Timezone = "UTC" },
		"weekday":    func(r *database.APIKeyModelRequestLimit) { r.ResetWeekday = 2 },
		"time":       func(r *database.APIKeyModelRequestLimit) { r.ResetTime = "12:00" },
		"foreign ID": func(r *database.APIKeyModelRequestLimit) { r.ID = "mr_foreign" },
	} {
		t.Run(name, func(t *testing.T) {
			rule := original
			mutate(&rule)
			rec := callModelRequestKeyMutation(t, h, http.MethodPatch, id, map[string]any{
				"limits": database.APIKeyLimits{ModelRequestLimits: []database.APIKeyModelRequestLimit{rule}},
			})
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("immutable field accepted: %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}
