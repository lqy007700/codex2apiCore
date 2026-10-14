package admin

import (
	"sync"
	"time"

	"github.com/codex2api/auth"
)

// ==================== Grok Device OAuth 会话 ====================
//
// xAI / Grok CLI 正式授权路径是 RFC 8628 Device Code。
// 浏览器 redirect PKCE 会落到「复制代码到 Grok Build」页，token 受众不对时
// 会被用量探针误封，因此管理台只暴露 device start/poll。

const grokDeviceSessionTTL = 30 * time.Minute

type grokDeviceSession struct {
	DeviceCode    string
	UserCode      string
	TokenEndpoint string
	ProxyURL      string
	Name          string
	BaseURL       string
	Models        []string
	Interval      int
	ExpiresAt     time.Time
	CreatedAt     time.Time
}

type grokDeviceSessionStore struct {
	mu       sync.Mutex
	sessions map[string]*grokDeviceSession
}

var globalGrokDeviceStore = &grokDeviceSessionStore{sessions: make(map[string]*grokDeviceSession)}

func init() {
	go globalGrokDeviceStore.cleanupLoop()
}

func (s *grokDeviceSessionStore) set(id string, sess *grokDeviceSession) {
	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()
}

func (s *grokDeviceSessionStore) get(id string) (*grokDeviceSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok || time.Since(sess.CreatedAt) > grokDeviceSessionTTL {
		return nil, false
	}
	if !sess.ExpiresAt.IsZero() && time.Now().After(sess.ExpiresAt) {
		return nil, false
	}
	return sess, true
}

func (s *grokDeviceSessionStore) delete(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

func (s *grokDeviceSessionStore) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		for id, sess := range s.sessions {
			if time.Since(sess.CreatedAt) > grokDeviceSessionTTL ||
				(!sess.ExpiresAt.IsZero() && time.Now().After(sess.ExpiresAt)) {
				delete(s.sessions, id)
			}
		}
		s.mu.Unlock()
	}
}

// StartGrokDeviceAuth 启动 xAI Device Code 授权
// POST /api/admin/accounts/grok/oauth/device/start

// PollGrokDeviceAuth 轮询 Device 授权；成功则入库
// POST /api/admin/accounts/grok/oauth/device/poll

// 交互式重授权命中既有账号(含回收站)时更新凭据而非报"已存在"。

// GenerateGrokAuthURL 旧 PKCE 接口：引导改用 device flow。

// ExchangeGrokOAuthCode 旧 code 兑换接口：引导改用 device flow。

type createGrokOAuthAccountInput struct {
	Name          string
	ProxyURL      string
	BaseURL       string
	Models        []string
	Token         *auth.GrokTokenData
	Subject       string
	Email         string
	TokenEndpoint string
	Source        string
	// ReauthorizeExisting 开启"重授权即更新"语义:凭据身份命中既有账号时,
	// 更新该账号的凭据(回收站账号顺带复活),而不是报"已存在"。
	// 关闭时命中既有身份直接报错;身份栅栏含回收站账号,报错会让"删过的号
	// 重新导入"走进死胡同,所以交互式授权与批量导入路径都应打开。
	ReauthorizeExisting bool
}

type createGrokOAuthAccountResult struct {
	ID    int64
	Email string
	// Updated 表示命中既有账号并已更新凭据(而非新建)。
	Updated bool
	// Revived 表示既有账号原先在回收站,本次已复活。
	Revived bool
}

// access_token 的 tier claim 是套餐权威来源；缺失/无效 tier 保持空白。

// OAuth 默认走 chat-proxy（与官方 CLI 聊天路径一致）

// reauthorizeGrokOAuthAccount 把同一 OAuth 身份的新凭据合并进占位账号:
// 回收站账号复活,正常账号原地更新 token(对齐 Codex OAuth 重授权语义)。
// base_url/models/name/proxy 只有本次授权显式提供时才覆盖既有配置。

// token 不带 email claim 时 credentials["email"] 只是 subject 兜底,
// 不能拿它覆盖既有账号的真实 email。

// 无论账号原先是否在内存池(回收站账号不在),都按"移除后重载"拿到最新凭据。
