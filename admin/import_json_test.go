package admin

import (
	"bytes"
	"context"

	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/codex2api/auth"
	"github.com/codex2api/database"
	"github.com/codex2api/internal/openaiidentity"
	"github.com/gin-gonic/gin"
)

func TestConflictingImportChatGPTIDs(t *testing.T) {
	tokens := []importToken{
		{chatgptAccountID: "shared", refreshToken: "rt-1"},
		{chatgptAccountID: "shared", refreshToken: "rt-2"},
		{chatgptAccountID: "stable", refreshToken: "rt-3"},
		{chatgptAccountID: "stable", refreshToken: "rt-3"},
	}

	conflicts := conflictingImportChatGPTIDs(tokens)
	if !conflicts["shared"] {
		t.Fatal("shared chatgpt_account_id should be marked conflicting")
	}
	if conflicts["stable"] {
		t.Fatal("stable chatgpt_account_id should not be marked conflicting")
	}
	if got := reliableImportChatGPTID(tokens[0], conflicts); got != "" {
		t.Fatalf("reliableImportChatGPTID(shared) = %q, want empty", got)
	}
	if got := reliableImportChatGPTID(tokens[2], conflicts); got != "stable" {
		t.Fatalf("reliableImportChatGPTID(stable) = %q, want stable", got)
	}
}

func TestParseCredentialExpiresAtSupportsUnixSeconds(t *testing.T) {
	got := parseCredentialExpiresAt("1779071020").UTC()
	want := time.Unix(1779071020, 0).UTC()
	if !got.Equal(want) {
		t.Fatalf("parseCredentialExpiresAt = %s, want %s", got, want)
	}
}

func TestAccountFromCredentialSeedRestoresUsageSnapshots(t *testing.T) {
	account := accountFromCredentialSeed(42, "", tokenCredentialSeed{
		planType:              "free",
		codex7DUsedPercent:    "3",
		codex7DResetAt:        "2026-05-15T20:33:11+08:00",
		codex5HUsedPercent:    "0",
		codex5HResetAt:        "2026-05-11T11:39:07+08:00",
		codex5HUsageUpdatedAt: "2026-05-11T10:39:07+08:00",
		codexUsageUpdatedAt:   "2026-05-11T11:39:07+08:00",
	})

	if got := account.GetPlanType(); got != "free" {
		t.Fatalf("PlanType = %q, want free", got)
	}
	pct7d, ok := account.GetUsagePercent7d()
	if !ok || pct7d != 3 {
		t.Fatalf("7d usage = %v/%t, want 3/true", pct7d, ok)
	}
	if account.GetReset7dAt().IsZero() {
		t.Fatal("Reset7dAt is zero")
	}
	pct5h, ok := account.GetUsagePercent5h()
	if !ok || pct5h != 0 {
		t.Fatalf("5h usage = %v/%t, want 0/true", pct5h, ok)
	}
	if account.GetUsageUpdatedAt5h().IsZero() {
		t.Fatal("UsageUpdatedAt5h is zero")
	}
	if account.GetUsageUpdatedAt5h().Equal(account.GetUsageUpdatedAt()) {
		t.Fatalf("UsageUpdatedAt5h = %s, want separate 5h timestamp from 7d", account.GetUsageUpdatedAt5h())
	}
}

func TestAccountFromCredentialSeedDoesNotReuse7dFreshnessForMissing5hTimestamp(t *testing.T) {
	account := accountFromCredentialSeed(42, "", tokenCredentialSeed{
		codex7DUsedPercent:  "3",
		codex5HUsedPercent:  "95",
		codex5HResetAt:      time.Now().Add(time.Hour).Format(time.RFC3339),
		codexUsageUpdatedAt: time.Now().Format(time.RFC3339),
	})

	if account.GetUsageUpdatedAt().IsZero() {
		t.Fatal("UsageUpdatedAt is zero")
	}
	if !account.GetUsageUpdatedAt5h().IsZero() {
		t.Fatalf("UsageUpdatedAt5h = %s, want zero when codex_5h_usage_updated_at is missing", account.GetUsageUpdatedAt5h())
	}
}

func TestImportTokensFromTextFilesReadsAllUploadedFiles(t *testing.T) {
	files := []uploadedImportFile{
		{name: "one.txt", data: append([]byte{0xef, 0xbb, 0xbf}, []byte("rt-1\nrt-shared\n")...)},
		{name: "two.txt", data: []byte("rt-2\nrt-shared\n")},
	}

	tokens := importTokensFromTextFiles(files, func(token string) importToken {
		return importToken{refreshToken: token}
	})

	if len(tokens) != 3 {
		t.Fatalf("tokens len = %d, want 3", len(tokens))
	}
	for i, want := range []string{"rt-1", "rt-shared", "rt-2"} {
		if tokens[i].refreshToken != want {
			t.Fatalf("tokens[%d] = %q, want %q", i, tokens[i].refreshToken, want)
		}
	}
}

func TestValidateImportFileSize(t *testing.T) {
	if err := validateImportFileSize(&multipart.FileHeader{Filename: "ok.txt", Size: importFileSizeLimitBytes}); err != nil {
		t.Fatalf("validateImportFileSize returned error for boundary size: %v", err)
	}

	err := validateImportFileSize(&multipart.FileHeader{Filename: "too-big.txt", Size: importFileSizeLimitBytes + 1})
	if err == nil {
		t.Fatal("expected oversized file error, got nil")
	}
	if got, want := err.Error(), "文件 too-big.txt 大小超过 200MB"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func TestImportAccountsCommonDoesNotCollapseConflictingChatGPTAccountID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	store := auth.NewStore(nil, nil, &database.SystemSettings{MaxConcurrency: 2, TestConcurrency: 1, TestModel: "gpt-5.4"})
	handler := &Handler{
		db:    db,
		store: store,
		probeUsage: func(context.Context, *auth.Account) error {
			return nil
		},
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/admin/accounts/import", nil)

	handler.importAccountsCommon(ctx, []importToken{
		{name: "sub2api-1", refreshToken: "rt-shared-id-1", accessToken: "at-shared-id-1", chatgptAccountID: "same-exported-id"},
		{name: "sub2api-2", refreshToken: "rt-shared-id-2", accessToken: "at-shared-id-2", chatgptAccountID: "same-exported-id"},
	}, importSettings{})

	rows, err := db.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("active rows = %d, want 2", len(rows))
	}
	for _, row := range rows {
		if got := row.GetCredential("account_id"); got != "" {
			t.Fatalf("account_id = %q, want empty for conflicting chatgpt_account_id", got)
		}
	}
}

