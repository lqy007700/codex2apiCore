package proxy

import (
	"net/http"

	"strings"
	"time"

	"github.com/codex2api/auth"
	"github.com/codex2api/database"
)

// Grok 媒体(生图/生视频)直投官方 REST 端点:
//   POST /images/generations|edits           (同步,响应即产物)
//   POST /videos/generations|edits|extensions (异步,返回 request_id)
//   GET  /videos/{id}                         (客户端轮询状态)
//   GET  /videos/{id}/content                 (产物下载,经网关代理)
//
// 上游有两套 profile:OAuth 账号先走 CLI 网关(cli-chat-proxy),媒体端点不可用
// (403/404/413 等)时回落 xAI 公开 API;API Key 账号直接走 xAI 公开 API。
// 两套 profile 的差异:视频 1.5 模型名带不带 -preview 后缀、图片引用字段名
// (image_url vs url)。
//
// 视频任务是账号亲和的:创建成功后把 request_id → 账号/profile 绑定写入运行态
// 缓存(Redis 部署跨实例、跨重启有效),后续状态/下载必须命中同一账号,否则上游
// 查不到任务。产物按会话选择的模式透传:图片原样透传上游 JSON(data[].url /
// b64_json),视频 done 状态里的上游签名 URL 重写为本网关的 /content 代理地址。

const (
	grokMediaProfileCLI = "cli"
	grokMediaProfileXAI = "xai"

	// maxGrokMediaAttempts caps ordinary finite media retries. A deliberately
	// selected continuous retry bypasses it until the client disconnects.
	maxGrokMediaAttempts = 3

	// grokMaxImageEditInputs Grok 图片编辑上游最多接受的源图数。
	grokMaxImageEditInputs = 3

	grokImagineAliasModel        = "grok-imagine"
	grokImagineImageQualityModel = "grok-imagine-image-quality"
	grokImagineVideoModel        = "grok-imagine-video"
	grokImagineVideo15Model      = "grok-imagine-video-1.5"
	grokImagineVideo15Preview    = "grok-imagine-video-1.5-preview"
	grokDefaultVideoModel        = grokImagineVideo15Model

	grokVideoBindingNamespace = "grok_video_bind"
	// grokVideoBindingTTL 与上游任务的可查询窗口对齐(过期后上游返回 expired)。
	grokVideoBindingTTL = 24 * time.Hour

	grokMediaErrorBodyLimit  = 1 << 20
	grokVideoStatusBodyLimit = 4 << 20
	// grokImagesResponseLimit 生图响应体上限:b64_json 形态下多张 2k 图可达数十 MB。
	grokImagesResponseLimit = 128 << 20
)

// ==================== 模型分类与准入 ====================

func isGrokImageModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return model == grokImagineAliasModel || strings.HasPrefix(model, "grok-imagine-image")
}

func isGrokVideoModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "grok-imagine-video")
}

func isGrokMediaModel(model string) bool {
	return isGrokImageModel(model) || isGrokVideoModel(model)
}

// isMediaOnlyModel 判断模型是否只能走媒体端点(images/videos),不接受文本请求。
func isMediaOnlyModel(model string) bool {
	return isImageOnlyModel(model) || isGrokMediaModel(model)
}

// normalizeGrokMediaModel 把裸别名 grok-imagine 归一到质量档生图模型。
func normalizeGrokMediaModel(model string) string {
	if strings.EqualFold(strings.TrimSpace(model), grokImagineAliasModel) {
		return grokImagineImageQualityModel
	}
	return strings.TrimSpace(model)
}

// grokMediaModelsForAccount 返回账号可服务的媒体模型集。媒体是与文本目录独立的
// 能力轴:账号导入/同步会把文本目录写进 models 白名单,而媒体模型永远不在上游
// /models 目录里,若把"白名单没有媒体条目"当成关闭,所有存量账号都会被误关。
// 因此白名单只有显式出现 grok-imagine 条目时才对媒体收窄;纯文本白名单不影响
// 媒体能力,取默认集并并入账号目录里出现的媒体条目(默认集即候选全集)。
func grokMediaModelsForAccount(account *auth.Account) []string {
	if account == nil || !account.IsGrokAPI() {
		return nil
	}
	declared := make([]string, 0)
	for _, model := range account.GrokModels() {
		if isGrokMediaModel(model) {
			declared = append(declared, normalizeGrokMediaModel(model))
		}
	}
	if len(declared) > 0 {
		return auth.NormalizeAccountModels(declared)
	}
	result := append([]string{}, auth.GrokImageDefaultModelIDs()...)
	result = append(result, auth.GrokVideoDefaultModelIDs()...)
	for _, item := range account.GrokCatalogModels() {
		if !item.Hidden && isGrokMediaModel(item.ModelID) {
			result = append(result, item.ModelID)
		}
	}
	return auth.NormalizeAccountModels(result)
}

