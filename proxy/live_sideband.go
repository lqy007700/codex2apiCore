package proxy

import (
	"context"

	"net/http"
	"net/url"

	"github.com/codex2api/auth"

	"github.com/gorilla/websocket"
)

// liveSidebandDialForTest 仅测试替换上游 sideband 拨号。
var liveSidebandDialForTest func(ctx context.Context, record *liveCallRecord, headers http.Header) (*websocket.Conn, error)

// LiveSideband 代理 GET /v1/live/:call_id 与 Codex call_id 控制面 WebSocket。

// copyLiveSideband 在两个 WebSocket 连接之间双向复制一条数据流。

// Gorilla 允许 Close 与读写并发，直接关闭可同时解除两端阻塞，
// 也避免与 SetReadDeadline/SetWriteDeadline 竞争。

func (h *Handler) liveSidebandHeaders(ctx context.Context, record *liveCallRecord) (http.Header, error) {
	account := h.liveAccountForRecord(record)
	if account == nil {
		return nil, errLiveCallNotFound
	}
	attestation, err := h.decryptLiveAttestation(record)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://chatgpt.com/backend-api/codex/"+url.PathEscape(record.CallID), nil)
	if err != nil {
		return nil, err
	}
	if err := h.applyLiveUpstreamHeaders(req, account, attestation, nil, ""); err != nil {
		return nil, err
	}
	req.Header.Del("Content-Type")
	req.Header.Del("Accept")
	return req.Header, nil
}

func (h *Handler) liveAccountForRecord(record *liveCallRecord) *auth.Account {
	if h == nil || h.store == nil || record == nil {
		return nil
	}
	if session := h.liveCalls().localSession(record.CallHash); session != nil && session.account != nil {
		return session.account
	}
	return h.store.FindByID(record.AccountID)
}

func (s *liveCallStore) localSession(callHash string) *liveSession {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[callHash]
}
