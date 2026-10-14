package admin

import (
	"encoding/json"

	"time"
)

// sub2api 导入支持的范围
const (
	sub2apiScopeAvailable = "available" // 健康且非限流
	sub2apiScopeHealthy   = "healthy"   // 健康（含限流）
	sub2apiScopeAll       = "all"       // 全部
)

type sub2apiCredentialRequest struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
}

type sub2apiImportRequest struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Scope   string `json:"scope"`
}

// sub2api 服务端响应外壳：{ code, message, data }
type sub2apiEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// sub2api accounts/data 接口返回的账号条目（仅取我们关心的字段）
type sub2apiDataAccount struct {
	Name        string                 `json:"name"`
	Platform    string                 `json:"platform"`
	Type        string                 `json:"type"`
	Credentials map[string]interface{} `json:"credentials"`
	Extra       map[string]interface{} `json:"extra,omitempty"`
}

type sub2apiDataPayload struct {
	ExportedAt string               `json:"exported_at"`
	Accounts   []sub2apiDataAccount `json:"accounts"`
}

// sub2api 分页 list 返回的账号状态条目
type sub2apiListAccount struct {
	ID               int64                  `json:"id"`
	Name             string                 `json:"name"`
	Platform         string                 `json:"platform"`
	Status           string                 `json:"status"`
	ErrorMessage     string                 `json:"error_message"`
	RateLimitedAt    *time.Time             `json:"rate_limited_at"`
	RateLimitResetAt *time.Time             `json:"rate_limit_reset_at"`
	Credentials      map[string]interface{} `json:"credentials"`
}

type sub2apiPaginatedItems struct {
	Items    []sub2apiListAccount `json:"items"`
	Total    int64                `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
}

// sub2apiAccountSummary 给前端预览用的合并视图
type sub2apiAccountSummary struct {
	Name             string `json:"name"`
	ChatGPTAccountID string `json:"chatgpt_account_id"`
	Email            string `json:"email"`
	PlanType         string `json:"plan_type"`
	Status           string `json:"status"`
	ErrorMessage     string `json:"error_message,omitempty"`
	RateLimited      bool   `json:"rate_limited"`
	Available        bool   `json:"available"`
	Healthy          bool   `json:"healthy"`
}

type sub2apiPreviewResponse struct {
	Total            int                     `json:"total"`
	OpenAITotal      int                     `json:"openai_total"`
	AvailableCount   int                     `json:"available_count"`
	HealthyCount     int                     `json:"healthy_count"`
	RateLimitedCount int                     `json:"rate_limited_count"`
	ErrorCount       int                     `json:"error_count"`
	OtherPlatform    int                     `json:"other_platform"`
	Accounts         []sub2apiAccountSummary `json:"accounts"`
}

// PreviewSub2APIAccounts 拉取 sub2api 账号情况并返回预览。
// 凭证仅用于本次请求，不落盘。

// ImportFromSub2API 从 sub2api 拉取账号并按 scope 导入到本系统。
// 复用 importAccountsCommon 的 SSE 进度推送、chatgpt_account_id 去重等逻辑。

// sub2apiAccountInternal 后端聚合的中间结构，包含 credentials + 状态。
type sub2apiAccountInternal struct {
	Name             string
	Platform         string
	ChatGPTAccountID string
	Email            string
	PlanType         string
	Status           string
	ErrorMessage     string
	RateLimited      bool
	Credentials      map[string]interface{}
}

// fetchSub2APISummaries 同时调 list 接口拿状态和 data 接口拿明文凭证，按 chatgpt_account_id 合并。

// list 是按 ID 主键的；data 不带 status。我们按 (name, platform) 关联，
// 因为 sub2api 的 name 在同 platform 内是唯一键。

// summarizeSub2API 从聚合数据生成给前端的预览统计 + 列表。

// sub2apiFetchAllList 分页拉完整 list（带 status 字段）。

// 1 万账号上限的硬保护

// sub2apiFetchData 拉 /accounts/data，含明文 credentials。

// sub2apiHTTPGet 发送 GET 请求并解析成 envelope。
