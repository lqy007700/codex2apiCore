package admin

import (
	"context"

	"path/filepath"
	"testing"

	"github.com/codex2api/auth"
	"github.com/codex2api/cache"
	"github.com/codex2api/database"
	"github.com/codex2api/proxy"
)

func newImagesSettingsHandler(t *testing.T) (*Handler, *database.DB, string) {
	t.Helper()
	previous := proxy.CurrentRuntimeSettings()
	t.Cleanup(func() { proxy.ApplyRuntimeSettings(previous) })
	path := filepath.Join(t.TempDir(), "images-settings.sqlite")
	db, err := database.New("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	settings := defaultBootstrapSettings()
	if err := db.UpdateSystemSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	proxy.ApplyRuntimeSettingsFromSystem(settings)
	tc := cache.NewMemory(4)
	t.Cleanup(func() { _ = tc.Close() })
	store := auth.NewStore(db, tc, settings)
	t.Cleanup(store.Stop)
	return NewHandler(store, db, tc, proxy.NewRateLimiter(settings.GlobalRPM), "admin-secret"), db, path
}

// Simulate startup loading the persisted value after runtime settings were reset.
