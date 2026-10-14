package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"

	"testing"
)

func TestQueryPersonalAccessTokenMetadata_RejectsRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://malicious.external.com/leak", http.StatusFound)
	}))
	defer server.Close()

	oldURL := patWhoAmIURLForTest
	patWhoAmIURLForTest = server.URL
	defer func() { patWhoAmIURLForTest = oldURL }()

	_, err := QueryPersonalAccessTokenMetadata(context.Background(), "at-test-token", "")
	if err == nil {
		t.Fatal("expected error on redirect, got nil")
	}
	if !strings.Contains(err.Error(), "302") && !strings.Contains(err.Error(), "status") {
		t.Logf("got error: %v", err)
	}
}

// 批量导入内：网络层失败熔断本批剩余 token，端点 4xx 只影响那一枚。

// 把端点指向一个已关闭的服务器模拟网络不通。
