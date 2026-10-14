package proxy

import (
	"bytes"

	"net/http/httptest"

	"testing"

	"github.com/codex2api/database"

	"github.com/gin-gonic/gin"
)

func TestEncryptLiveAttestationRoundTrip(t *testing.T) {
	cipher, err := encryptLiveAttestation(`{"v":1,"s":0,"t":"token"}`, "admin-secret")
	if err != nil {
		t.Fatal(err)
	}
	plain, err := decryptLiveAttestation(cipher, "admin-secret")
	if err != nil || plain != `{"v":1,"s":0,"t":"token"}` {
		t.Fatalf("plain=%q err=%v", plain, err)
	}
	if _, err := decryptLiveAttestation(cipher, "other"); err == nil {
		t.Fatal("wrong secret should fail")
	}
}

func liveTestContext(t *testing.T, method, path string, body []byte, row *database.APIKeyRow) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	if row != nil {
		c.Set(contextAPIKeyRow, row)
		c.Set(contextAPIKeyID, row.ID)
	}
	return c, rec
}

func containsAll(items []string, want ...string) bool {
	set := make(map[string]struct{}, len(items))
	for _, item := range items {
		set[item] = struct{}{}
	}
	for _, item := range want {
		if _, ok := set[item]; !ok {
			return false
		}
	}
	return true
}
