package proxy

import (
	"net/http"
	"testing"

	"github.com/codex2api/auth"
)

func TestGrokSupportsXHighReasoningEffort(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		{"grok-4.7", true},
		{"grok-4.7-beta", true},
		{"grok-4.6", true},
		{"GROK-4.6", true},
		{"grok-4.6-beta", true},
		{"grok-4.6-build", true},
		{"grok-4.20-multi-agent", true},
		{"grok-5", true},
		{"grok-5.0", true},
		{"grok-4.5", false},
		{"grok-4.5-build", false},
		{"grok-4", false},
		{"grok-4-fast", false},
		{"grok-3", false},
		{"gpt-5.6-sol", false},
		{"", false},
	}
	for _, c := range cases {
		if got := grokSupportsXHighReasoningEffort(c.model); got != c.want {
			t.Fatalf("%q = %v, want %v", c.model, got, c.want)
		}
	}
}

// 旧 Grok build 只有 low/medium/high：xhigh/max 降到 high、minimal 降到 low。
// grok-4.6 起放行 xhigh；Codex 的 max 落到 xhigh。无模型时按旧 build 处理。

// 无 effort 字段不崩、不误增。

// sanitize 主入口：旧模型仍降级，4.6 放行 xhigh。

// reasoning 写在 model 前面，确认先窥探 model 再钳位，而不是按键序误降级。

func TestGrokConversationGroupIDMatchesBuildDerivation(t *testing.T) {
	const root = "0f4c7a1e-6c1b-5a0e-8e6c-3b0f6a6a2a1d"
	if got := grokConversationGroupID(root); got != "d5de345a-3509-59bc-b73c-7d33029d840b" {
		t.Fatalf("conversation group id = %q", got)
	}
	req, err := http.NewRequest(http.MethodPost, "https://cli-chat-proxy.grok.com/v1/responses", nil)
	if err != nil {
		t.Fatal(err)
	}
	headers := make(http.Header)
	headers.Set("Session-Id", root)
	applyGrokRequestHeaders(req, &auth.Account{DBID: 1, UpstreamType: auth.UpstreamGrok, AccessToken: "at"}, "tok", headers, nil)
	if got := req.Header.Get("x-grok-conv-id"); got != root {
		t.Fatalf("conv-id = %q", got)
	}
	if got := req.Header.Get("x-grok-conv-group-id"); got != "d5de345a-3509-59bc-b73c-7d33029d840b" {
		t.Fatalf("conv-group-id = %q", got)
	}
}
