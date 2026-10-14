package admin

import (
	"context"

	"crypto/subtle"

	"encoding/json"

	"net"
	"net/http"

	"strings"
	"sync"
	"time"
)

const (
	antigravityOAuthSessionTTL   = 15 * time.Minute
	antigravityOAuthCallbackPath = "/oauth-callback"
)

type antigravityOAuthSession struct {
	mu sync.Mutex

	ID             string
	State          string
	CodeVerifier   string
	RedirectURI    string
	OAuthClientKey string
	ProxyURL       string
	Name           string
	GroupIDs       []int64
	CreatedAt      time.Time
	ExpiresAt      time.Time

	Status    string
	AccountID int64
	Email     string
	Warning   string
	Error     string
	Server    *http.Server
	Listener  net.Listener
}

type antigravityOAuthSessionStore struct {
	mu       sync.Mutex
	sessions map[string]*antigravityOAuthSession
}

var globalAntigravityOAuthSessions = &antigravityOAuthSessionStore{
	sessions: make(map[string]*antigravityOAuthSession),
}

func init() {
	go globalAntigravityOAuthSessions.cleanupLoop()
}

func (s *antigravityOAuthSessionStore) add(session *antigravityOAuthSession) {
	s.mu.Lock()
	s.sessions[session.ID] = session
	s.mu.Unlock()
}

func (s *antigravityOAuthSessionStore) get(id string) (*antigravityOAuthSession, bool) {
	id = strings.TrimSpace(id)
	s.mu.Lock()
	session, ok := s.sessions[id]
	if !ok {
		s.mu.Unlock()
		return nil, false
	}
	if time.Now().After(session.ExpiresAt) {
		delete(s.sessions, id)
		s.mu.Unlock()
		// Expiry is lazy (the background janitor runs once per minute), but an
		// expired session must release its loopback listener as soon as it is
		// observed by a request.
		go closeAntigravityOAuthListener(session)
		return nil, false
	}
	s.mu.Unlock()
	return session, true
}

func (s *antigravityOAuthSessionStore) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		s.mu.Lock()
		for id, session := range s.sessions {
			if now.After(session.ExpiresAt) {
				delete(s.sessions, id)
				go closeAntigravityOAuthListener(session)
			}
		}
		s.mu.Unlock()
	}
}

type antigravityOAuthExchangeInput struct {
	ID             string
	CodeVerifier   string
	RedirectURI    string
	OAuthClientKey string
	ProxyURL       string
	Name           string
	GroupIDs       []int64
}

func (s *antigravityOAuthSessionStore) claim(id, state string) (antigravityOAuthExchangeInput, bool) {
	session, ok := s.get(id)
	if !ok {
		return antigravityOAuthExchangeInput{}, false
	}
	session.mu.Lock()
	// State is an opaque CSRF nonce. Do not trim or otherwise normalize it:
	// accepting a whitespace-mutated value weakens the exact callback binding.
	expired := time.Now().After(session.ExpiresAt)
	if state == "" || subtle.ConstantTimeCompare([]byte(session.State), []byte(state)) != 1 || session.Status != "waiting" || expired {
		session.mu.Unlock()
		if expired {
			go closeAntigravityOAuthListener(session)
		}
		return antigravityOAuthExchangeInput{}, false
	}
	session.Status = "processing"
	input := antigravityOAuthExchangeInput{
		ID: session.ID, CodeVerifier: session.CodeVerifier, RedirectURI: session.RedirectURI,
		OAuthClientKey: session.OAuthClientKey, ProxyURL: session.ProxyURL, Name: session.Name,
		GroupIDs: append([]int64(nil), session.GroupIDs...),
	}
	session.mu.Unlock()
	return input, true
}

func closeAntigravityOAuthListener(session *antigravityOAuthSession) {
	if session == nil {
		return
	}
	session.mu.Lock()
	server := session.Server
	listener := session.Listener
	session.Server = nil
	session.Listener = nil
	session.mu.Unlock()
	if server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_ = server.Shutdown(ctx)
		cancel()
	}
	// Shutdown closes listeners owned by Serve, but an expiry/cancel can race
	// with the goroutine that is about to call Serve. Close the raw listener as
	// well so that pre-Serve and partially-started servers cannot leak a port.
	if listener != nil {
		_ = listener.Close()
	}
}

type antigravityOAuthStartRequest struct {
	Name           string          `json:"name"`
	ProxyURL       string          `json:"proxy_url"`
	OAuthClientKey string          `json:"oauth_client_key"`
	GroupIDs       json.RawMessage `json:"group_ids"`
}

// An OAuth denial is still an attacker-controlled callback. Require and
// atomically claim the exact state before mutating the session.

// antigravityOAuthHTTPHostMatches validates the Host header when a callback
// arrived through the loopback listener. Direct unit invocations may omit
// Host, but a supplied value must match the exact host:port bound to the
// session redirect URI.

// antigravityOAuthCallbackURLMatches binds a pasted callback to the exact
// loopback redirect that was allocated for this session. In particular, do
// not accept another port, a path variant, user-info, or a fragment that was
// never sent to the callback server.

type antigravityOAuthCompleteRequest struct {
	SessionID   string `json:"session_id"`
	CallbackURL string `json:"callback_url"`
}

type antigravityOAuthAccountResult struct {
	ID      int64
	Email   string
	Warning string
}
