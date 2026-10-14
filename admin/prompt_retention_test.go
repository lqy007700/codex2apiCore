package admin

import (
	"path/filepath"

	"testing"

	"github.com/codex2api/database"
)

func newPromptRetentionTestHandler(t *testing.T) *Handler {
	t.Helper()
	db, err := database.New("sqlite", filepath.Join(t.TempDir(), "retention.db"))
	if err != nil {
		t.Fatalf("New(sqlite): %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &Handler{db: db}
}
