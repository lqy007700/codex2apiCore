package admin

import (
	"context"

	"strings"
	"testing"

	"github.com/codex2api/auth"
	"github.com/codex2api/database"
)

// 号池为空时下拉仍要有默认候选。

// 只传 claude 不得清掉 antigravity。

func TestConnectionTestUsesChannelSettingsForAntigravity(t *testing.T) {
	store := auth.NewStore(nil, nil, nil)
	account := newAntigravityConnectionTestAccount()
	account.Models = []string{"gemini-3.6-flash-low", "gemini-3.6-flash-medium", "gemini-3.6-flash-high", "claude-sonnet-4-6"}
	store.AddAccount(account)
	handler := &Handler{store: store}
	handler.channelTestCfg.Store(&database.ChannelTestConfig{
		Antigravity: database.ChannelTestSettings{TestModel: "claude-sonnet-4-6", TestContent: "渠道专用测活 {{date}}"},
	})

	model, err := handler.connectionTestModelForAccount(context.Background(), account, "")
	if err != nil || model != "claude-sonnet-4-6" {
		t.Fatalf("model=%q err=%v, want configured channel default", model, err)
	}
	payload := handler.buildAccountConnectionTestPayload(context.Background(), account, model, auth.ClaudeSecurityConfig{})
	if !strings.Contains(string(payload), "渠道专用测活") || strings.Contains(string(payload), "{{date}}") {
		t.Fatalf("payload %s must carry the rendered channel test content", payload)
	}

	// 配置的模型不在账号目录时退回自动选模，而不是报错。
	handler.channelTestCfg.Store(&database.ChannelTestConfig{Antigravity: database.ChannelTestSettings{TestModel: "gemini-9-flash-low"}})
	model, err = handler.connectionTestModelForAccount(context.Background(), account, "")
	if err != nil || model != "gemini-3.6-flash-low" {
		t.Fatalf("fallback model=%q err=%v", model, err)
	}
}

func TestBatchTestConcurrencyUsesChannelValueForHomogeneousBatches(t *testing.T) {
	store := auth.NewStore(nil, nil, nil)
	store.SetTestConcurrency(5)
	handler := &Handler{store: store}
	handler.channelTestCfg.Store(&database.ChannelTestConfig{
		Antigravity: database.ChannelTestSettings{TestConcurrency: 2},
	})
	antigravity := newAntigravityConnectionTestAccount()
	codex := &auth.Account{DBID: 9, AccessToken: "codex-token"}

	if got := handler.batchTestConcurrency(context.Background(), []*auth.Account{antigravity, antigravity}); got != 2 {
		t.Fatalf("antigravity-only batch concurrency = %d, want channel value 2", got)
	}
	if got := handler.batchTestConcurrency(context.Background(), []*auth.Account{antigravity, codex}); got != 5 {
		t.Fatalf("mixed batch concurrency = %d, want global 5", got)
	}
	if got := handler.batchTestConcurrency(context.Background(), nil); got != 5 {
		t.Fatalf("empty batch concurrency = %d, want global 5", got)
	}
	handler.channelTestCfg.Store(&database.ChannelTestConfig{})
	if got := handler.batchTestConcurrency(context.Background(), []*auth.Account{antigravity}); got != 5 {
		t.Fatalf("unconfigured channel concurrency = %d, want global 5", got)
	}
}
