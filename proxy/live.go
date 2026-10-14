package proxy

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"net/http"

	"strings"
	"time"

	"github.com/codex2api/auth"
	"github.com/codex2api/proxy/liveattestation"

	"github.com/google/uuid"
)

const (
	defaultLiveMaxSessionDuration = time.Hour
	liveLeaseRefreshInterval      = 20 * time.Second
	liveUpstreamBodyLimit         = 2 << 20
	liveAttestationHeader         = "X-Oai-Attestation"
	liveCallCacheNamespace        = "live_call"
	liveAccountLeaseNamespace     = "live_acct"
	liveKeyLeaseNamespace         = "live_key"

	liveControllerPending  = "pending"
	liveControllerObserver = "observer"
	liveControllerProxy    = "proxy"
	liveControllerClosed   = "closed"
)

var (
	chatGPTLiveCallsURL        = "https://chatgpt.com/backend-api/codex/realtime/calls?intent=quicksilver&architecture=avas"
	chatGPTLiveSidebandBaseURL = "wss://chatgpt.com/backend-api/codex"
	// liveCallsURLForTest 仅测试替换上游 POST URL。
	liveCallsURLForTest               = ""
	liveMaxSessionDuration            = defaultLiveMaxSessionDuration
	liveAttestationUnavailableMessage = "ChatGPT Live requires an X-Oai-Attestation header on this host. DeviceCheck generation is only available on Apple Silicon with the official ChatGPT app installed."
)

var generateLiveAttestation = defaultGenerateLiveAttestation

func defaultGenerateLiveAttestation(ctx context.Context) (string, error) {
	return liveattestation.NewProvider().Generate(ctx)
}

var liveModelAliases = []string{"gpt-live", "gpt-live-1", "gpt-live-1-mini"}

type liveCallRequest struct {
	SDP     string          `json:"sdp"`
	Session json.RawMessage `json:"session"`
}

type liveCallRecord struct {
	CallID                string    `json:"call_id"`
	CallHash              string    `json:"call_hash"`
	AccountID             int64     `json:"account_id"`
	APIKeyID              int64     `json:"api_key_id"`
	LeaseID               string    `json:"lease_id"`
	Model                 string    `json:"model"`
	CreatedAt             time.Time `json:"created_at"`
	ExpiresAt             time.Time `json:"expires_at"`
	Controller            string    `json:"controller"`
	ControllerOwner       string    `json:"controller_owner,omitempty"`
	AttestationCiphertext string    `json:"attestation_ciphertext,omitempty"`
	UserAgent             string    `json:"user_agent,omitempty"`
	ClientIP              string    `json:"client_ip,omitempty"`
	InboundEndpoint       string    `json:"inbound_endpoint,omitempty"`
	RequestID             string    `json:"request_id,omitempty"`
	UpstreamRequestID     string    `json:"upstream_request_id,omitempty"`
	UpstreamProxyID       int64     `json:"upstream_proxy_id,omitempty"`
	UpstreamProxyName     string    `json:"upstream_proxy_name,omitempty"`
	UsageLogged           bool      `json:"usage_logged,omitempty"`
}

type liveAttestationError struct {
	Reason string
}

func (e *liveAttestationError) Error() string {
	if e == nil || e.Reason == "" {
		return liveAttestationUnavailableMessage
	}
	return e.Reason
}

func liveModelExplicitlyDenied(id string, deny []string) bool {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return false
	}
	for _, item := range deny {
		if strings.ToLower(strings.TrimSpace(item)) == id {
			return true
		}
	}
	return false
}

func hashLiveCallID(callID string) string {
	sum := sha256.Sum256([]byte(callID))
	return fmt.Sprintf("%x", sum[:])
}

// LiveCreate 处理 POST /v1/live 与 Codex realtime/calls。

type liveCallCreated struct {
	sdp    []byte
	callID string
}

func (h *Handler) applyLiveUpstreamHeaders(req *http.Request, account *auth.Account, attestation string, downstream http.Header, apiKey string) error {
	if req == nil || account == nil {
		return nil
	}
	accessToken := account.GetAccessToken()
	identity, err := ResolveCodexOutboundClientIdentity(CodexClientIdentityInput{Account: account, APIKey: apiKey, DeviceConfig: h.deviceCfg, Headers: downstream})
	if err != nil {
		return err
	}
	userAgent, version := identity.UserAgent, identity.Version
	if account.IsCodexAgentIdentity() {
		if assertion, err := account.BuildCodexAgentAssertion(time.Now()); err == nil {
			req.Header.Set("Authorization", assertion)
		} else {
			req.Header.Set("Authorization", "Bearer "+accessToken)
		}
	} else {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/sdp")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Originator", CodexOriginatorForGeneratedUserAgent(userAgent))
	if version != "" {
		req.Header.Set("Version", version)
	}
	if accountID := account.EffectiveAccountID(); accountID != "" {
		req.Header.Set("chatgpt-account-id", accountID)
	}
	applyCodexAllowedForwardHeaders(req, downstream)
	ApplyCodexFingerprintHeaders(req.Header, account, downstream)
	applyAccountCustomHeaders(req, account)
	req.Header.Set("OpenAI-Alpha", "quicksilver=v2")
	req.Header.Del("OpenAI-Beta")
	if strings.TrimSpace(req.Header.Get("session-id")) == "" {
		req.Header.Set("session-id", uuid.NewString())
	}
	if strings.TrimSpace(req.Header.Get("thread-id")) == "" {
		req.Header.Set("thread-id", uuid.NewString())
	}
	if strings.TrimSpace(attestation) != "" {
		req.Header.Set(liveAttestationHeader, attestation)
	}
	return nil
}

func (h *Handler) decryptLiveAttestation(record *liveCallRecord) (string, error) {
	if record == nil || strings.TrimSpace(record.AttestationCiphertext) == "" {
		return "", &liveAttestationError{Reason: "the Live call has no reusable attestation"}
	}
	plain, err := decryptLiveAttestation(record.AttestationCiphertext, h.liveAttestationSecret())
	if err != nil {
		return "", &liveAttestationError{Reason: "the Live call attestation cannot be decrypted on this instance"}
	}
	return plain, nil
}

func (h *Handler) liveAttestationSecret() string {
	if h != nil && h.cfg != nil {
		return strings.TrimSpace(h.cfg.AdminSecret)
	}
	return ""
}

func liveAttestationKey(secret string) [32]byte {
	return sha256.Sum256([]byte("codex2api/live-attestation/v1\x00" + secret))
}

func encryptLiveAttestation(plaintext, secret string) (string, error) {
	key := liveAttestationKey(secret)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	encrypted := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.RawStdEncoding.EncodeToString(encrypted), nil
}

func decryptLiveAttestation(ciphertext, secret string) (string, error) {
	encrypted, err := base64.RawStdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	key := liveAttestationKey(secret)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(encrypted) < gcm.NonceSize() {
		return "", errors.New("encrypted Live attestation is too short")
	}
	plain, err := gcm.Open(nil, encrypted[:gcm.NonceSize()], encrypted[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
