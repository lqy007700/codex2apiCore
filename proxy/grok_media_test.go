package proxy

import (
	"bytes"

	"io"

	"net/http/httptest"

	"testing"

	"github.com/codex2api/auth"
	"github.com/codex2api/cache"
	"github.com/codex2api/database"
	"github.com/gin-gonic/gin"
)

func TestGrokMediaModelClassification(t *testing.T) {
	for model, wantImage := range map[string]bool{
		"grok-imagine":               true,
		"grok-imagine-image":         true,
		"grok-imagine-image-quality": true,
		"grok-imagine-video":         false,
		"grok-4.6":                   false,
		"gpt-image-2":                false,
	} {
		if got := isGrokImageModel(model); got != wantImage {
			t.Errorf("isGrokImageModel(%q) = %v, want %v", model, got, wantImage)
		}
	}
	if !isGrokVideoModel("grok-imagine-video-1.5-preview") || isGrokVideoModel("grok-imagine-image") {
		t.Error("isGrokVideoModel misclassified")
	}
	if !isMediaOnlyModel("gpt-image-2") || !isMediaOnlyModel("grok-imagine-video") || isMediaOnlyModel("grok-4.6") {
		t.Error("isMediaOnlyModel misclassified")
	}
	if got := normalizeGrokMediaModel("grok-imagine"); got != grokImagineImageQualityModel {
		t.Errorf("normalizeGrokMediaModel(grok-imagine) = %q", got)
	}
}

// 媒体模型集是独立能力轴:未声明白名单或只声明文本模型的账号得到默认媒体集
// (账号导入会把文本目录写进白名单,不能因此关闭媒体);白名单里显式出现
// grok-imagine 条目时才对媒体收窄。

func newGrokMediaTestHandler(t *testing.T, account *auth.Account) *Handler {
	t.Helper()
	store := auth.NewStore(nil, nil, &database.SystemSettings{MaxConcurrency: 2})
	store.AddAccount(account)
	handler := NewHandler(store, nil, nil, nil)
	handler.SetRuntimeCache(cache.NewMemory(1))
	return handler
}

func newGrokMediaTestContext(t *testing.T, method, target string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	ctx.Set(contextAPIKeyID, int64(11))
	return ctx, rec
}

// 生图:请求直投上游 images 端点,响应原样透传;上游收到的 body 不含 size。

// 生视频端到端:创建返回 request_id 并绑定账号;状态查询把 video.url 重写为网关
// content 代理地址;content 下载经网关流式转发。

// API Key 账号走 xai profile:1.5 模型名须归一成 -preview。

// 测试上游是 http 且非官方资产域,签名 URL 不可信,content 会走带凭据的回退路径。

// 1. 创建

// 2. 状态查询:video.url 必须被重写为网关 content 代理地址

// 3. content 下载:经网关代理转发

// 状态/下载必须命中创建任务的 API Key;其他 Key 一律 404,不泄露任务存在性。

// 另一个 Key

// 上游报错时透传错误且只做模型级冷却,不牵连账号的文本调度。

// 上游账号级 403 按网关既有语义改写为池级 503(sendFinalUpstreamError),
// 不把账号侧的套餐/权限问题当成下游客户端错误。

// 无前缀路由下状态响应的代理地址不带 /v1 前缀,与请求形态一致。

// 生视频请求体缺 prompt 且无图片输入时 400,不消耗账号额度。

// 媒体调度优先付费凭据:free OAuth 号打媒体端点必 403,不该在首选层出现;
// API Key 账号按量计费视同付费。

// AT 无 tier、导入无 plan_type 时,控制面 display 事实给出的付费套餐同样进首选层。

// 首选层是基础媒体过滤器的子集:非 Grok 账号仍被拒

// 1x1 PNG,可解出尺寸与字节数

// pending 期上游以 202 携带状态体返回;网关须按 200 透传而不是包装成错误。
// edits/extensions 缺省模型是 grok-imagine-video(1.5 系列不支持这两个操作)。

// CLI 身份头只发 CLI 网关:xAI 公开 API 收到 x-grok-client-* 会按 CLI 的
// Zero Data Retention 政策强制 output.upload_url,媒体请求 400。

// API Key 账号 → xai profile

// OAuth 账号 → cli profile 保留 CLI 头
