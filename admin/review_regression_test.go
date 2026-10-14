package admin

import (
	"context"
	"encoding/json"

	"strings"
	"testing"

	"github.com/codex2api/auth"

	"github.com/codex2api/proxy"
)

func TestReviewClaudeBillingReprobeRestoresWhitelist(t *testing.T) {
	db := newTestAdminDB(t)
	ctx := context.Background()
	models := []string{"claude-haiku-4-5", "claude-fable-5-1"}
	id, err := db.InsertAccountWithUpstream(ctx, "probe", "anthropic", auth.UpstreamClaude, map[string]interface{}{"upstream_type": auth.UpstreamClaude, "models": models, "access_token": "test-token"}, "")
	if err != nil {
		t.Fatal(err)
	}
	store := auth.NewStore(db, nil, nil)
	t.Cleanup(store.Stop)
	account := &auth.Account{DBID: id, UpstreamType: auth.UpstreamClaude, AccessToken: "test-token", Models: models}
	store.AddAccount(account)
	h := &Handler{db: db, store: store}
	if !proxy.HandleClaudeModelBillingRejection(store, account, models[1], 429, []byte(`{"error":{"message":"Usage credits are required for this model."}}`)) {
		t.Fatal("billing rejection not handled")
	}
	if got, err := h.connectionTestModelForAccount(ctx, account, models[1]); err != nil || got != models[1] {
		t.Fatalf("explicit reprobe rejected: %q %v", got, err)
	}
	if _, err := h.connectionTestModelForAccount(ctx, account, "claude-unrelated"); err == nil {
		t.Fatal("unrelated whitelist exclusion was bypassed")
	}
	if err := store.RestoreClaudeAccountModel(ctx, account, models[1]); err != nil {
		t.Fatal(err)
	}
	store.ClearModelCooldown(account, models[1])
	row, err := db.GetAccountByID(ctx, id)
	if err != nil || len(row.GetCredentialStringSlice("models")) != 2 {
		t.Fatal("successful probe did not restore the persisted catalog")
	}
}

func TestReviewShortDiagnosticSecretsPreserveIDs(t *testing.T) {
	got := sanitizeCodexTestText(`{"response_id":"resp_0123","usage":1234,"api_key":"1","echo":"1 1"}`, []string{"1"})
	var parsed map[string]any
	if json.Unmarshal([]byte(got), &parsed) != nil || parsed["response_id"] != "resp_0123" || parsed["usage"] != float64(1234) {
		t.Fatalf("short placeholder corrupted diagnostics: %s", got)
	}
	if strings.Contains(got, `"1"`) || strings.Contains(got, `1 1`) {
		t.Fatal("standalone credential was exposed")
	}
}
