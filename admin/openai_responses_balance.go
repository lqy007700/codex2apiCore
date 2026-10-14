package admin

import (
	"time"
)

const openAIResponsesBalanceQueryURLCredential = "balance_query_url"
const openAIResponsesBalanceCacheTTL = 30 * time.Second
const openAIResponsesBalanceAttemptTimeout = 8 * time.Second

type openAIResponsesBalanceCacheEntry struct {
	Response  openAIResponsesBalanceResponse
	ExpiresAt time.Time
}

type openAIResponsesBalanceResponse struct {
	Balance   float64 `json:"balance"`
	Unit      string  `json:"unit"`
	Source    string  `json:"source"`
	Unlimited bool    `json:"unlimited,omitempty"`
	QueriedAt string  `json:"queried_at"`
}

// GetOpenAIResponsesBalance queries the upstream platform with the API key that
// is already stored on the account. The key is never returned to the browser.
// GET /api/admin/accounts/:id/openai-responses/balance

// 余额请求会携带账号 API Key 与 custom_headers。跨主机的自定义地址
// 等于把存储密钥送往任意第三方(该密钥被设计为永不回传浏览器),也
// 构成 SSRF 探测入口,因此绝对地址必须与 Base URL 同主机。