func TestImportAccountsCommonUpdatesKnownWorkspaceWhenDuplicatesAllowed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	store := auth.NewStore(db, nil, &database.SystemSettings{MaxConcurrency: 2, TestConcurrency: 1, TestModel: "gpt-5.4"})
	probed := make(chan int64, 1)
	handler := &Handler{
		db:    db,
		store: store,
		probeUsage: func(_ context.Context, acc *auth.Account) error {
			probed <- acc.DBID
			return nil
		},
	}

	existingID, err := db.InsertAccountWithCredentials(context.Background(), "existing", map[string]interface{}{
		"refresh_token": "rt-old",
		"email":         "import@example.com",
		"account_id":    "acc-import",
		"workspace_id":  "acc-import",
	}, "")
	if err != nil {
		t.Fatalf("InsertAccountWithCredentials: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/admin/accounts/import", nil)

	handler.importAccountsCommon(ctx, []importToken{{
		refreshToken: "rt-new",
		accessToken:  "at-new",
		idToken:      makeOAuthTestIDToken("Import@Example.com", "acc-import", "team"),
		email:        "Import@Example.com",
		accountID:    "acc-import",
		planType:     "team",
	}}, importSettings{allowDuplicate: true})

	select {
	case id := <-probed:
		if id != existingID {
			t.Fatalf("probed account id = %d, want %d", id, existingID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("usage probe was not triggered for updated OAuth identity")
	}

	rows, err := db.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("active rows = %d, want 1", len(rows))
	}
	row, err := db.GetAccountByID(context.Background(), existingID)
	if err != nil {
		t.Fatalf("GetAccountByID: %v", err)
	}
	if got := row.GetCredential("refresh_token"); got != "rt-new" {
		t.Fatalf("refresh_token = %q, want rt-new", got)
	}
	if got := row.GetCredential("access_token"); got != "at-new" {
		t.Fatalf("access_token = %q, want at-new", got)
	}
	if got := row.GetCredential("plan_type"); got != "team" {
		t.Fatalf("plan_type = %q, want team", got)
	}
	if account := store.FindByID(existingID); account == nil {
		t.Fatalf("runtime account %d not found after import update", existingID)
	}
}

func TestImportAccountsCommonSkipsExistingOAuthIdentityWithSameCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	store := auth.NewStore(db, nil, &database.SystemSettings{MaxConcurrency: 2, TestConcurrency: 1, TestModel: "gpt-5.4"})
	handler := &Handler{
		db:    db,
		store: store,
		probeUsage: func(context.Context, *auth.Account) error {
			t.Fatal("usage probe should not run for unchanged duplicate import")
			return nil
		},
	}

	existingID, err := db.InsertAccountWithCredentials(context.Background(), "existing", map[string]interface{}{
		"refresh_token": "rt-same",
		"session_token": "st-same",
		"access_token":  "at-same",
		"email":         "same@example.com",
		"account_id":    "acc-same",
		"workspace_id":  "acc-same",
	}, "")
	if err != nil {
		t.Fatalf("InsertAccountWithCredentials: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/admin/accounts/import", nil)

	handler.importAccountsCommon(ctx, []importToken{{
		refreshToken: "rt-same",
		sessionToken: "st-same",
		accessToken:  "at-same",
		idToken:      makeOAuthTestIDToken("Same@Example.com", "acc-same", ""),
		email:        "Same@Example.com",
		accountID:    "acc-same",
	}}, importSettings{})

	var payload map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, recorder.Body.String())
	}
	if got := int(payload["success"].(float64)); got != 0 {
		t.Fatalf("success = %d, want 0", got)
	}
	if got := int(payload["duplicate"].(float64)); got != 1 {
		t.Fatalf("duplicate = %d, want 1", got)
	}
	if got := int(payload["total"].(float64)); got != 1 {
		t.Fatalf("total = %d, want 1", got)
	}

	rows, err := db.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != existingID {
		t.Fatalf("active rows = %+v, want only existing id %d", rows, existingID)
	}
}

func TestImportAccountsCommonSkipsAmbiguousOAuthIdentityWithExistingAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	store := auth.NewStore(db, nil, &database.SystemSettings{MaxConcurrency: 2, TestConcurrency: 1, TestModel: "gpt-5.4"})
	handler := &Handler{
		db:    db,
		store: store,
		probeUsage: func(context.Context, *auth.Account) error {
			return nil
		},
	}

	existingID, err := db.InsertAccountWithCredentials(context.Background(), "existing", map[string]interface{}{
		"refresh_token": "rt-old",
		"email":         "ambiguous@example.com",
		"account_id":    "acc-ambiguous",
		"workspace_id":  "acc-ambiguous",
	}, "")
	if err != nil {
		t.Fatalf("InsertAccountWithCredentials: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/admin/accounts/import", nil)

	handler.importAccountsCommon(ctx, []importToken{
		{refreshToken: "rt-new-1", idToken: makeOAuthTestIDToken("ambiguous@example.com", "acc-ambiguous", ""), email: "ambiguous@example.com", accountID: "acc-ambiguous"},
		{refreshToken: "rt-new-2", idToken: makeOAuthTestIDToken("Ambiguous@Example.com", "acc-ambiguous", ""), email: "Ambiguous@Example.com", accountID: "acc-ambiguous"},
	}, importSettings{})

	var payload map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, recorder.Body.String())
	}
	if got := int(payload["success"].(float64)); got != 0 {
		t.Fatalf("success = %d, want 0", got)
	}
	if got := int(payload["duplicate"].(float64)); got != 2 {
		t.Fatalf("duplicate = %d, want 2", got)
	}
	if got := int(payload["total"].(float64)); got != 2 {
		t.Fatalf("total = %d, want 2", got)
	}

	row, err := db.GetAccountByID(context.Background(), existingID)
	if err != nil {
		t.Fatalf("GetAccountByID: %v", err)
	}
	if got := row.GetCredential("refresh_token"); got != "rt-old" {
		t.Fatalf("refresh_token = %q, want rt-old", got)
	}
}

func TestImportAccountsCommonSkipsAmbiguousKnownWorkspaceWhenDuplicatesAllowed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	store := auth.NewStore(db, nil, &database.SystemSettings{MaxConcurrency: 2, TestConcurrency: 1, TestModel: "gpt-5.4"})
	handler := &Handler{
		db:    db,
		store: store,
		probeUsage: func(context.Context, *auth.Account) error {
			return nil
		},
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/admin/accounts/import", nil)

	handler.importAccountsCommon(ctx, []importToken{
		{refreshToken: "rt-new-1", idToken: makeOAuthTestIDToken("new-ambiguous@example.com", "acc-new-ambiguous", ""), email: "new-ambiguous@example.com", accountID: "acc-new-ambiguous"},
		{refreshToken: "rt-new-2", idToken: makeOAuthTestIDToken("New-Ambiguous@Example.com", "acc-new-ambiguous", ""), email: "New-Ambiguous@Example.com", accountID: "acc-new-ambiguous"},
	}, importSettings{allowDuplicate: true})

	var payload map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, recorder.Body.String())
	}
	if got := int(payload["success"].(float64)); got != 0 {
		t.Fatalf("success = %d, want 0", got)
	}
	if got := int(payload["duplicate"].(float64)); got != 2 {
		t.Fatalf("duplicate = %d, want 2", got)
	}
	if got := int(payload["total"].(float64)); got != 2 {
		t.Fatalf("total = %d, want 2", got)
	}

	rows, err := db.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("active rows = %d, want 0", len(rows))
	}
}

