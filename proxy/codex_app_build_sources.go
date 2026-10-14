package proxy

import (
	"context"

	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	codexMacAppcastURL    = "https://persistent.oaistatic.com/codex-app-prod/appcast.xml"
	codexWindowsUpdateURL = "https://persistent.oaistatic.com/codex-app-prod/windows-store-update.json"
	codexMarketplaceURL   = "https://marketplace.visualstudio.com/_apis/public/gallery/extensionquery?api-version=7.1-preview.1"
)

type codexMarketplaceProperty struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type codexBuildRequestSpec struct {
	Method string
	URL    string
	Body   io.Reader
}

func codexBuildParts(version string, count int) ([]int64, bool) {
	parts := strings.Split(strings.TrimSpace(version), ".")
	if len(parts) != count {
		return nil, false
	}
	values := make([]int64, count)
	for i, part := range parts {
		if part == "" || strings.Trim(part, "0123456789") != "" {
			return nil, false
		}
		v, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return nil, false
		}
		values[i] = v
	}
	return values, true
}

func codexBuildIsNewer(candidate, current string) bool {
	a, ok := codexBuildParts(candidate, 3)
	if !ok {
		return false
	}
	b, ok := codexBuildParts(current, 3)
	if !ok {
		return true
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

func codexBuildHTTPClient(endpoint, proxyURL string) (*http.Client, func()) {
	return newVersionSyncClient(endpoint, proxyURL, 90*time.Second)
}

func codexBuildRequest(ctx context.Context, client *http.Client, spec codexBuildRequestSpec) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, spec.Method, spec.URL, spec.Body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "codex2api")
	req.Header.Set("Accept-Encoding", "identity")
	if spec.Method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json;api-version=7.1-preview.1")
	}
	resp, err := client.Do(req)
	return resp, codexVersionSourceError(err)
}

func codexReadSmallResponse(resp *http.Response, limit int64) ([]byte, error) {
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("upstream response exceeds %d bytes", limit)
	}
	return data, nil
}

// FetchCodexDesktopMacBuild 读取官方 Sparkle appcast 中最新正式版的应用构建号。

// FetchCodexVSCodeBuild 只选择 Marketplace openai.chatgpt 的稳定版。

// 只获取最新稳定版与预览版，避免读取全部历史。

func codexMarketplacePrerelease(properties []codexMarketplaceProperty) bool {
	for _, property := range properties {
		if property.Key == "Microsoft.VisualStudio.Code.PreRelease" {
			return strings.EqualFold(property.Value, "true")
		}
	}
	return false
}
