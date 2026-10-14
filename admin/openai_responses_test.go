package admin

import (
	"context"

	"testing"

	"github.com/codex2api/auth"
	"github.com/codex2api/database"
)

func TestConnectionTestModelForOpenAIResponsesAccountUsesFirstSupportedFallback(t *testing.T) {
	store := auth.NewStore(nil, nil, &database.SystemSettings{TestModel: "gpt-5.4"})
	handler := &Handler{store: store}
	account := &auth.Account{
		UpstreamType: auth.UpstreamOpenAIResponses,
		BaseURL:      "https://api.openai.com",
		APIKey:       "sk-test",
		Models:       []string{"gpt-4.1-mini", "gpt-4.1"},
	}

	model, err := handler.connectionTestModelForAccount(context.Background(), account, "")
	if err != nil {
		t.Fatalf("connectionTestModelForAccount returned error: %v", err)
	}
	if model != "gpt-4.1-mini" {
		t.Fatalf("model = %q, want first account model", model)
	}

	model, err = handler.connectionTestModelForAccount(context.Background(), account, "gpt-4.1")
	if err != nil {
		t.Fatalf("requested model returned error: %v", err)
	}
	if model != "gpt-4.1" {
		t.Fatalf("requested model = %q, want gpt-4.1", model)
	}
}

func TestConnectionTestModelForOpenAIResponsesAccountAcceptsMappingAlias(t *testing.T) {
	store := auth.NewStore(nil, nil, &database.SystemSettings{TestModel: "gpt-5.4"})
	handler := &Handler{store: store}
	account := &auth.Account{
		UpstreamType: auth.UpstreamOpenAIResponses,
		BaseURL:      "https://api.openai.com",
		APIKey:       "sk-test",
		Models:       []string{"gpt-4.1"},
		ModelMapping: `{"client-alias":"gpt-4.1"}`,
	}

	model, err := handler.connectionTestModelForAccount(context.Background(), account, "client-alias")
	if err != nil {
		t.Fatalf("requested alias returned error: %v", err)
	}
	if model != "gpt-4.1" {
		t.Fatalf("mapped model = %q, want gpt-4.1", model)
	}
}

// TestNormalizeAccountModelMappingValidatesModelNames 验证映射键值必须通过
// security.ValidateModelName——源别名会进入 /v1/models 响应和使用日志,
// 不能放任意字符串注入(PR #325 评审遗留)。
