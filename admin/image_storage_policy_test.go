package admin

import (
	"io"
	"net/http"
	"net/http/httptest"

	"testing"

	"github.com/codex2api/internal/imagestore"

	"github.com/gin-gonic/gin"
)

func configureRetentionTestStorage(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("IMAGE_ASSET_DIR", root)
	previous := imagestore.CurrentConfig()
	t.Cleanup(func() { _ = imagestore.Configure(previous) })
	if err := imagestore.Configure(imagestore.Config{Backend: imagestore.BackendLocal, LocalDir: root}); err != nil {
		t.Fatal(err)
	}
}

func retentionRequest(router *gin.Engine, method, path, key string, body io.Reader) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	return w
}

type failingImageOutputWriter struct{}

type failingImageHTTPWriter struct{ header http.Header }

func (w *failingImageHTTPWriter) Header() http.Header { return w.header }
