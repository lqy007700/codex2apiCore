package admin

import (
	"context"

	"os"

	"sync"
	"testing"
)

type fakeSystemReleaseClient struct {
	release  *systemGitHubRelease
	files    map[string][]byte
	fetchErr error
	fetches  int
}

func (c *fakeSystemReleaseClient) FetchLatestRelease(context.Context) (*systemGitHubRelease, error) {
	if c.fetchErr != nil {
		return nil, c.fetchErr
	}
	c.fetches++
	return c.release, nil
}

func (c *fakeSystemReleaseClient) DownloadFile(_ context.Context, rawURL, dest string, _ int64) error {
	data, ok := c.files[rawURL]
	if !ok {
		return os.ErrNotExist
	}
	return os.WriteFile(dest, data, 0644)
}

func (c *fakeSystemReleaseClient) FetchText(_ context.Context, rawURL string, _ int64) ([]byte, error) {
	data, ok := c.files[rawURL]
	if !ok {
		return nil, os.ErrNotExist
	}
	return data, nil
}

func TestValidateSystemUpdateURL(t *testing.T) {
	allowed := []string{
		"https://github.com/james-6-23/codex2api/releases/download/v1/codex2api.tar.gz",
		"https://release-assets.githubusercontent.com/github-production-release-asset/file",
		"https://objects.githubusercontent.com/github-production-release-asset/file",
	}
	for _, rawURL := range allowed {
		if err := validateSystemUpdateURL(rawURL); err != nil {
			t.Fatalf("validateSystemUpdateURL(%q) unexpected error: %v", rawURL, err)
		}
	}

	blocked := []string{
		"http://github.com/james-6-23/codex2api/releases/download/v1/codex2api.tar.gz",
		"https://example.com/codex2api.tar.gz",
	}
	for _, rawURL := range blocked {
		if err := validateSystemUpdateURL(rawURL); err == nil {
			t.Fatalf("validateSystemUpdateURL(%q) expected error", rawURL)
		}
	}
}

func TestHandlerSystemUpdaterConcurrentSingleInstance(t *testing.T) {
	handler := &Handler{}
	const workers = 32
	results := make(chan *systemUpdater, workers)
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			results <- handler.systemUpdater()
		}()
	}

	wg.Wait()
	close(results)

	var first *systemUpdater
	for updater := range results {
		if updater == nil {
			t.Fatal("systemUpdater() returned nil")
		}
		if first == nil {
			first = updater
			continue
		}
		if updater != first {
			t.Fatal("systemUpdater() returned multiple instances")
		}
	}
}

// 生产代码在替换前会 EvalSymlinks 解析真实路径(如 macOS 下 /var → /private/var),
// 断言期望值同样解析,避免在软链接临时目录的平台上误报。

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}
