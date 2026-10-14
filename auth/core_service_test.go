package auth

import (
	"context"
	"github.com/codex2api/cache"
	"github.com/codex2api/database"
	"path/filepath"
	"testing"
)

func TestCoreServiceIgnoresPersistedPromptPolicy(t *testing.T) {
	db, err := database.New("sqlite", filepath.Join(t.TempDir(), "core.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tc := cache.NewMemory(8)
	defer tc.Close()
	settings := &database.SystemSettings{MaxConcurrency: 2, PromptFilterEnabled: true, PromptFilterReviewEnabled: true, PromptFilterAdvancedConfig: `{"newapi":{"enabled":true},"enforcement":{"conversation_lock_enabled":true}}`}
	store := NewStore(db, tc, settings)
	store.EnableCodexOnly()
	defer store.Stop()
	if err := store.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	cfg := store.GetPromptFilterConfigSnapshot()
	if cfg.Enabled || cfg.Review.Enabled || cfg.LogMatches || cfg.Advanced.NewAPI.Enabled || cfg.Advanced.Enforcement.ConversationLockEnabled {
		t.Fatalf("retired prompt policy is active: %+v", cfg)
	}
}
