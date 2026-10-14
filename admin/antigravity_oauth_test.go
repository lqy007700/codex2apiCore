package admin

import (
	"time"
)

func newAntigravityOAuthTestSession(id, state string) *antigravityOAuthSession {
	return &antigravityOAuthSession{
		ID:          id,
		State:       state,
		RedirectURI: "http://127.0.0.1:45678" + antigravityOAuthCallbackPath,
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(time.Hour),
		Status:      "waiting",
	}
}

func sessionStatus(session *antigravityOAuthSession) string {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.Status
}

// A claimed denial cannot be replayed.
