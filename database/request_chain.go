package database

import (
	"context"
	"time"
)

// UsageChainRecord is a bounded diagnostic projection. Account credentials and
// billing fields are deliberately not loaded when following a request chain.
type UsageChainRecord struct {
	ExecutionTrace    string    `json:"execution_trace,omitempty"`
	ID                int64     `json:"id"`
	RequestID         string    `json:"request_id"`
	ParentRequestID   string    `json:"parent_request_id"`
	UpstreamRequestID string    `json:"upstream_request_id"`
	AccountID         int64     `json:"account_id"`
	AccountName       string    `json:"account_name"`
	AttemptIndex      int       `json:"attempt_index"`
	IsRetryAttempt    bool      `json:"is_retry_attempt"`
	StatusCode        int       `json:"status_code"`
	Model             string    `json:"model"`
	EffectiveModel    string    `json:"effective_model"`
	Endpoint          string    `json:"endpoint"`
	UpstreamEndpoint  string    `json:"upstream_endpoint"`
	DurationMs        int       `json:"duration_ms"`
	FirstTokenMs      int       `json:"first_token_ms"`
	InputTokens       int       `json:"input_tokens"`
	OutputTokens      int       `json:"output_tokens"`
	CachedTokens      int       `json:"cached_tokens"`
	Stream            bool      `json:"stream"`
	ViaWebsocket      bool      `json:"via_websocket"`
	UpstreamProxyName string    `json:"upstream_proxy_name"`
	UpstreamErrorKind string    `json:"upstream_error_kind"`
	ErrorMessage      string    `json:"error_message"`
	InternalReason    string    `json:"internal_reason"`
	CreatedAt         time.Time `json:"created_at"`
}

type UsageRequestChain struct {
	RequestID string              `json:"request_id"`
	Linked    bool                `json:"linked"`
	LogMode   string              `json:"log_mode"`
	Records   []*UsageChainRecord `json:"records"`
	Total     int64               `json:"total"`
	Truncated bool                `json:"truncated"`
}

// GetUsageRequestChain follows an exact gateway request ID across all retained
// rows, independently of the list's time/account/error/page filters. Empty IDs
// must never group unrelated legacy requests. Internal child requests with their
// own IDs are not counted as retries of the parent.
func (db *DB) GetUsageRequestChain(ctx context.Context, logID int64) (*UsageRequestChain, error) {
	var requestID string
	if err := db.conn.QueryRowContext(ctx, `SELECT COALESCE(request_id, '') FROM usage_logs WHERE id = $1`, logID).Scan(&requestID); err != nil {
		return nil, err
	}
	result := &UsageRequestChain{RequestID: requestID, Linked: requestID != "", LogMode: db.GetUsageLogMode(), Records: []*UsageChainRecord{}}
	where := `u.id = $1`
	var arg interface{} = logID
	if result.Linked {
		where = `u.request_id = $1 AND u.request_id <> ''`
		arg = requestID
	}
	if err := db.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM usage_logs u WHERE `+where, arg).Scan(&result.Total); err != nil {
		return nil, err
	}
	const limit = 500
	rows, err := db.conn.QueryContext(ctx, `SELECT u.id, COALESCE(u.request_id,''), COALESCE(u.parent_request_id,''), COALESCE(u.upstream_request_id,''), u.account_id, COALESCE(a.name,''),
 COALESCE(u.attempt_index,0), COALESCE(u.is_retry_attempt,false), u.status_code, u.model, COALESCE(u.effective_model,''), u.endpoint, COALESCE(u.upstream_endpoint,''),
 COALESCE(u.duration_ms,0), COALESCE(u.first_token_ms,0), COALESCE(u.input_tokens,0), COALESCE(u.output_tokens,0), COALESCE(u.cached_tokens,0), COALESCE(u.stream,false), COALESCE(u.via_websocket,false),
 COALESCE(u.upstream_proxy_name,''), COALESCE(u.upstream_error_kind,''), COALESCE(u.error_message,''), COALESCE(u.internal_reason,''), u.created_at, COALESCE(u.execution_trace,'')
 FROM usage_logs u LEFT JOIN accounts a ON a.id=u.account_id WHERE `+where+` ORDER BY u.created_at ASC, u.id ASC LIMIT 500`, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		record := &UsageChainRecord{}
		var createdAt interface{}
		if err := rows.Scan(&record.ID, &record.RequestID, &record.ParentRequestID, &record.UpstreamRequestID, &record.AccountID, &record.AccountName, &record.AttemptIndex, &record.IsRetryAttempt, &record.StatusCode, &record.Model, &record.EffectiveModel, &record.Endpoint, &record.UpstreamEndpoint, &record.DurationMs, &record.FirstTokenMs, &record.InputTokens, &record.OutputTokens, &record.CachedTokens, &record.Stream, &record.ViaWebsocket, &record.UpstreamProxyName, &record.UpstreamErrorKind, &record.ErrorMessage, &record.InternalReason, &createdAt, &record.ExecutionTrace); err != nil {
			return nil, err
		}
		record.CreatedAt, err = parseDBTimeValue(createdAt)
		if err != nil {
			return nil, err
		}
		result.Records = append(result.Records, record)
	}
	result.Truncated = result.Total > limit
	return result, rows.Err()
}
