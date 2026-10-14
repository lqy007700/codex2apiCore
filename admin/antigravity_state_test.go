package admin

import (
	"context"

	"net/http/httptest"

	"strconv"

	"testing"

	"github.com/codex2api/auth"
	"github.com/codex2api/database"
	"github.com/gin-gonic/gin"
)

func newAntigravityStateTestHandler(t *testing.T, credentials map[string]any) (*Handler, *database.DB, int64) {
	t.Helper()
	db := newTestAdminDB(t)
	id, err := db.InsertAccountWithUpstream(context.Background(), "antigravity-state", "google", auth.UpstreamAntigravity, credentials, "")
	if err != nil {
		t.Fatal(err)
	}
	store := auth.NewStore(db, nil, nil)
	t.Cleanup(store.Stop)
	if err := store.LoadAccountByID(context.Background(), id); err != nil {
		t.Fatalf("load runtime account: %v", err)
	}
	return &Handler{db: db, store: store}, db, id
}

func callAntigravityAdminHandler(t *testing.T, method, target string, id int64, handler func(*gin.Context)) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(id, 10)}}
	c.Request = httptest.NewRequest(method, target, nil)
	handler(c)
	return recorder
}

// Current clients also receive per-tier 3.7 IDs, so "gemini-3.7-flash-low"
// names both a raw backing and the public tier served by the tiered model.