// grokMediaAccountFilter 媒体请求的账号过滤器:仅 Grok 账号,账号级模型映射先行,
// 模型级冷却生效。

// grokMediaChannelBlocked 判断当前 Key 的上游渠道限定是否排除 Grok 媒体能力。

// grokMediaPreferredAccountFilter 在媒体过滤器上再收一层:优先付费凭据。
// free 计划的 OAuth 号打媒体端点必 403,先挑付费号能避免把重试额度和 6h 模型
// 冷却烧在注定失败的账号上;API Key 账号按量计费视同付费。

// nextGrokMediaAccount 两层选号:先付费凭据,挑不到再放开到全部候选
// (与生图路径的 plus 优先层级同构)。两层都过 scope 预算闸门。

// ==================== 上游 profile 与请求投递 ====================

type grokMediaProfile struct {
	Kind    string
	BaseURL string
}

// grokMediaProfilesForAccount 按凭据形态返回有序的上游候选:OAuth 先 CLI 网关
// 再回落 xAI 公开 API(OAuth Bearer 在两边都有效);API Key 只有 xAI 公开 API。

// grokMediaProfileForKind 取指定 kind 的 profile;绑定的 profile 已不可用(如账号
// 凭据形态变化)时回退第一个可用 profile。

// mapGrokMediaModelForProfile 按上游 profile 归一模型名:grok-imagine-video-1.5
// 在 xAI 公开 API 上以 -preview 后缀发布,CLI 网关则用不带后缀的名字。

// grokMediaShouldFallbackProfile 判断状态码是否值得换下一个上游 profile 重试
// (CLI 网关不认媒体端点或 body 体积超限时回落 xAI 公开 API)。

// doGrokMediaRequest 向指定 profile 的媒体端点发一次请求。extra 里的头最后应用
// (用于 Range 透传等逐请求覆盖)。

// CLI 身份头只发给 CLI 网关。xAI 公开 API 识别到 x-grok-client-* 会按 CLI
// 客户端的 Zero Data Retention 政策处理(实测报 "Zero Data Retention teams
// must provide output.upload_url"),媒体请求直接 400。

// stripGrokCLIIdentityHeaders 移除 Grok CLI 专属的身份/会话头,保留 Authorization、
// Content-Type、Accept 等通用头。

type grokMediaSendResult struct {
	Resp    *http.Response
	Profile grokMediaProfile
	Model   string
}

// grokMediaInvalidSuccessSelected handles failures hidden inside HTTP 200:
// truncated bodies, error envelopes, or missing media identifiers. Catch-all
// retries every such upstream failure; selective mode can match transport,
// exact error-code, or context categories from the original body/error.
func grokMediaInvalidSuccessSelected(policy database.ContinuousRetryPolicy, body []byte, readErr error) bool {
	if isExplicitUpstreamCyberPolicy(body) || isExplicitUpstreamCyberPolicyError(readErr) {
		return false
	}
	if readErr != nil {
		return continuousRetryLimitForRequestError(readErr, 0, policy) == -1
	}
	return policy.CatchesAllUpstreamFailures() || continuousRetryHTTPSelected(policy, http.StatusOK, body)
}

// sendGrokMediaWithProfiles 依次尝试账号的上游 profile;每个 profile 的请求体和
// 模型名由 buildBody 按 profile 差异构造。

// applyGrokMediaCooldown 媒体路径的上游错误 → 冷却映射。与文本路径不同,媒体
// 能力的失败尽量只做模型级隔离,不牵连同账号的文本调度;401 例外(凭据问题
// 两边一致),交给通用映射走 token 强刷。

// 凭据被接受但媒体能力不可用(免费号/无 billing/端点未开放):模型级长隔离。

// ==================== 生图 ====================

