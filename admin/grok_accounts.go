package admin

import (
	"encoding/json"

	"time"
)

// addGrokAccountReq 是新增/更新 Grok 账号的请求体。
// AuthKind 取 api_key / oauth；oauth 时 AuthJSON 为 Grok CLI auth.json 原文，
// api_key 时 APIKey 为 xAI API Key。
type addGrokAccountReq struct {
	Name         string   `json:"name"`
	AuthKind     string   `json:"auth_kind"`
	AuthJSON     string   `json:"auth_json"`
	APIKey       string   `json:"api_key"`
	BaseURL      string   `json:"base_url"`
	Models       []string `json:"models"`
	ModelMapping string   `json:"model_mapping"`
	ProxyURL     string   `json:"proxy_url"`
	// GroupIDs 让添加时就把新账号绑进指定分组（UpdateGrokAccount 复用本结构时忽略该字段，
	// 编辑分组走账号编辑抽屉的 group_ids）。
	GroupIDs json.RawMessage `json:"group_ids"`
}

// grokCredentialsFromRequest 校验请求并构造待入库的 credentials map。
// 返回的 email 用于列表展示（OAuth 取 subject，API Key 取脱敏 key）。

type grokImportMeta struct {
	Disabled        bool
	DisabledPresent bool
	Subject         string
	FamilyID        string
}

// OAuth 凭据若带 access_token，会在下方从 JWT tier claim 识别真实套餐；
// 缺失/无效 tier 保持空值。API Key 账号无订阅档位，单独标记为 "api"。

// 取第一条可用凭据（多 scope 文件通常首条即目标账号）

// 凭据文件带真实 email 时优先用它（CPA / 带 email 的 auth.json）。

// AddGrokAccount 新增一个 Grok 上游账号（POST /api/admin/accounts/grok）。

// 分组校验放在建号之前：分组 ID 打错时不该留下一个没绑上分组的账号。

// 身份栅栏含回收站账号:提示持有者 ID,避免"列表明明是空的却说已存在"的死胡同。

// 异步 billing 探针：拉取套餐/周月额度，避免被 ChatGPT wham 误封

// UpdateGrokAccount 更新 Grok 账号的可编辑配置（PATCH /api/admin/accounts/:id/grok）。
// 仅更新 base_url / models / model_mapping / proxy / api_key（凭据留空则不改）。

// proxy_url 是独立列，不在 credentials 里；UpdateCredentials 不会写它。
// 编辑是整体重写语义（空值即清空代理），须单独持久化，否则代理只落到内存
// store，重载 / 重启 / 后台刷新后被 DB 旧值覆盖，表现为"添加代理不生效"。

// An identity change advances credential generation and invalidates prior
// facts/capabilities. Rebuild the fenced catalog and minimal three-protocol
// capability set asynchronously; configuration-only edits are deduplicated
// by fresh capability rows.

type batchUpdateGrokModelsReq struct {
	IDs    []int64  `json:"ids"`
	Models []string `json:"models"`
}

// BatchUpdateGrokModels 批量替换 Grok 账号的模型列表
// （POST /api/admin/accounts/grok/batch-models）。
// 空数组 = 清空模型列表（未声明，仅 grok 渠道 Key 可调度）；非空则整体替换。
// 非 Grok / 不存在的 ID 计入 failed，不中断整批。

// FetchGrokModels 用请求内凭据或已保存账号凭据探测 Grok 上游模型目录
// （POST /api/admin/accounts/grok/models）。

// 构造临时账号做探测，不入库、不入池

// OAuth 凭据无 AT 时先刷一次

// grokPlanTypeFromCredentials 优先读取 access_token 的 tier claim，再兼容已有
// plan_type 展示值；API Key 账号没有订阅 tier，其余缺失/无效值保持空白。

// grokAccountFromCredentials 从入库用的 credentials map 构造内存态 Account，
// 供单条添加与批量文件导入共用。models/model_mapping/base_url/email 由调用方按需覆写。

// 与 credentials 保持一致：OAuth 使用 tier 映射，API Key 为 api。

// batchImportGrokReq 是 CPA / auth.json 文件批量导入的请求体。
// Files 每项是一个凭据文件的原始 JSON 内容（CPA 单对象或 Grok CLI auth.json 均可）。
type batchImportGrokReq struct {
	Files    []string `json:"files"`
	BaseURL  string   `json:"base_url"`
	Models   []string `json:"models"`
	ProxyURL string   `json:"proxy_url"`
	// ImportProxy 打开后采用文件内携带的代理，并把它们注册进代理池；关闭时文件
	// 里的 proxy_url 一律忽略，全部账号用表单填的代理。
	ImportProxy bool `json:"import_proxy"`
	// GroupIDs 让导入时就把新账号绑进指定分组；跳过的重复账号不受影响。
	GroupIDs json.RawMessage `json:"group_ids"`
}

type grokBatchImportItem struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	ID    int64  `json:"id,omitempty"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	// Updated/Revived 标记该条命中既有身份:凭据已合并进账号 ID(Revived 表示
	// 该账号原在回收站,本次已复活),而非新建。
	Updated bool `json:"updated,omitempty"`
	Revived bool `json:"revived,omitempty"`
}

const grokBatchImportMaxFiles = 5000

// 批量导入的整体超时按文件数缩放：整个循环串行地逐个文件落库，写死一个常量会让
// 每个文件分到的预算随批量增大而被摊薄（5000 个文件时只剩 12ms/个），数据库稍有抖动
// 就会中途超时、后面的文件全部报 context deadline exceeded。封顶是为了不让一个超大
// 请求无限期占住连接。
const (
	grokBatchImportBaseTimeout    = 30 * time.Second
	grokBatchImportPerFileTimeout = 100 * time.Millisecond
	grokBatchImportMaxTimeout     = 10 * time.Minute
)

// BatchImportGrokAccounts 批量导入 Grok 凭据文件（POST /api/admin/accounts/grok/import）。
// 每个文件独立解析入库，按 subject / refresh_token 去重（批内 + 与现有账号）。

// 已存在 Grok 账号的 principal / credential-family 集合。family ID 不随 RT
// 轮换变化，避免同一身份因 refresh token 已更新而被重复导入。

// 代理必须在任何账号落库之前注册进代理表并同步到内存代理池，顺序理由见
// registerImportedProxyBindings。索引与 req.Files 对齐，逐文件取用。

// 文件内代理优先，缺失/非法时回落表单值。

// 身份栅栏含回收站账号:命中时合并凭据(回收站复活)而非跳过,
// 否则删过的号重新导入永远失败(issue #602)。credentials 里的
// base_url/models 只在显式提供时存在,不会覆盖既有配置。
//
// 代理传空 = 保留账号原绑定。文件带来的代理是被动数据，不覆盖目标端
// 已有的绑定（那里可能已经做过精细分配）；表单填的代理是操作员的显式
// 换绑意图，维持既有覆盖语义。
