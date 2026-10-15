package database

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestMergedRequestsRecoverRetriesBeforePaginationAndFilterLastStatus(t *testing.T) {
	db, err := New("sqlite", filepath.Join(t.TempDir(), "merged.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetUsageLogConfig(UsageLogModeFull, 1000, 3600)
	ctx := context.Background()
	rows := []*UsageLogInput{
		{RequestID: "retry-success", AccountID: 1, AttemptIndex: 1, IsRetryAttempt: true, StatusCode: 429, InputTokens: 10, ErrorMessage: "rate limited"},
		{RequestID: "retry-success", AccountID: 2, AttemptIndex: 2, StatusCode: 200, InputTokens: 20, OutputTokens: 4, ExecutionTrace: `{"elapsed_ms":800}`},
		{RequestID: "failed", AccountID: 1, AttemptIndex: 1, StatusCode: 503},
		{RequestID: "", AccountID: 1, StatusCode: 200}, {RequestID: "", AccountID: 2, StatusCode: 200},
	}
	for _, row := range rows {
		row.Endpoint = "/v1/responses"
		row.Model = "test"
		if err := db.InsertUsageLog(ctx, row); err != nil {
			t.Fatal(err)
		}
	}
	db.FlushUsageLogs()
	f := UsageLogFilter{Start: time.Now().Add(-time.Hour), End: time.Now().Add(time.Hour), Page: 1, PageSize: 20, IncludeCanceled: true}
	all, err := db.ListMergedUsageLogs(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	if all.Total != 4 || len(all.Logs) != 4 {
		t.Fatalf("not merged or legacy combined: %+v", all)
	}
	f.RequestID = "retry-success"
	one := int64(1)
	f.AccountID = &one
	page, err := db.ListMergedUsageLogs(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Logs) != 1 {
		t.Fatalf("account-filtered seed: %+v", page)
	}
	row := page.Logs[0]
	if row.StatusCode != 200 || row.AccountID != 2 || row.AttemptCount != 2 || row.RetryCount != 1 || row.AccountCount != 2 || row.InputTokens != 30 || row.OutputTokens != 4 || row.ElapsedMs == nil || *row.ElapsedMs != 800 {
		t.Fatalf("lost complete chain: %+v", row)
	}
	// Force the matching failed attempt outside the successful attempt's date,
	// then recover the success through its request ID.
	if _, err := db.conn.ExecContext(ctx, `UPDATE usage_logs SET created_at='2020-01-01 00:00:00' WHERE request_id='retry-success' AND attempt_index=1`); err != nil {
		t.Fatal(err)
	}
	f.Start = time.Date(2019, 12, 31, 0, 0, 0, 0, time.UTC)
	f.End = time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)
	page, err = db.ListMergedUsageLogs(ctx, f)
	if err != nil || len(page.Logs) != 1 || page.Logs[0].StatusCode != 200 {
		t.Fatalf("range cropped chain: %+v %v", page, err)
	}
	f.Start = time.Now().AddDate(-10, 0, 0)
	f.End = time.Now().Add(time.Hour)
	f.AccountID = nil
	f.RequestID = ""
	f.ErrorOnly = true
	page, err = db.ListMergedUsageLogs(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.Logs[0].RequestID != "failed" {
		t.Fatalf("failed attempts counted as failed requests: %+v", page)
	}
	f.ErrorOnly = false
	f.StatusFamily = "2xx"
	f.PageSize = 1
	page, err = db.ListMergedUsageLogs(ctx, f)
	if err != nil || page.Total != 3 || len(page.Logs) != 1 {
		t.Fatalf("group pagination: %+v %v", page, err)
	}
	f.Page = 4
	page, err = db.ListMergedUsageLogs(ctx, f)
	if err != nil || page.Total != 3 || len(page.Logs) != 0 {
		t.Fatalf("empty page: %+v %v", page, err)
	}
}
