package database

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestUsageRequestChainExactGroupingAndLegacyIsolation(t *testing.T) {
	db, err := New("sqlite", filepath.Join(t.TempDir(), "chain.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetUsageLogConfig(UsageLogModeFull, 1000, 3600)
	ctx := context.Background()
	for _, row := range []*UsageLogInput{
		{RequestID: "req-main", AccountID: 1, AttemptIndex: 1, StatusCode: 429, IsRetryAttempt: true, UpstreamRequestID: "up-1", ErrorMessage: "rate limited"},
		{RequestID: "req-main", AccountID: 1, AttemptIndex: 2, StatusCode: 503, IsRetryAttempt: true, UpstreamRequestID: "up-2"},
		{RequestID: "req-main", AccountID: 2, AttemptIndex: 3, StatusCode: 200, UpstreamRequestID: "up-3", InputTokens: 123, OutputTokens: 45},
		{RequestID: "req-other", AccountID: 3, AttemptIndex: 1, StatusCode: 499},
		{RequestID: "req-child", ParentRequestID: "req-main", AccountID: 1, StatusCode: 200},
		{AccountID: 8, StatusCode: 429}, {AccountID: 9, StatusCode: 200},
	} {
		row.Endpoint = "/v1/responses"
		row.Model = "gpt-5.1-codex"
		if err := db.InsertUsageLog(ctx, row); err != nil {
			t.Fatal(err)
		}
	}
	db.FlushUsageLogs()
	var id int64
	if err := db.conn.QueryRowContext(ctx, `SELECT id FROM usage_logs WHERE request_id='req-main' AND attempt_index=3`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	chain, err := db.GetUsageRequestChain(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !chain.Linked || chain.Truncated || chain.Total != 3 || len(chain.Records) != 3 || chain.LogMode != "full" {
		t.Fatalf("unexpected chain: %+v", chain)
	}
	for index, row := range chain.Records {
		if row.AttemptIndex != index+1 || row.UpstreamRequestID != []string{"up-1", "up-2", "up-3"}[index] {
			t.Fatalf("ordering/trace lost: %+v", row)
		}
	}
	if chain.Records[1].AccountID != 1 || chain.Records[2].AccountID != 2 || chain.Records[2].InputTokens != 123 || chain.Records[2].OutputTokens != 45 {
		t.Fatalf("attempt facts lost: %+v", chain.Records)
	}
	if err := db.conn.QueryRowContext(ctx, `SELECT id FROM usage_logs WHERE request_id='' ORDER BY id LIMIT 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	legacy, err := db.GetUsageRequestChain(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Linked || legacy.Total != 1 || len(legacy.Records) != 1 || legacy.Records[0].ID != id {
		t.Fatalf("unrelated legacy records grouped: %+v", legacy)
	}
	if _, err := db.GetUsageRequestChain(ctx, 999999); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing log: %v", err)
	}
	db.SetUsageLogConfig(UsageLogModeErrors, 1000, 3600)
	chain, err = db.GetUsageRequestChain(ctx, chain.Records[0].ID)
	if err != nil || chain.LogMode != "errors" {
		t.Fatalf("current log mode missing: %+v %v", chain, err)
	}
}

func TestUsageRequestChainIncludesCanceledAndBoundsLongChains(t *testing.T) {
	db, err := New("sqlite", filepath.Join(t.TempDir(), "long-chain.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetUsageLogConfig(UsageLogModeFull, 1000, 3600)
	ctx := context.Background()
	for i := 1; i <= 501; i++ {
		if err := db.InsertUsageLog(ctx, &UsageLogInput{RequestID: "req-long", Endpoint: "/v1/responses", Model: "test", AccountID: 1, AttemptIndex: i, StatusCode: 499}); err != nil {
			t.Fatal(err)
		}
	}
	db.FlushUsageLogs()
	var id int64
	if err := db.conn.QueryRowContext(ctx, `SELECT MAX(id) FROM usage_logs`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	chain, err := db.GetUsageRequestChain(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !chain.Truncated || chain.Total != 501 || len(chain.Records) != 500 || chain.Records[0].StatusCode != 499 || chain.Records[499].AttemptIndex != 500 {
		t.Fatalf("unexpected bounded chain: total=%d records=%d truncated=%v", chain.Total, len(chain.Records), chain.Truncated)
	}
}