func TestImportAccountsCommonCollapsesIdenticalOAuthIdentityInFile(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	store := auth.NewStore(db, nil, &database.SystemSettings{MaxConcurrency: 2, TestConcurrency: 1, TestModel: "gpt-5.4"})
	handler := &Handler{
		db:    db,
		store: store,
		probeUsage: func(context.Context, *auth.Account) error {
			return nil
		},
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/admin/accounts/import", nil)

	handler.importAccountsCommon(ctx, []importToken{
		{refreshToken: "rt-same-file", accessToken: "at-same-file", idToken: makeOAuthTestIDToken("same-file@example.com", "acc-same-file", ""), email: "same-file@example.com", accountID: "acc-same-file"},
		{refreshToken: "rt-same-file", accessToken: "at-same-file", idToken: makeOAuthTestIDToken("Same-File@Example.com", "acc-same-file", ""), email: "Same-File@Example.com", accountID: "acc-same-file"},
	}, importSettings{})

	if !strings.Contains(recorder.Body.String(), `"type":"complete"`) ||
		!strings.Contains(recorder.Body.String(), `"success":1`) ||
		!strings.Contains(recorder.Body.String(), `"total":1`) {
		t.Fatalf("SSE payload = %q, want complete success=1 total=1", recorder.Body.String())
	}

	rows, err := db.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("active rows = %d, want 1", len(rows))
	}
	if got := rows[0].GetCredential("refresh_token"); got != "rt-same-file" {
		t.Fatalf("refresh_token = %q, want rt-same-file", got)
	}
}

