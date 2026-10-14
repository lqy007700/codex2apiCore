package admin

import (
	"bytes"
	"context"
	"encoding/json"

	"net/http/httptest"
	"testing"

	"github.com/codex2api/auth"
	"github.com/codex2api/database"
	"github.com/gin-gonic/gin"
)

func invokeOpenAIResponsesJSONHandler(t *testing.T, method, path string, params gin.Params, body any, handler func(*gin.Context)) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = params
	ctx.Request = httptest.NewRequest(method, path, bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	handler(ctx)
	return recorder
}

func assertOpenAIResponsesMetadataMode(t *testing.T, db *database.DB, store *auth.Store, accountID int64, want string) {
	t.Helper()
	row, err := db.GetAccountByID(context.Background(), accountID)
	if err != nil {
		t.Fatalf("GetAccountByID(%d): %v", accountID, err)
	}
	if got := row.GetCredential("codex_client_metadata_mode"); got != want {
		t.Fatalf("persisted metadata mode = %q, want %q", got, want)
	}
	account := store.FindByID(accountID)
	if account == nil {
		t.Fatalf("runtime account %d not found", accountID)
	}
	if got := account.OpenAIResponsesCodexClientMetadataMode(); got != want {
		t.Fatalf("runtime metadata mode = %q, want %q", got, want)
	}
}
