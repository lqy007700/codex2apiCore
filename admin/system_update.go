package admin

import (
	"context"

	"encoding/json"
	"errors"
	"fmt"
	"io"

	"net/http"
	"net/url"
	"os"

	"runtime"

	"strings"
	"sync"
	"time"

	"github.com/codex2api/internal/version"
	"github.com/codex2api/proxy"
)

const (
	systemUpdateRepo             = "james-6-23/codex2api"
	systemUpdateUserAgent        = "Codex2API-Updater"
	systemUpdateMaxDownloadBytes = 200 * 1024 * 1024
	systemUpdateRestartDelay     = 900 * time.Millisecond
	systemUpdateReleaseCacheTTL  = 2 * time.Minute
)

var (
	errSystemUpdateBusy        = errors.New("已有更新任务正在执行")
	errSystemUpdateLatest      = errors.New("当前已是最新版本")
	errSystemUpdateUnsupported = errors.New("当前运行环境不支持在线更新")
)

type systemUpdater struct {
	currentVersion     string
	client             systemReleaseClient
	goos               string
	goarch             string
	executablePath     func() (string, error)
	restartProcess     func(string) error
	restartDelay       time.Duration
	runningInContainer func() bool

	mu                    sync.Mutex
	releaseCacheMu        sync.Mutex
	releaseCache          *systemGitHubRelease
	releaseCacheExpiresAt time.Time
}

type systemReleaseClient interface {
	FetchLatestRelease(ctx context.Context) (*systemGitHubRelease, error)
	DownloadFile(ctx context.Context, rawURL, dest string, maxSize int64) error
	FetchText(ctx context.Context, rawURL string, maxSize int64) ([]byte, error)
}

type systemUpdateInfo struct {
	CurrentVersion    string `json:"current_version"`
	LatestVersion     string `json:"latest_version"`
	HasUpdate         bool   `json:"has_update"`
	Supported         bool   `json:"supported"`
	UnsupportedReason string `json:"unsupported_reason,omitempty"`
	RuntimeOS         string `json:"runtime_os"`
	RuntimeArch       string `json:"runtime_arch"`
	Mode              string `json:"mode"`
	ReleaseURL        string `json:"release_url,omitempty"`
	AssetName         string `json:"asset_name,omitempty"`
	PublishedAt       string `json:"published_at,omitempty"`
	Warning           string `json:"warning,omitempty"`
}

type systemUpdateResult struct {
	Message        string `json:"message"`
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	NeedRestart    bool   `json:"need_restart"`
	Restarting     bool   `json:"restarting"`
	Mode           string `json:"mode"`
	BackupPath     string `json:"backup_path,omitempty"`
}

type systemUpdateInspection struct {
	info          *systemUpdateInfo
	asset         *systemGitHubAsset
	checksumAsset *systemGitHubAsset
}

type systemGitHubRelease struct {
	TagName     string              `json:"tag_name"`
	Name        string              `json:"name"`
	Body        string              `json:"body"`
	PublishedAt string              `json:"published_at"`
	HTMLURL     string              `json:"html_url"`
	Assets      []systemGitHubAsset `json:"assets"`
}

type systemGitHubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
}

type defaultSystemReleaseClient struct {
	apiClient      *http.Client
	downloadClient *http.Client
}

func newSystemUpdater() *systemUpdater {
	client := newDefaultSystemReleaseClient()
	return &systemUpdater{
		currentVersion:     version.Current(),
		client:             client,
		goos:               runtime.GOOS,
		goarch:             runtime.GOARCH,
		executablePath:     os.Executable,
		restartProcess:     defaultRestartProcess,
		restartDelay:       systemUpdateRestartDelay,
		runningInContainer: detectRunningInContainer,
	}
}

// detectRunningInContainer 尽力判断当前进程是否运行在容器内:
// 更新容器内的二进制在容器重建后会被镜像版本覆盖,需要提示用户改用镜像升级。
func detectRunningInContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	if _, err := os.Stat("/run/.containerenv"); err == nil { // podman
		return true
	}
	data, err := os.ReadFile("/proc/1/cgroup")
	if err != nil {
		return false
	}
	content := string(data)
	return strings.Contains(content, "docker") || strings.Contains(content, "kubepods") || strings.Contains(content, "containerd")
}

func newDefaultSystemReleaseClient() *defaultSystemReleaseClient {
	redirectPolicy := func(req *http.Request, _ []*http.Request) error {
		return validateSystemUpdateURL(req.URL.String())
	}
	// GitHub 专用代理（issue #522）：配置了则优先，否则维持原行为（环境变量代理）。
	// 每请求动态解析，设置热更新即时生效。
	proxyFunc := func(req *http.Request) (*url.URL, error) {
		if req != nil && req.URL != nil {
			if dedicated := proxy.GithubProxyOrDefault(req.URL.String(), ""); dedicated != "" {
				return url.Parse(dedicated)
			}
		}
		return http.ProxyFromEnvironment(req)
	}
	return &defaultSystemReleaseClient{
		apiClient: &http.Client{
			Timeout:       30 * time.Second,
			CheckRedirect: redirectPolicy,
			Transport:     &http.Transport{Proxy: proxyFunc},
		},
		downloadClient: &http.Client{
			Timeout:       10 * time.Minute,
			CheckRedirect: redirectPolicy,
			Transport:     &http.Transport{Proxy: proxyFunc},
		},
	}
}

func (h *Handler) systemUpdater() *systemUpdater {
	h.systemUpdateOnce.Do(func() {
		if h.systemUpdate == nil {
			h.systemUpdate = newSystemUpdater()
		}
	})
	return h.systemUpdate
}

func (c *defaultSystemReleaseClient) FetchLatestRelease(ctx context.Context) (*systemGitHubRelease, error) {
	apiURL := "https://api.github.com/repos/" + systemUpdateRepo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", systemUpdateUserAgent)
	proxy.ApplyGithubAuth(req)

	resp, err := c.apiClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API 返回 HTTP %d", resp.StatusCode)
	}

	var release systemGitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
}

func (c *defaultSystemReleaseClient) DownloadFile(ctx context.Context, rawURL, dest string, maxSize int64) error {
	if err := validateSystemUpdateURL(rawURL); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", systemUpdateUserAgent)

	resp, err := c.downloadClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载返回 HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxSize {
		return fmt.Errorf("下载文件过大: %d bytes", resp.ContentLength)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	limited := io.LimitReader(resp.Body, maxSize+1)
	written, copyErr := io.Copy(out, limited)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(dest)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(dest)
		return closeErr
	}
	if written > maxSize {
		_ = os.Remove(dest)
		return fmt.Errorf("下载超过大小上限: %d bytes", maxSize)
	}
	return nil
}

func (c *defaultSystemReleaseClient) FetchText(ctx context.Context, rawURL string, maxSize int64) ([]byte, error) {
	if err := validateSystemUpdateURL(rawURL); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", systemUpdateUserAgent)

	resp, err := c.apiClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载校验和返回 HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxSize {
		return nil, fmt.Errorf("校验和文件超过大小上限: %d bytes", maxSize)
	}
	return data, nil
}

func validateSystemUpdateURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("仅允许 HTTPS")
	}
	host := strings.ToLower(parsed.Hostname())
	switch host {
	case "github.com", "api.github.com", "objects.githubusercontent.com", "release-assets.githubusercontent.com":
		return nil
	default:
		if strings.HasSuffix(host, ".githubusercontent.com") {
			return nil
		}
		return fmt.Errorf("不允许的下载域名: %s", host)
	}
}