func TestImportAccountsCommonTriggersUsageProbeForImportedAccountWithAccessToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	store := auth.NewStore(nil, nil, &database.SystemSettings{MaxConcurrency: 2, TestConcurrency: 1, TestModel: "gpt-5.4"})
	probed := make(chan int64, 1)
	handler := &Handler{
		db:    db,
		store: store,
		probeUsage: func(_ context.Context, acc *auth.Account) error {
			probed <- acc.DBID
			return nil
		},
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/admin/accounts/import", nil)

	handler.importAccountsCommon(ctx, []importToken{{
		refreshToken: "rt-import-probe",
		accessToken:  "at-import-probe",
	}}, importSettings{})

	select {
	case id := <-probed:
		if id == 0 {
			t.Fatal("probed account id is zero")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("usage probe was not triggered for imported account with access token")
	}
}

func TestImportAccountsCommonMarksImported7dUsageAsRateLimited(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	store := auth.NewStore(db, nil, &database.SystemSettings{MaxConcurrency: 2, TestConcurrency: 1, TestModel: "gpt-5.4"})
	handler := &Handler{
		db:    db,
		store: store,
		probeUsage: func(context.Context, *auth.Account) error {
			return nil
		},
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/admin/accounts/import", nil)

	resetAt := time.Now().Add(6 * time.Hour).UTC().Truncate(time.Second)
	handler.importAccountsCommon(ctx, []importToken{{
		refreshToken:       "rt-import-limited",
		accessToken:        "at-import-limited",
		planType:           "team",
		codex7DUsedPercent: "100",
		codex7DResetAt:     resetAt.Format(time.RFC3339),
	}}, importSettings{})

	accounts := store.Accounts()
	if len(accounts) != 1 {
		t.Fatalf("store accounts = %d, want 1", len(accounts))
	}
	account := accounts[0]
	if got := account.RuntimeStatus(); got != "rate_limited" {
		t.Fatalf("RuntimeStatus() = %q, want rate_limited", got)
	}
	reason, until := account.GetCooldownSnapshot()
	if reason != "rate_limited" || !until.After(time.Now()) {
		t.Fatalf("cooldown = (%q, %s), want active rate_limited", reason, until)
	}

	row, err := db.GetAccountByID(context.Background(), account.DBID)
	if err != nil {
		t.Fatalf("GetAccountByID: %v", err)
	}
	if row.CooldownReason != "rate_limited" || !row.CooldownUntil.Valid {
		t.Fatalf("persisted cooldown = (%q, %v), want active rate_limited", row.CooldownReason, row.CooldownUntil)
	}
}

func TestImportAccountsCommonRefreshesAndProbesRTOnlyImport(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	store := auth.NewStore(nil, nil, &database.SystemSettings{MaxConcurrency: 2, TestConcurrency: 1, TestModel: "gpt-5.4"})
	probed := make(chan int64, 1)
	handler := &Handler{
		db:    db,
		store: store,
		refreshAccount: func(_ context.Context, id int64) error {
			acc := store.FindByID(id)
			if acc == nil {
				return fmt.Errorf("account %d not found", id)
			}
			acc.Mu().Lock()
			acc.AccessToken = "at-refreshed"
			acc.Mu().Unlock()
			return nil
		},
		probeUsage: func(_ context.Context, acc *auth.Account) error {
			probed <- acc.DBID
			return nil
		},
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/admin/accounts/import", nil)

	handler.importAccountsCommon(ctx, []importToken{{refreshToken: "rt-import-refresh-probe"}}, importSettings{})

	select {
	case id := <-probed:
		if id == 0 {
			t.Fatal("probed account id is zero")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("usage probe was not triggered after RT-only import refresh")
	}
}

func TestImportAccountsCommonRefreshesOAuthIdentityRTOnlyImport(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	store := auth.NewStore(nil, nil, &database.SystemSettings{MaxConcurrency: 2, TestConcurrency: 1, TestModel: "gpt-5.4"})
	refreshed := make(chan int64, 2)
	probed := make(chan int64, 1)
	handler := &Handler{
		db:    db,
		store: store,
		refreshAccount: func(_ context.Context, id int64) error {
			refreshed <- id
			acc := store.FindByID(id)
			if acc == nil {
				return fmt.Errorf("account %d not found", id)
			}
			acc.Mu().Lock()
			acc.AccessToken = "at-oauth-identity-refreshed"
			acc.Mu().Unlock()
			return nil
		},
		probeUsage: func(_ context.Context, acc *auth.Account) error {
			probed <- acc.DBID
			return nil
		},
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/admin/accounts/import", nil)

	handler.importAccountsCommon(ctx, []importToken{{
		refreshToken: "rt-oauth-identity-refresh-probe",
		email:        "identity-refresh@example.com",
		accountID:    "acc-identity-refresh",
	}}, importSettings{})

	select {
	case id := <-probed:
		if id == 0 {
			t.Fatal("probed account id is zero")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("usage probe was not triggered after OAuth identity RT-only import refresh")
	}
	select {
	case id := <-refreshed:
		if id == 0 {
			t.Fatal("refreshed account id is zero")
		}
	default:
		t.Fatal("refresh was not triggered")
	}
	select {
	case id := <-refreshed:
		t.Fatalf("refresh triggered more than once, second id=%d", id)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestAddAccountStreamReportsProgressAndProbesAfterRefresh(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	store := auth.NewStore(nil, nil, &database.SystemSettings{MaxConcurrency: 2, TestConcurrency: 1, TestModel: "gpt-5.4"})
	probed := make(chan int64, 2)
	handler := &Handler{
		db:    db,
		store: store,
		refreshAccount: func(_ context.Context, id int64) error {
			acc := store.FindByID(id)
			if acc == nil {
				return fmt.Errorf("account %d not found", id)
			}
			acc.Mu().Lock()
			acc.AccessToken = fmt.Sprintf("at-%d", id)
			acc.Mu().Unlock()
			return nil
		},
		probeUsage: func(_ context.Context, acc *auth.Account) error {
			probed <- acc.DBID
			return nil
		},
	}

	body := bytes.NewBufferString(`{"refresh_token":"rt-stream-1\nrt-stream-2"}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/admin/accounts?stream=true", body)
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.AddAccount(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	payload := recorder.Body.String()
	if !strings.Contains(payload, `"type":"complete"`) || !strings.Contains(payload, `"success":2`) {
		t.Fatalf("SSE payload = %q, want complete success=2", payload)
	}

	seen := map[int64]bool{}
	deadline := time.After(2 * time.Second)
	for len(seen) < 2 {
		select {
		case id := <-probed:
			seen[id] = true
		case <-deadline:
			t.Fatalf("usage probes = %v, want 2 accounts probed", seen)
		}
	}
}

func TestAddAccountSkipRefreshDoesNotWarmup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	store := auth.NewStore(nil, nil, &database.SystemSettings{MaxConcurrency: 2, TestConcurrency: 1, TestModel: "gpt-5.4"})
	t.Cleanup(store.Stop)
	var refreshed atomic.Int64
	handler := &Handler{
		db:    db,
		store: store,
		refreshAccount: func(context.Context, int64) error {
			refreshed.Add(1)
			return nil
		},
		probeUsage: func(context.Context, *auth.Account) error {
			t.Fatal("usage probe should not run when skip_refresh is set")
			return nil
		},
	}

	body := bytes.NewBufferString(`{"refresh_token":"rt-skip-1\nrt-skip-2","skip_refresh":true}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/admin/accounts", body)
	ctx.Request.Header.Set("Content-Type", "application/json")
	handler.AddAccount(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}

	time.Sleep(150 * time.Millisecond)
	if got := refreshed.Load(); got != 0 {
		t.Fatalf("refresh count = %d, want 0", got)
	}
	if store.AccountCount() != 2 {
		t.Fatalf("runtime accounts = %d, want 2", store.AccountCount())
	}
}

func TestAddAccountRTRefreshUsesImportProbeGate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	store := auth.NewStore(nil, nil, &database.SystemSettings{
		MaxConcurrency: 2, TestConcurrency: 1, TestModel: "gpt-5.4", UsageProbeConcurrency: 1,
	})
	store.SetUsageProbeConcurrency(1)
	t.Cleanup(store.Stop)

	var current atomic.Int64
	var maxConcurrent atomic.Int64
	var finished atomic.Int64
	handler := &Handler{
		db:    db,
		store: store,
		refreshAccount: func(context.Context, int64) error {
			n := current.Add(1)
			for {
				prev := maxConcurrent.Load()
				if n <= prev || maxConcurrent.CompareAndSwap(prev, n) {
					break
				}
			}
			time.Sleep(80 * time.Millisecond)
			current.Add(-1)
			finished.Add(1)
			return nil
		},
		probeUsage: func(context.Context, *auth.Account) error {
			return nil
		},
	}

	body := bytes.NewBufferString(`{"refresh_token":"rt-gate-1\nrt-gate-2\nrt-gate-3\nrt-gate-4"}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/admin/accounts", body)
	ctx.Request.Header.Set("Content-Type", "application/json")
	handler.AddAccount(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}

	deadline := time.After(2 * time.Second)
	for finished.Load() < 4 {
		select {
		case <-deadline:
			t.Fatalf("finished refreshes = %d, want 4", finished.Load())
		case <-time.After(20 * time.Millisecond):
		}
	}
	if got := maxConcurrent.Load(); got != 1 {
		t.Fatalf("max concurrent refreshes = %d, want 1", got)
	}
}

func TestImportedATAndRTWarmupsReserveCapacityAtHighProbeSetting(t *testing.T) {
	store := auth.NewStore(nil, nil, &database.SystemSettings{
		MaxConcurrency: 2, TestConcurrency: 1, TestModel: "gpt-5.4", UsageProbeConcurrency: 64,
	})
	store.SetUsageProbeConcurrency(64)
	t.Cleanup(store.Stop)

	var current atomic.Int64
	var maxConcurrent atomic.Int64
	var finished atomic.Int64
	work := func() {
		n := current.Add(1)
		for {
			previous := maxConcurrent.Load()
			if n <= previous || maxConcurrent.CompareAndSwap(previous, n) {
				break
			}
		}
		time.Sleep(40 * time.Millisecond)
		current.Add(-1)
		finished.Add(1)
	}

	handler := &Handler{
		store: store,
		importLoadSnapshot: func() importRuntimeLoadSnapshot {
			return importRuntimeLoadSnapshot{RPM: 1000, DBMaxOpen: 100}
		},
		refreshAccount: func(_ context.Context, id int64) error {
			work()
			account := store.FindByID(id)
			if account == nil {
				return fmt.Errorf("account %d not found", id)
			}
			account.Mu().Lock()
			account.AccessToken = fmt.Sprintf("refreshed-at-%d", id)
			account.Mu().Unlock()
			return nil
		},
		probeUsage: func(context.Context, *auth.Account) error {
			work()
			return nil
		},
	}

	accounts := make([]*auth.Account, 0, 8)
	for id := int64(1); id <= 8; id++ {
		account := &auth.Account{DBID: id, RefreshToken: fmt.Sprintf("rt-%d", id)}
		if id <= 4 {
			account.AccessToken = fmt.Sprintf("at-%d", id)
		}
		accounts = append(accounts, account)
	}
	handler.commitImportedRuntimeAccounts(accounts, "test_import", false)

	// 4 个 AT 各探测一次；4 个 RT 各刷新并探测一次。
	const wantFinished = 12
	deadline := time.Now().Add(3 * time.Second)
	for finished.Load() < wantFinished {
		if time.Now().After(deadline) {
			t.Fatalf("finished warmup operations = %d, want %d", finished.Load(), wantFinished)
		}
		time.Sleep(10 * time.Millisecond)
	}
	// 等待一个完整 worker 周期，确保 RT 刷新内部没有偷偷多跑一次 probe。
	time.Sleep(100 * time.Millisecond)
	if got := finished.Load(); got != wantFinished {
		t.Fatalf("finished warmup operations = %d, want exactly %d (RT refresh probed twice)", got, wantFinished)
	}
	const wantHighLoadConcurrency = 4
	if got := maxConcurrent.Load(); got > wantHighLoadConcurrency {
		t.Fatalf("max concurrent import warmups = %d, want <= %d", got, wantHighLoadConcurrency)
	}
	if got := handler.importProbeConcurrency(); got != wantHighLoadConcurrency {
		t.Fatalf("import probe concurrency = %d, want %d at high load", got, wantHighLoadConcurrency)
	}
}

// workspace_id 无法从 JWT 识别时，允许重复添加可保留相同邮箱的多个账号。
func TestImportAccountsCommonAllowsDuplicateWithoutWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	store := auth.NewStore(db, nil, &database.SystemSettings{MaxConcurrency: 2, TestConcurrency: 1, TestModel: "gpt-5.4"})
	handler := &Handler{
		db:    db,
		store: store,
		probeUsage: func(context.Context, *auth.Account) error {
			return nil
		},
	}

	if _, err := db.InsertAccountWithCredentials(context.Background(), "existing", map[string]interface{}{
		"refresh_token": "rt-dup",
		"email":         "dup@example.com",
		"account_id":    "acc-dup",
	}, ""); err != nil {
		t.Fatalf("InsertAccountWithCredentials: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/admin/accounts/import", nil)

	handler.importAccountsCommon(ctx, []importToken{{
		refreshToken: "rt-dup-2",
		email:        "dup@example.com",
		accountID:    "acc-dup",
	}}, importSettings{allowDuplicate: true})

	rows, err := db.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("active rows = %d, want 2 (duplicate allowed)", len(rows))
	}
}

func TestImportAccountsCommonSeparatesCredentialWorkspaceRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	store := auth.NewStore(db, nil, &database.SystemSettings{MaxConcurrency: 2, TestConcurrency: 1, TestModel: "gpt-5.4"})
	store.SetLazyMode(true)
	t.Cleanup(store.Stop)
	handler := &Handler{db: db, store: store}

	if _, err := db.InsertAccountWithCredentials(context.Background(), "personal", map[string]interface{}{
		"refresh_token": "rt-import-shared",
	}, ""); err != nil {
		t.Fatalf("Insert personal: %v", err)
	}

	runImport := func(workspaceID string, allowDuplicate bool) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/admin/accounts/import", nil)
		headers := map[string]string(nil)
		if workspaceID != "" {
			headers = map[string]string{"Chatgpt-Account-Id": workspaceID}
		}
		handler.importAccountsCommon(ctx, []importToken{{
			refreshToken: "rt-import-shared",
		}}, importSettings{allowDuplicate: allowDuplicate, customHeaders: headers})
		if recorder.Code != http.StatusOK {
			t.Fatalf("workspace %q status = %d: %s", workspaceID, recorder.Code, recorder.Body.String())
		}
	}

	runImport("team-a", false)
	runImport("team-a", true)
	runImport("team-b", false)

	rows, err := db.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("active rows = %d, want personal + team-a + team-b", len(rows))
	}
	gotRoutes := make(map[string]bool, len(rows))
	for _, row := range rows {
		gotRoutes[openaiidentity.WorkspaceOverrideFromHeaders(row.GetCredentialStringMap("custom_headers"))] = true
	}
	for _, workspaceID := range []string{"", "team-a", "team-b"} {
		if !gotRoutes[workspaceID] {
			t.Fatalf("persisted routes = %#v, missing %q", gotRoutes, workspaceID)
		}
	}
}

// TestAddAccountDedupsRefreshToken 验证：RT 单账号添加默认按 RT 原文对已有库去重。
func TestAddAccountDedupsRefreshToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	store := auth.NewStore(db, nil, &database.SystemSettings{MaxConcurrency: 2, TestConcurrency: 1, TestModel: "gpt-5.4"})
	handler := &Handler{
		db:    db,
		store: store,
		probeUsage: func(context.Context, *auth.Account) error {
			return nil
		},
	}

	if _, err := db.InsertAccountWithCredentials(context.Background(), "existing", map[string]interface{}{
		"refresh_token": "rt-existing",
	}, ""); err != nil {
		t.Fatalf("InsertAccountWithCredentials: %v", err)
	}

	doAdd := func(body string) map[string]interface{} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/admin/accounts", strings.NewReader(body))
		ctx.Request.Header.Set("Content-Type", "application/json")
		handler.AddAccount(ctx)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
		}
		var resp map[string]interface{}
		if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return resp
	}

	// 默认：重复 RT 应被跳过
	resp := doAdd(`{"refresh_token":"rt-existing"}`)
	if dup := resp["duplicate"]; dup != float64(1) {
		t.Fatalf("duplicate = %v, want 1", dup)
	}
	if rows, _ := db.ListActive(context.Background()); len(rows) != 1 {
		t.Fatalf("active rows = %d, want 1 (duplicate skipped)", len(rows))
	}

	// 勾选允许重复：同一 RT 应被新建
	resp = doAdd(`{"refresh_token":"rt-existing","allow_duplicate":true}`)
	if suc := resp["success"]; suc != float64(1) {
		t.Fatalf("success = %v, want 1", suc)
	}
	if rows, _ := db.ListActive(context.Background()); len(rows) != 2 {
		t.Fatalf("active rows = %d, want 2 (duplicate allowed)", len(rows))
	}
}

func TestAddAccountSeparatesRefreshTokenWorkspaceRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	store := auth.NewStore(db, nil, &database.SystemSettings{MaxConcurrency: 2, TestConcurrency: 1, TestModel: "gpt-5.4"})
	store.SetLazyMode(true)
	t.Cleanup(store.Stop)
	handler := &Handler{db: db, store: store}

	add := func(workspaceID string, allowDuplicate bool) map[string]interface{} {
		payload := map[string]interface{}{
			"refresh_token":   "rt-shared-route",
			"allow_duplicate": allowDuplicate,
		}
		if workspaceID != "" {
			payload["custom_headers"] = map[string]string{
				"Chatgpt-Account-Id": workspaceID,
			}
		}
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/admin/accounts", bytes.NewReader(body))
		ctx.Request.Header.Set("Content-Type", "application/json")
		handler.AddAccount(ctx)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
		}
		var response map[string]interface{}
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		return response
	}

	if response := add("", false); response["success"] != float64(1) {
		t.Fatalf("personal add = %#v, want success=1", response)
	}
	if response := add("team-a", false); response["success"] != float64(1) {
		t.Fatalf("team-a add = %#v, want success=1", response)
	}
	if response := add("team-a", true); response["duplicate"] != float64(1) {
		t.Fatalf("team-a duplicate = %#v, want duplicate=1 even with allow_duplicate", response)
	}
	if response := add("team-b", false); response["success"] != float64(1) {
		t.Fatalf("team-b add = %#v, want success=1", response)
	}
	if response := add("", false); response["duplicate"] != float64(1) {
		t.Fatalf("personal duplicate = %#v, want duplicate=1", response)
	}

	rows, err := db.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("active rows = %d, want personal + team-a + team-b", len(rows))
	}
	gotRoutes := make(map[string]bool, len(rows))
	for _, row := range rows {
		gotRoutes[openaiidentity.WorkspaceOverrideFromHeaders(row.GetCredentialStringMap("custom_headers"))] = true
	}
	for _, workspaceID := range []string{"", "team-a", "team-b"} {
		if !gotRoutes[workspaceID] {
			t.Fatalf("persisted routes = %#v, missing %q", gotRoutes, workspaceID)
		}
	}
}

// 重复添加同一身份的 AT（JWT 身份）时，命中已有账号应计入"更新"而非"新增"，
// 新增计数保持为 0（不再把更新的账号重复计进 success/duplicate）。
func TestAddATAccountCountsUpdateNotNew(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	store := auth.NewStore(db, nil, &database.SystemSettings{MaxConcurrency: 2, TestConcurrency: 1, TestModel: "gpt-5.4"})
	handler := &Handler{
		db:    db,
		store: store,
		probeUsage: func(context.Context, *auth.Account) error {
			return nil
		},
	}

	makeAT := func(exp time.Time) string {
		return makeAdminTestJWT(t, map[string]interface{}{
			"exp": exp.Unix(),
			"https://api.openai.com/profile": map[string]interface{}{
				"email": "solo@example.com",
			},
			"https://api.openai.com/auth": map[string]interface{}{
				"chatgpt_account_id": "acc-count-1",
				"chatgpt_plan_type":  "team",
			},
		})
	}

	doAddAT := func(token string) map[string]interface{} {
		body, _ := json.Marshal(map[string]interface{}{"access_token": token})
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/admin/accounts/at", strings.NewReader(string(body)))
		ctx.Request.Header.Set("Content-Type", "application/json")
		handler.AddATAccount(ctx)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
		}
		var resp map[string]interface{}
		if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return resp
	}

	// 首次添加：应新增 1。
	resp := doAddAT(makeAT(time.Now().Add(2 * time.Hour)))
	if resp["success"] != float64(1) || resp["updated"] != float64(0) {
		t.Fatalf("first add: success=%v updated=%v, want 1/0", resp["success"], resp["updated"])
	}

	// 再次添加同身份（AT 已轮换，exp 不同）：应计入更新，新增为 0。
	resp = doAddAT(makeAT(time.Now().Add(3 * time.Hour)))
	if resp["success"] != float64(0) {
		t.Fatalf("re-add success = %v, want 0 (更新不应计入新增)", resp["success"])
	}
	if resp["updated"] != float64(1) {
		t.Fatalf("re-add updated = %v, want 1", resp["updated"])
	}

	// 库里始终只有一个账号。
	if rows, _ := db.ListActive(context.Background()); len(rows) != 1 {
		t.Fatalf("active rows = %d, want 1", len(rows))
	}
}

func TestAddOpaqueATPersistsAndDeduplicatesWorkspaceRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	store := auth.NewStore(db, nil, &database.SystemSettings{MaxConcurrency: 2, TestConcurrency: 1, TestModel: "gpt-5.4"})
	t.Cleanup(store.Stop)
	handler := &Handler{
		db:    db,
		store: store,
		probeUsage: func(context.Context, *auth.Account) error {
			return nil
		},
	}

	add := func(workspaceID string, allowDuplicate bool) map[string]interface{} {
		payload := map[string]interface{}{
			"access_token":    "at-opaque-shared",
			"allow_duplicate": allowDuplicate,
		}
		if workspaceID != "" {
			payload["custom_headers"] = map[string]string{
				"Chatgpt-Account-Id": workspaceID,
			}
		}
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/admin/accounts/at", bytes.NewReader(body))
		ctx.Request.Header.Set("Content-Type", "application/json")
		handler.AddATAccount(ctx)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
		}
		var response map[string]interface{}
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		return response
	}

	if response := add("", false); response["success"] != float64(1) {
		t.Fatalf("personal add = %#v, want success=1", response)
	}
	if response := add("team-a", false); response["success"] != float64(1) {
		t.Fatalf("team-a add = %#v, want success=1", response)
	}
	if response := add("team-a", true); response["duplicate"] != float64(1) {
		t.Fatalf("team-a duplicate = %#v, want duplicate=1 even with allow_duplicate", response)
	}
	if response := add("team-b", false); response["success"] != float64(1) {
		t.Fatalf("team-b add = %#v, want success=1", response)
	}
	if response := add("", false); response["duplicate"] != float64(1) {
		t.Fatalf("personal duplicate = %#v, want duplicate=1", response)
	}

	rows, err := db.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("active rows = %d, want personal + team-a + team-b", len(rows))
	}
	gotRoutes := make(map[string]bool, len(rows))
	for _, row := range rows {
		if got := row.GetCredential("access_token"); got != "at-opaque-shared" {
			t.Fatalf("access_token = %q, want at-opaque-shared", got)
		}
		if got := row.GetCredential("access_token_type"); got != accessTokenTypeCodexAT {
			t.Fatalf("access_token_type = %q, want %s", got, accessTokenTypeCodexAT)
		}
		gotRoutes[openaiidentity.WorkspaceOverrideFromHeaders(row.GetCredentialStringMap("custom_headers"))] = true
	}
	for _, workspaceID := range []string{"", "team-a", "team-b"} {
		if !gotRoutes[workspaceID] {
			t.Fatalf("persisted routes = %#v, missing %q", gotRoutes, workspaceID)
		}
	}
}

func TestStreamAddOpaqueATPersistsWorkspaceRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	store := auth.NewStore(db, nil, &database.SystemSettings{MaxConcurrency: 2, TestConcurrency: 1, TestModel: "gpt-5.4"})
	t.Cleanup(store.Stop)
	handler := &Handler{
		db:    db,
		store: store,
		probeUsage: func(context.Context, *auth.Account) error {
			return nil
		},
	}

	body := []byte(`{
		"access_token": "at-opaque-stream",
		"custom_headers": {"chatgpt-account-id": "team-stream"}
	}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/admin/accounts/at?stream=true", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	handler.AddATAccount(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}

	rows, err := db.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("active rows = %d, want 1", len(rows))
	}
	headers := rows[0].GetCredentialStringMap("custom_headers")
	if got := openaiidentity.WorkspaceOverrideFromHeaders(headers); got != "team-stream" {
		t.Fatalf("workspace override = %q, want team-stream", got)
	}
}

// RT 刷新后 workspace 身份可知时，应把新凭证合并进已有账号。

// 已有 AT 账号：带用量统计

// 新导入的 RT 账号：刷新完成后身份与旧账号相同

// wham 返回的 account_id 不作为 JWT workspace_id；无法解析 workspace 的账号可并存。
func TestProbeImportedAccountUsageKeepsUnknownWorkspaceDuplicate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	store := auth.NewStore(db, nil, &database.SystemSettings{MaxConcurrency: 2, TestConcurrency: 1, TestModel: "gpt-5.4"})
	handler := &Handler{db: db, store: store}

	// 已有账号：身份完整、带用量统计。
	oldID, err := db.InsertAccountWithCredentials(context.Background(), "at-first", map[string]interface{}{
		"access_token":          "at-old",
		"email":                 "solo@example.com",
		"account_id":            "workspace-probe1",
		"workspace_id":          "workspace-probe1",
		"codex_7d_used_percent": "37.0",
	}, "")
	if err != nil {
		t.Fatalf("Insert old: %v", err)
	}

	// 新添加的 codex_at 账号：插入时无身份，仅有 access_token 原文。
	newID, err := db.InsertAccountWithCredentials(context.Background(), "at-new", map[string]interface{}{
		"access_token": "at-new",
	}, "")
	if err != nil {
		t.Fatalf("Insert new: %v", err)
	}
	store.AddAccount(&auth.Account{DBID: newID, AccessToken: "at-new", Status: auth.StatusReady})

	// 模拟 wham 探针：只补齐旧 account_id，不产生 workspace_id。
	handler.probeUsage = func(ctx context.Context, acc *auth.Account) error {
		store.UpdateAccountIdentity(acc, "solo@example.com", "workspace-probe1")
		return nil
	}

	handler.probeImportedAccountUsage(context.Background(), newID, "manual_at")

	rows, err := db.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("active rows = %d, want 2", len(rows))
	}

	oldRow, err := db.GetAccountByID(context.Background(), oldID)
	if err != nil {
		t.Fatalf("GetAccountByID: %v", err)
	}
	if got := oldRow.GetCredential("access_token"); got != "at-old" {
		t.Fatalf("access_token = %q, want at-old", got)
	}
	if got := oldRow.GetCredential("codex_7d_used_percent"); got != "37.0" {
		t.Fatalf("codex_7d_used_percent = %q, want 37.0 (用量统计必须保留)", got)
	}
	newRow, err := db.GetAccountByID(context.Background(), newID)
	if err != nil {
		t.Fatalf("GetAccountByID new: %v", err)
	}
	if got := newRow.GetCredential("workspace_id"); got != "" {
		t.Fatalf("workspace_id = %q, want empty", got)
	}
}

// workspace 已知时，历史 allow_duplicate 标记不能绕过去重。

// 平铺在 credentials 里的 Agent Identity 条目（sub2api 导出形态）应被通用 JSON
// 导入识别，而不是当作"无凭据"静默丢弃（issue #424 同根因）。

// 流式解析器必须与全量解析器对所有形态产出完全一致的 tokens——覆盖顶层数组、
// 单个平铺对象(带 BOM)、sub2api {accounts:[...]}、多余顶层字段、空 accounts。

// BOM + 流式:json.Decoder 不吃 BOM,parseImportJSONTokensStream 须先剥。
func TestParseImportJSONTokensStreamHandlesBOM(t *testing.T) {
	body := append([]byte{0xef, 0xbb, 0xbf}, []byte(`{"refresh_token":"rt-bom"}`)...)
	stream, err := parseImportJSONTokensStream(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("stream parse BOM: %v", err)
	}
	if len(stream) != 1 || stream[0].refreshToken != "rt-bom" {
		t.Fatalf("stream = %+v", stream)
	}
}

func TestParseImportJSONTokensStreamRejectsBrokenJSON(t *testing.T) {
	if _, err := parseImportJSONTokensStream(bytes.NewReader([]byte(`{"accounts":[}`))); err == nil {
		t.Fatal("expected error for broken json")
	}
}

func TestAdaptiveImportLimitsUseHysteresisAndDatabasePressure(t *testing.T) {
	snapshot := importRuntimeLoadSnapshot{DBMaxOpen: 100}
	now := time.Unix(1_800_000_000, 0)
	h := &Handler{
		importLoadSnapshot: func() importRuntimeLoadSnapshot { return snapshot },
		importLoadNow:      func() time.Time { return now },
	}

	if got := h.adaptiveImportLimits(); got != (importConcurrencyLimits{db: 12, probe: 8}) {
		t.Fatalf("low-load limits = %+v, want db=12 probe=8", got)
	}
	snapshot.RPM = 200
	if got := h.adaptiveImportLimits(); got != (importConcurrencyLimits{db: 8, probe: 6}) {
		t.Fatalf("medium-load limits = %+v, want db=8 probe=6", got)
	}
	// 介于升档(180)和降档(120)阈值之间时保持中档，避免抖动。
	snapshot.RPM = 150
	if got := h.adaptiveImportLimits(); got != (importConcurrencyLimits{db: 8, probe: 6}) {
		t.Fatalf("hysteresis limits = %+v, want medium tier", got)
	}
	snapshot.RPM = 100
	now = now.Add(importTierRecoveryDelay)
	if got := h.adaptiveImportLimits(); got != (importConcurrencyLimits{db: 12, probe: 8}) {
		t.Fatalf("recovered limits = %+v, want low tier", got)
	}
	snapshot.RPM = 700
	if got := h.adaptiveImportLimits(); got != (importConcurrencyLimits{db: 4, probe: 4}) {
		t.Fatalf("high-load limits = %+v, want db=4 probe=4", got)
	}
	// 高档每次最多降一档。
	snapshot.RPM = 0
	if got := h.adaptiveImportLimits(); got != (importConcurrencyLimits{db: 4, probe: 4}) {
		t.Fatalf("early recovery limits = %+v, want high tier during hold", got)
	}
	now = now.Add(importTierRecoveryDelay)
	if got := h.adaptiveImportLimits(); got != (importConcurrencyLimits{db: 8, probe: 6}) {
		t.Fatalf("first recovery limits = %+v, want medium tier", got)
	}
	now = now.Add(importTierRecoveryDelay)
	if got := h.adaptiveImportLimits(); got != (importConcurrencyLimits{db: 12, probe: 8}) {
		t.Fatalf("second recovery limits = %+v, want low tier", got)
	}
	snapshot.DBWaitCount++
	if got := h.adaptiveImportLimits(); got != (importConcurrencyLimits{db: 4, probe: 4}) {
		t.Fatalf("DB-wait limits = %+v, want immediate high tier", got)
	}
	now = now.Add(importTierRecoveryDelay)
	if got := h.adaptiveImportLimits(); got != (importConcurrencyLimits{db: 4, probe: 4}) {
		t.Fatalf("DB-wait backoff limits = %+v, want latched high tier", got)
	}
}

func TestAdaptiveImportLimitsReserveDatabasePoolCapacity(t *testing.T) {
	h := &Handler{importLoadSnapshot: func() importRuntimeLoadSnapshot {
		return importRuntimeLoadSnapshot{DBMaxOpen: 8}
	}}
	if got := h.adaptiveImportLimits(); got != (importConcurrencyLimits{db: 2, probe: 8}) {
		t.Fatalf("small-pool limits = %+v, want db=2 probe=8", got)
	}
}

func TestAdaptiveImportLimitsReactToLongRunningRequests(t *testing.T) {
	snapshot := importRuntimeLoadSnapshot{Active: 64, DBMaxOpen: 100}
	now := time.Unix(1_800_000_000, 0)
	h := &Handler{
		importLoadSnapshot: func() importRuntimeLoadSnapshot { return snapshot },
		importLoadNow:      func() time.Time { return now },
	}
	if got := h.adaptiveImportLimits(); got != (importConcurrencyLimits{db: 4, probe: 4}) {
		t.Fatalf("high-active limits = %+v, want db=4 probe=4", got)
	}
	snapshot.Active = 0
	now = now.Add(importTierRecoveryDelay)
	if got := h.adaptiveImportLimits(); got != (importConcurrencyLimits{db: 8, probe: 6}) {
		t.Fatalf("first active recovery limits = %+v, want medium tier", got)
	}
	now = now.Add(importTierRecoveryDelay)
	if got := h.adaptiveImportLimits(); got != (importConcurrencyLimits{db: 12, probe: 8}) {
		t.Fatalf("second active recovery limits = %+v, want low tier", got)
	}
}

func TestAdaptiveImportDBLimiterShrinksWhileImportIsRunning(t *testing.T) {
	var rpm atomic.Int64
	h := &Handler{importLoadSnapshot: func() importRuntimeLoadSnapshot {
		return importRuntimeLoadSnapshot{RPM: rpm.Load(), DBMaxOpen: 100}
	}}
	limiter := &adaptiveImportDBLimiter{handler: h}
	for i := 0; i < 12; i++ {
		if !limiter.acquire(context.Background()) {
			t.Fatalf("low-load permit %d was not acquired", i+1)
		}
	}
	if got := limiter.active.Load(); got != 12 {
		t.Fatalf("low-load active permits = %d, want 12", got)
	}

	rpm.Store(1000)
	blockedCtx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	if limiter.acquire(blockedCtx) {
		cancel()
		t.Fatal("high-load limiter granted a 13th permit")
	}
	cancel()
	for i := 0; i < 8; i++ {
		limiter.release()
	}
	blockedCtx, cancel = context.WithTimeout(context.Background(), 75*time.Millisecond)
	if limiter.acquire(blockedCtx) {
		cancel()
		t.Fatal("high-load limiter exceeded four active permits")
	}
	cancel()

	limiter.release()
	if !limiter.acquire(context.Background()) {
		t.Fatal("high-load limiter did not refill the fourth permit")
	}
	for limiter.active.Load() > 0 {
		limiter.release()
	}
}

func TestRunImportProbeTaskUsesEightWorkersAtLowLoad(t *testing.T) {
	store := auth.NewStore(nil, nil, &database.SystemSettings{MaxConcurrency: 1, UsageProbeConcurrency: 64})
	store.SetUsageProbeConcurrency(64)
	t.Cleanup(store.Stop)
	h := &Handler{
		store: store,
		importLoadSnapshot: func() importRuntimeLoadSnapshot {
			return importRuntimeLoadSnapshot{DBMaxOpen: 100}
		},
	}

	const n = 8
	started := make(chan struct{}, n)
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		h.runImportProbeTask(func(context.Context) {
			defer wg.Done()
			started <- struct{}{}
			<-release
		})
	}
	for i := 0; i < n; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			close(release)
			t.Fatalf("only %d/%d low-load probe workers started", i, n)
		}
	}
	close(release)
	wg.Wait()
}

// 导入采样 worker pool:同一时刻在途任务数不超过容量，超出的任务只进队列，
// 不会各自创建一个阻塞 goroutine。
func TestRunImportProbeTaskConcurrencyGate(t *testing.T) {
	store := auth.NewStore(nil, nil, &database.SystemSettings{MaxConcurrency: 1})
	store.SetUsageProbeConcurrency(2)
	h := &Handler{store: store}

	const n = 6
	var inFlight int32
	var maxSeen int32
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		h.runImportProbeTask(func(_ context.Context) {
			defer wg.Done()
			cur := atomic.AddInt32(&inFlight, 1)
			for {
				old := atomic.LoadInt32(&maxSeen)
				if cur <= old || atomic.CompareAndSwapInt32(&maxSeen, old, cur) {
					break
				}
			}
			<-release
			atomic.AddInt32(&inFlight, -1)
		})
	}
	// 给两个 worker 时间拿到任务；其余四个应该仍是普通队列元素。
	time.Sleep(150 * time.Millisecond)
	if peak := atomic.LoadInt32(&maxSeen); peak > 2 {
		close(release)
		t.Fatalf("in-flight peak = %d, want ≤ 2 (gate leaked)", peak)
	}
	h.importProbeQueueMu.Lock()
	workers := h.importProbeWorkers
	queued := len(h.importProbeQueue)
	h.importProbeQueueMu.Unlock()
	if workers != 2 || queued != n-workers {
		close(release)
		t.Fatalf("workers/queued = %d/%d, want 2/%d", workers, queued, n-workers)
	}
	close(release)
	wg.Wait()
	if peak := atomic.LoadInt32(&maxSeen); peak == 0 {
		t.Fatal("no probe task ran")
	}
}
