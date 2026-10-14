package admin

import (
	"bytes"
	"context"
	"encoding/json"

	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/codex2api/auth"
	"github.com/codex2api/cache"
	"github.com/codex2api/database"
	"github.com/codex2api/proxy"
	"github.com/gin-gonic/gin"
)

// A@ 与 a@ 视为重复（忽略大小写），保留首次出现的大小写形式。

func newInviteCacheTestHandler(t *testing.T, withCache bool) *Handler {
	t.Helper()
	db, err := database.New("sqlite", filepath.Join(t.TempDir(), "invite-cache.db"))
	if err != nil {
		t.Fatalf("database.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	h := &Handler{db: db}
	if withCache {
		h.cache = cache.NewMemory(1)
		t.Cleanup(func() { _ = h.cache.Close() })
	}
	return h
}

type inviteCacheProbe struct {
	Value string `json:"value"`
}

// cache=nil 等价于「进程重启、运行态缓存全空」：数据必须还能从库里读回来。

// 重新授权后资格属于另一份身份，旧快照不能继续端出来。

// 归一化后的参数才进作用域，否则「不传」与「传默认值」会各存一份相同内容。
func TestInviteCacheScopeNormalization(t *testing.T) {
	programA, entryA := proxy.NormalizeInviteProgram("", "")
	programB, entryB := proxy.NormalizeInviteProgram(proxy.DefaultProgramID, proxy.DefaultEntrypoint)
	if inviteEligibilityScope(programA, entryA) != inviteEligibilityScope(programB, entryB) {
		t.Fatal("empty and explicit-default program params must share one scope")
	}

	periodA, limitA := proxy.NormalizeInviteTracking("", 0)
	periodB, limitB := proxy.NormalizeInviteTracking(periodA, limitA)
	if inviteTrackingScope(programA, periodA, limitA) != inviteTrackingScope(programB, periodB, limitB) {
		t.Fatal("tracking defaults must normalize to one scope")
	}
	if inviteTrackingScope(programA, periodA, 10) == inviteTrackingScope(programA, periodA, limitA) {
		t.Fatal("different limits must not share a scope")
	}
}

func newInviteRecipientTestHandler(t *testing.T) (*Handler, *database.DB) {
	t.Helper()
	db, err := database.New("sqlite", filepath.Join(t.TempDir(), "invite-recipients.db"))
	if err != nil {
		t.Fatalf("database.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := auth.NewStore(db, nil, &database.SystemSettings{MaxConcurrency: 1})
	store.AddAccount(&auth.Account{
		DBID:        7,
		AccessToken: "test-access-token",
		AccountID:   "workspace-7",
		Email:       "sender@example.com",
	})
	return &Handler{db: db, store: store}, db
}

func invokeInviteJSON(t *testing.T, method, target string, body any, params gin.Params, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, target, bytes.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = params
	handler(c)
	return recorder
}

func TestRememberInviteTrackingRecipientsBackfillsLedger(t *testing.T) {
	h, db := newInviteRecipientTestHandler(t)
	h.rememberInviteTrackingRecipients(context.Background(), 7, proxy.DefaultProgramID,
		proxy.DefaultEntrypoint, &proxy.CodexInviteTracking{
			OK: true, StatusCode: http.StatusOK,
			Items: []proxy.CodexInviteTrackingItem{{
				Email: "history@example.com", Status: "redeemed",
				CreatedAt: "2026-08-03T05:24:58.842913Z",
			}},
		})
	items, err := db.ListCodexInviteRecipientsByEmails(context.Background(), []string{"history@example.com"})
	if err != nil || len(items) != 1 {
		t.Fatalf("read backfill: %v %+v", err, items)
	}
	if items[0].State != database.CodexInviteRecipientStateKnownInvited || items[0].UpstreamRecipientStatus != "redeemed" {
		t.Fatalf("unexpected backfill: %+v", items[0])
	}
}
