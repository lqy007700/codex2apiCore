package admin

import (
	"time"
)

// Explicit response types prevent persistence fields and image caches from
// leaking into this lightweight polling API when the database model grows.
type externalImageJobResult struct {
	ID           int64                         `json:"id"`
	Status       string                        `json:"status"`
	Assets       []externalImageJobResultAsset `json:"assets"`
	ErrorMessage string                        `json:"error_message"`
	Warning      string                        `json:"warning,omitempty"`
	DurationMs   int                           `json:"duration_ms"`
	CreatedAt    time.Time                     `json:"created_at"`
	StartedAt    *time.Time                    `json:"started_at,omitempty"`
	CompletedAt  *time.Time                    `json:"completed_at,omitempty"`
}

type externalImageJobResultAsset struct {
	ExpiresAt       int64  `json:"expires_at,omitempty"`
	DeleteAfterRead bool   `json:"delete_after_read,omitempty"`
	ID              int64  `json:"id"`
	ProxyURL        string `json:"proxy_url"`
	ThumbnailURL    string `json:"thumbnail_url,omitempty"`
	MimeType        string `json:"mime_type"`
	Bytes           int    `json:"bytes"`
	Width           int    `json:"width"`
	Height          int    `json:"height"`
	Model           string `json:"model"`
	OutputFormat    string `json:"output_format"`
}

type externalImageJobResultsRequest struct {
	IDs    []int64 `json:"ids"`
	JobIDs []int64 `json:"job_ids"`
}

type externalImageJobResultsResponse struct {
	Jobs       []externalImageJobResult `json:"jobs"`
	MissingIDs []int64                  `json:"missing_ids,omitempty"`
}

// GetExternalImageJobResult returns only status and output metadata. Inputs and
// cached output Base64 are omitted, including when include_cache=1 is supplied.

// GetExternalImageJobResults returns the result projection for a batch of jobs.
// Failed jobs are returned with status=failed so clients can mark them as
// terminal and skip image downloads; they are never retried by this endpoint.
