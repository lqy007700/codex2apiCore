package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type MergedUsageLog struct {
	UsageChainRecord
	AttemptCount    int    `json:"attempt_count"`
	RetryCount      int    `json:"retry_count"`
	AccountCount    int    `json:"account_count"`
	RecordCount     int    `json:"record_count"`
	MissingAttempts bool   `json:"missing_attempts"`
	ElapsedMs       *int64 `json:"elapsed_ms"`
}
type MergedUsagePage struct {
	Logs    []*MergedUsageLog `json:"logs"`
	Total   int64             `json:"total"`
	LogMode string            `json:"log_mode"`
}

// Filters select candidate requests using any attempt. Status filters are
// applied to each request's last retained record, after recovering retries
// outside the list time/account filters. Empty request IDs remain independent.
func (db *DB) ListMergedUsageLogs(ctx context.Context, f UsageLogFilter) (*MergedUsagePage, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 500 {
		f.PageSize = 20
	}
	status := UsageLogFilter{StatusCode: f.StatusCode, StatusFamily: f.StatusFamily, ErrorOnly: f.ErrorOnly, IncludeCanceled: true}
	f.StatusCode = 0
	f.StatusFamily = ""
	f.ErrorOnly = false
	f.IncludeCanceled = true
	seed, args := db.buildUsageLogWhere(f)
	lastWhere, args := buildUsageLogFilterWhere("u.position = 1", args, status, false)
	// Rank narrow rows; read error bodies and execution JSON only for the page.
	lastWhere = strings.ReplaceAll(lastWhere, usageLogErrorPredicate, "u.has_error")
	cte := `WITH matched AS (
 SELECT DISTINCT COALESCE(u.request_id,'') AS request_id, CASE WHEN COALESCE(u.request_id,'')='' THEN u.id ELSE 0 END AS legacy_id FROM usage_logs u WHERE ` + seed + `
 ), all_rows AS (
 SELECT u.id,u.request_id,u.account_id,u.attempt_index,u.status_code,u.is_retry_attempt,u.created_at,u.input_tokens,u.output_tokens,u.cached_tokens, (` + usageLogErrorPredicate + `) AS has_error, CASE WHEN COALESCE(u.request_id,'')='' THEN 'l:' || CAST(u.id AS TEXT) ELSE 'r:' || u.request_id END AS group_key,
 ROW_NUMBER() OVER (PARTITION BY CASE WHEN COALESCE(u.request_id,'')='' THEN 'l:' || CAST(u.id AS TEXT) ELSE 'r:' || u.request_id END ORDER BY u.created_at DESC,u.id DESC) AS position
 FROM matched m JOIN usage_logs u ON (m.request_id<>'' AND u.request_id=m.request_id AND u.request_id<>'') OR (m.request_id='' AND u.id=m.legacy_id)
 ), selected AS (SELECT u.* FROM all_rows u WHERE ` + lastWhere + `) `
	result := &MergedUsagePage{Logs: []*MergedUsageLog{}, LogMode: db.GetUsageLogMode()}
	if err := db.conn.QueryRowContext(ctx, cte+`SELECT COUNT(*) FROM selected`, args...).Scan(&result.Total); err != nil {
		return nil, err
	}
	if result.Total == 0 {
		return result, nil
	}
	limitIndex := len(args) + 1
	query := cte + fmt.Sprintf(`, page AS (SELECT * FROM selected ORDER BY created_at DESC,id DESC LIMIT $%d OFFSET $%d), stats AS (
 SELECT r.group_key, COUNT(*) AS records, COUNT(DISTINCT CASE WHEN r.attempt_index>0 THEN r.attempt_index END) AS attempts,
 COUNT(DISTINCT CASE WHEN r.attempt_index>1 THEN r.attempt_index END) AS retries, COUNT(DISTINCT CASE WHEN r.account_id>0 THEN r.account_id END) AS accounts,
 MAX(COALESCE(r.attempt_index,0)) AS highest, SUM(COALESCE(r.input_tokens,0)) AS inputs,SUM(COALESCE(r.output_tokens,0)) AS outputs,SUM(COALESCE(r.cached_tokens,0)) AS cached
 FROM all_rows r JOIN page p ON p.group_key=r.group_key GROUP BY r.group_key
 ) SELECT p.id,COALESCE(p.request_id,''),p.account_id,COALESCE(a.name,''),l.model,COALESCE(l.effective_model,''),p.status_code,
 COALESCE(p.is_retry_attempt,false),COALESCE(p.attempt_index,0),COALESCE(l.duration_ms,0),COALESCE(l.first_token_ms,0),
 COALESCE(l.error_message,''),COALESCE(l.upstream_error_kind,''),COALESCE(l.stream,false),COALESCE(l.via_websocket,false),p.created_at,
 s.records,s.attempts,s.retries,s.accounts,s.highest,s.inputs,s.outputs,s.cached,COALESCE(l.execution_trace,'')
 FROM page p JOIN usage_logs l ON l.id=p.id JOIN stats s ON p.group_key=s.group_key LEFT JOIN accounts a ON a.id=p.account_id ORDER BY p.created_at DESC,p.id DESC`, limitIndex, limitIndex+1)
	pageArgs := append(append([]interface{}{}, args...), f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := db.conn.QueryContext(ctx, query, pageArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		row := &MergedUsageLog{}
		var rawTime interface{}
		var highest int
		var trace string
		if err := rows.Scan(&row.ID, &row.RequestID, &row.AccountID, &row.AccountName, &row.Model, &row.EffectiveModel, &row.StatusCode, &row.IsRetryAttempt, &row.AttemptIndex, &row.DurationMs, &row.FirstTokenMs, &row.ErrorMessage, &row.UpstreamErrorKind, &row.Stream, &row.ViaWebsocket, &rawTime, &row.RecordCount, &row.AttemptCount, &row.RetryCount, &row.AccountCount, &highest, &row.InputTokens, &row.OutputTokens, &row.CachedTokens, &trace); err != nil {
			return nil, err
		}
		row.CreatedAt, err = parseDBTimeValue(rawTime)
		if err != nil {
			return nil, err
		}
		row.MissingAttempts = highest > row.AttemptCount
		var snapshot struct {
			ElapsedMs *int64 `json:"elapsed_ms"`
		}
		if json.Unmarshal([]byte(trace), &snapshot) == nil {
			row.ElapsedMs = snapshot.ElapsedMs
		}
		result.Logs = append(result.Logs, row)
	}
	return result, rows.Err()
}
