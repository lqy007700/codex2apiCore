package database

import (
	"context"
	"path/filepath"
	"testing"
)

func TestCoreServicePreservesLegacyRowsButExcludesThemFromPool(t *testing.T) {
	db, err := New("sqlite", filepath.Join(t.TempDir(), "core.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	native, err := db.InsertAccountWithCredentials(ctx, "native", map[string]interface{}{"refresh_token": "test-native-refresh"}, "")
	if err != nil {
		t.Fatal(err)
	}
	relay, err := db.InsertOpenAIResponsesAccount(ctx, "legacy relay", map[string]interface{}{"upstream_type": "openai_responses", "base_url": "https://example.invalid/v1", "api_key": "test-relay-key"}, "")
	if err != nil {
		t.Fatal(err)
	}
	db.EnableCodexOnly()
	rows, err := db.ListActive(ctx)
	if err != nil || len(rows) != 1 || rows[0].ID != native {
		t.Fatalf("native pool = %v, %v", rows, err)
	}
	projection, err := db.ListAccountListProjection(ctx, "codex")
	if err != nil || len(projection) != 1 || projection[0].ID != native {
		t.Fatalf("admin projection = %v, %v", projection, err)
	}
	if _, err = db.GetAccountByID(ctx, relay); err != nil {
		t.Fatalf("legacy record was lost: %v", err)
	}
	for _, upstream := range []string{"grok", "claude", "antigravity", "openai_responses"} {
		if _, err = db.InsertAccountWithCredentials(ctx, "unsupported", map[string]interface{}{"upstream_type": upstream, "refresh_token": "test"}, ""); err == nil {
			t.Fatalf("accepted unsupported upstream %s", upstream)
		}
	}
}