type grokImagesParams struct {
	N           int64
	AspectRatio string
	Resolution  string
	Quality     string
}

// OpenAI 客户端只有 size 语义:未显式给 resolution 时,2048 边长以上映射到 2k 档。

// buildGrokImagesBody 构造发往 Grok 媒体上游的生图请求体。上游不接受 size 字段
// (size 语义已在参数提取时折算进 resolution 档位),源图统一放 images 数组。

// grokImagesUsageLogInfo 从 OpenAI Images 形状的响应(data[].url|b64_json)提取
// 用量维度:计数必有;b64 形态可解出尺寸/字节数,url 形态只有计数与格式。

// forwardGrokImagesRequest 把 /v1/images/* 请求直投 Grok 媒体上游,成功响应按
// OpenAI Images 形状原样透传(data[].url / b64_json 由上游按 response_format 决定)。

// 上游 200 但没有任何产物:按可疑失败换号重试。

// 媒体按张计费(image_count),不再把张数塞进输出 token 冒充用量;
// 上游自报的成本(usage.cost_in_usd_ticks)优先作为账号成本。

// ==================== 生视频:任务创建 ====================

// VideosGenerations 处理 POST /v1/videos/generations(文生视频/图生视频)。

// VideosEdits 处理 POST /v1/videos/edits(视频编辑)。

// VideosExtensions 处理 POST /v1/videos/extensions(视频延展)。

// buildGrokVideoBody 从下游请求体白名单拷贝视频字段,并按 profile 归一图片引用
// 的字段名(CLI 网关用 image_url,xAI 公开 API 用 url)。output/storage_options
// 等未支持字段一律丢弃。

// grokMediaImageRef 接受字符串或 {url|image_url} 对象,输出 profile 对应字段名的对象。

// 上游实测:edits/extensions 只有 grok-imagine-video 支持,1.5 系列会
// 400 "not supported for this model",默认模型按操作分开。

// ==================== 生视频:任务绑定 ====================

type grokVideoBinding struct {
	AccountID int64  `json:"account_id"`
	APIKeyID  int64  `json:"api_key_id"`
	Profile   string `json:"profile"`
	Model     string `json:"model"`
	CreatedAt int64  `json:"created_at"`
	// 以下字段供完成时结算:下游请求模型/映射后生效模型(与提交行一致的计费口径),
	// 以及请求时长(上游状态体缺 video.duration 时的兜底计费秒数)。
	RequestModel     string `json:"request_model,omitempty"`
	EffectiveModel   string `json:"effective_model,omitempty"`
	RequestedSeconds int    `json:"requested_seconds,omitempty"`
}

// resolveGrokVideoBinding 校验 request_id 的归属(必须是创建它的 API Key)并取回
// 绑定账号;失败统一回 404,不泄露任务是否存在。

// ==================== 生视频:状态查询与产物代理 ====================

// VideosStatus 代理 GET /v1/videos/:request_id 状态查询。请求命中创建时绑定的
// 账号;done 状态里的上游资产 URL 重写为本网关的 /content 代理地址(上游签名
// URL 会过期,统一走网关下载)。

// 终态(done/failed/expired)在首次被观察到时结算一次;引擎过载等失败可能以
// 非 2xx 携带 failed 状态体返回,因此先于状态码分支处理。

// 任务进行中上游以 202 携带 {"status":"pending","progress":N} 返回,
// 与 200 一样是合法状态体;统一以 200 透传,轮询客户端只看 body.status。

// rewriteGrokVideoContentURL 把状态响应里的 video.url 换成本网关的 content 代理地址。

// VideosContent 代理 GET /v1/videos/:request_id/content 产物下载:先查状态拿上游
// 签名 URL(白名单 host、禁跳转)匿名拉取,拿不到再回退带凭据的上游 content 端点。
// 两条路径都透传 Range。

// isTrustedGrokVideoAssetURL 校验上游签名资产 URL:仅 https、无 userinfo、默认端口,
// host 限官方资产域及其子域。

// streamGrokVideoAsset 匿名拉取上游签名资产 URL 并流式转发;返回 false 表示未写出
// 任何响应,调用方可回退带凭据的下载路径。

// 浅拷贝共享连接池但独立 CheckRedirect:签名 URL 域外跳转一律拒绝(防 SSRF)。
