package proxy

import (
	"net/http"
	"net/http/httptest"
)

// GenerateImageOnceForAdmin executes the existing Images API handler in-process.
// It keeps model aliasing, account dispatch, usage logging, and image parsing in one code path.

// 每个批量输出都是一次真实上游请求，因此复用标准 Images handler 的
// API Key 限额。并发槽由调用方决定：入队时已经为整个任务占过槽的路径
// 传 true，否则这里会为同一个 Key 再占一次，把上限打对折。

// GenerateImageEditForAdmin executes the ImagesEdits handler in-process for
// image-to-image (edit) jobs from the admin image studio.

// Unlike a network ResponseWriter, httptest.ResponseRecorder commits the first
// informational response. Image keepalives send 102 before the final response;
// do not let that heartbeat turn a successful image into a failed job.
type adminImageResponseRecorder struct {
	*httptest.ResponseRecorder
}

func newAdminImageResponseRecorder() *adminImageResponseRecorder {
	return &adminImageResponseRecorder{ResponseRecorder: httptest.NewRecorder()}
}

func (w *adminImageResponseRecorder) WriteHeader(code int) {
	if code >= 100 && code < 200 && code != http.StatusSwitchingProtocols {
		return
	}
	w.ResponseRecorder.WriteHeader(code)
}

// applyAdminScopeBudget 为 in-process 的管理端生图任务计算 scope 预算闸门：
// reject 类超额直接返回 429，skip 类挂到该 gin context 上供账号过滤链剔除候选。
