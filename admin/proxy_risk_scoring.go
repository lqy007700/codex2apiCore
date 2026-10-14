package admin

import (
	"context"

	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/codex2api/database"

	"github.com/tidwall/gjson"
)

const proxyRiskScoringMaxResponseBytes = 2 << 20

type proxyRiskScoringProfileRequest struct {
	Name               *string `json:"name"`
	Provider           *string `json:"provider"`
	Enabled            *bool   `json:"enabled"`
	Priority           *int    `json:"priority"`
	ScamalyticsHost    *string `json:"scamalytics_host"`
	ScamalyticsUser    *string `json:"scamalytics_user"`
	ScamalyticsKey     *string `json:"scamalytics_key"`
	TimeoutSeconds     *int    `json:"timeout_seconds"`
	Concurrency        *int    `json:"concurrency"`
	RequestDelayMS     *int    `json:"request_delay_ms"`
	CacheTTLSeconds    *int    `json:"cache_ttl_seconds"`
	MaxChecksPerJob    *int    `json:"max_checks_per_job"`
	DailyCheckLimit    *int    `json:"daily_check_limit"`
	CreditReserve      *int64  `json:"credit_reserve"`
	AllowForceRefresh  *bool   `json:"allow_force_refresh"`
	ResolveHostnames   *bool   `json:"resolve_hostnames"`
	AllowPrivateTarget *bool   `json:"allow_private_targets"`
	DocsURL            *string `json:"docs_url"`
	TutorialURL        *string `json:"tutorial_url"`
}

func validateScamalyticsHost(raw string) error {
	host := strings.ToLower(strings.TrimSpace(raw))
	if host == "" {
		return errors.New("Scamalytics Host 不能为空")
	}
	if strings.ContainsAny(host, "/?#@:") || (host != "scamalytics.com" && !strings.HasSuffix(host, ".scamalytics.com")) {
		return errors.New("Scamalytics Host 必须是 scamalytics.com 的域名")
	}
	return nil
}

type proxyRiskScoringClient struct {
	profile database.ProxyRiskScoringProfile
	http    *http.Client
}

func (client *proxyRiskScoringClient) requestIP(ctx context.Context, ip string) ([]byte, int, error) {
	if client == nil {
		return nil, 0, errors.New("评分客户端未初始化")
	}
	if err := validateScamalyticsHost(client.profile.ScamalyticsHost); err != nil {
		return nil, 0, err
	}
	if strings.TrimSpace(client.profile.ScamalyticsUser) == "" || strings.TrimSpace(client.profile.ScamalyticsKey) == "" {
		return nil, 0, errors.New("Scamalytics User 和 API Key 必须配置")
	}
	if parsed := net.ParseIP(strings.TrimSpace(ip)); parsed == nil || parsed.To4() == nil {
		return nil, 0, errors.New("评分请求必须使用 IPv4 地址")
	}
	requestURL := "https://" + strings.TrimSpace(client.profile.ScamalyticsHost) + "/v3/" + url.PathEscape(strings.TrimSpace(client.profile.ScamalyticsUser)) + "/?key=" + url.QueryEscape(strings.TrimSpace(client.profile.ScamalyticsKey)) + "&ip=" + url.QueryEscape(strings.TrimSpace(ip))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Codex2API-Scamalytics/1.0")
	started := time.Now()
	resp, err := client.http.Do(req)
	latency := int(time.Since(started).Milliseconds())
	if err != nil {
		return nil, latency, err
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, proxyRiskScoringMaxResponseBytes+1))
	if readErr != nil {
		return nil, latency, readErr
	}
	if len(body) > proxyRiskScoringMaxResponseBytes {
		return nil, latency, errors.New("Scamalytics 响应过大")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(body))
		if len(message) > 256 {
			message = message[:256]
		}
		if message == "" {
			message = fmt.Sprintf("Scamalytics 返回 HTTP %d", resp.StatusCode)
		}
		return nil, latency, errors.New(message)
	}
	return body, latency, nil
}

func (client *proxyRiskScoringClient) testConnection(ctx context.Context) (database.ProxyRiskScoreSnapshot, *proxyRiskCredits, error) {
	result, credits, err := client.checkIP(ctx, "8.8.8.8")
	if err != nil {
		return result, credits, err
	}
	return result, credits, nil
}

type proxyRiskCredits struct {
	Remaining *int64
	Used      *int64
	ResetAt   *time.Time
}

func (client *proxyRiskScoringClient) checkIP(ctx context.Context, ip string) (database.ProxyRiskScoreSnapshot, *proxyRiskCredits, error) {
	started := time.Now()
	body, latency, err := client.requestIP(ctx, ip)
	if latency == 0 {
		latency = int(time.Since(started).Milliseconds())
	}
	if err != nil {
		return database.ProxyRiskScoreSnapshot{Provider: client.profile.Provider, ResolvedIP: ip, Status: database.ProxyRiskScoringStatusError(), LatencyMS: latency, Error: err.Error()}, nil, err
	}
	result, credits, err := parseProxyRiskScoringResponse(body, latency)
	result.Provider = client.profile.Provider
	result.ResolvedIP = ip
	result.RawResponseJSON = redactProxyRiskJSON(body)
	return result, credits, err
}

func parseProxyRiskScoringResponse(body []byte, latencyMS int) (database.ProxyRiskScoreSnapshot, *proxyRiskCredits, error) {
	if !gjson.ValidBytes(body) {
		return database.ProxyRiskScoreSnapshot{Status: database.ProxyRiskScoringStatusError(), LatencyMS: latencyMS, Error: "评分服务返回了无效 JSON"}, nil, errors.New("评分服务返回了无效 JSON")
	}
	root := gjson.ParseBytes(body)
	if message := strings.TrimSpace(root.Get("error").String()); message != "" {
		return database.ProxyRiskScoreSnapshot{Status: database.ProxyRiskScoringStatusError(), LatencyMS: latencyMS, Error: message}, nil, errors.New(message)
	}
	if message := strings.TrimSpace(root.Get("scamalytics.error").String()); message != "" {
		return database.ProxyRiskScoreSnapshot{Status: database.ProxyRiskScoringStatusError(), LatencyMS: latencyMS, Error: message}, nil, errors.New(message)
	}
	result := database.ProxyRiskScoreSnapshot{Status: database.ProxyRiskScoringStatusSuccess(), LatencyMS: latencyMS, BlacklistSource: []string{}}
	scoreResult := root.Get("scamalytics.scamalytics_score")
	if !scoreResult.Exists() {
		scoreResult = root.Get("scamalytics.score")
	}
	if !scoreResult.Exists() {
		scoreResult = root.Get("score")
	}
	if scoreResult.Exists() {
		if value, err := parseProxyRiskScoreValue(scoreResult); err == nil {
			result.Score = &value
		}
	}
	result.RiskLevel = strings.ToLower(strings.TrimSpace(firstJSONString(root, "scamalytics.scamalytics_risk", "scamalytics.risk", "risk")))
	proxyData := root.Get("scamalytics.scamalytics_proxy")
	external := root.Get("external_datasources")
	result.IsVPN = jsonBool(proxyData.Get("is_vpn")) || jsonBool(external.Get("x4bnet.is_vpn"))
	result.IsTOR = jsonBool(proxyData.Get("is_tor")) || strings.EqualFold(firstJSONString(external, "ip2proxy_lite.proxy_type", "ip2proxy.proxy_type"), "TOR")
	result.IsDatacenter = jsonBool(proxyData.Get("is_datacenter")) || jsonBool(external.Get("x4bnet.is_datacenter"))
	result.IsBlacklisted = jsonBool(root.Get("scamalytics.is_blacklisted_external"))
	proxyType := strings.ToUpper(strings.TrimSpace(firstJSONString(external, "ip2proxy_lite.proxy_type", "ip2proxy.proxy_type")))
	result.ProxyType = proxyType
	if jsonBool(external.Get("firehol.is_proxy")) || proxyType == "VPN" || proxyType == "TOR" || proxyType == "PUB" || proxyType == "WEB" {
		result.IsVPN = result.IsVPN || proxyType == "VPN"
	}
	blacklist := []struct {
		path string
		name string
	}{
		{"ip2proxy_lite.ip_blacklisted", "ip2proxy"}, {"ip2proxy.ip_blacklisted", "ip2proxy"}, {"ipsum.ip_blacklisted", "ipsum"},
		{"spamhaus_drop.ip_blacklisted", "spamhaus"}, {"firehol.is_blacklisted_1day", "firehol"}, {"firehol.is_blacklisted_30", "firehol"},
		{"x4bnet.is_blacklisted_spambot", "x4bnet-spambot"},
	}
	seenBlacklist := map[string]struct{}{}
	for _, item := range blacklist {
		if jsonBool(external.Get(item.path)) {
			result.IsBlacklisted = true
			if _, exists := seenBlacklist[item.name]; !exists {
				result.BlacklistSource = append(result.BlacklistSource, item.name)
				seenBlacklist[item.name] = struct{}{}
			}
		}
	}
	result.ISP = boundedProxyScoringText(firstJSONString(root, "scamalytics.scamalytics_isp", "scamalytics.scamalytics_org", "external_datasources.dbip.isp_name", "external_datasources.ip2proxy_lite.isp_name"), 512)
	result.Country = boundedProxyScoringText(firstJSONString(root, "external_datasources.ip2proxy_lite.ip_country_code", "external_datasources.dbip.ip_country_code", "scamalytics.ip_country_code"), 128)
	result.Recommendation = proxyRiskRecommendation(result)
	features := map[string]any{"scamalytics_proxy": proxyData.Value(), "external_datasources": external.Value()}
	if encoded, err := json.Marshal(features); err == nil {
		result.FeaturesJSON = redactProxyRiskJSON(encoded)
	}
	credits := parseProxyRiskCredits(root)
	return result, credits, nil
}

func parseProxyRiskScoreValue(value gjson.Result) (int, error) {
	raw := strings.Trim(strings.TrimSpace(value.Raw), `"`)
	parsed, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || math.Trunc(parsed) != parsed || parsed < 0 || parsed > 100 {
		return 0, errors.New("评分服务返回了无效分数")
	}
	return int(parsed), nil
}

func firstJSONString(root gjson.Result, paths ...string) string {
	for _, path := range paths {
		value := root.Get(path)
		if value.Exists() && value.Type == gjson.String {
			if text := strings.TrimSpace(value.String()); text != "" {
				return text
			}
		}
	}
	return ""
}

func jsonBool(value gjson.Result) bool {
	if !value.Exists() {
		return false
	}
	return value.Bool() || value.String() == "1" || strings.EqualFold(value.String(), "true")
}

func boundedProxyScoringText(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) > max {
		return value[:max]
	}
	return value
}

func proxyRiskRecommendation(result database.ProxyRiskScoreSnapshot) string {
	if result.RiskLevel == "high" || result.RiskLevel == "very high" || result.IsBlacklisted || result.IsTOR {
		return "replace"
	}
	if result.RiskLevel == "medium" || result.IsVPN || result.IsDatacenter || (result.Score != nil && *result.Score >= 50) {
		return "watch"
	}
	return "keep"
}

func parseProxyRiskCredits(root gjson.Result) *proxyRiskCredits {
	credits := root.Get("scamalytics.credits")
	if !credits.Exists() {
		credits = root.Get("credits")
	}
	if !credits.Exists() || !credits.IsObject() {
		return nil
	}
	result := &proxyRiskCredits{}
	for key, target := range map[string]**int64{"remaining": &result.Remaining, "used": &result.Used} {
		value := credits.Get(key)
		if value.Exists() {
			if parsed, err := strconv.ParseInt(strings.TrimSpace(value.Raw), 10, 64); err == nil && parsed >= 0 {
				copyValue := parsed
				*target = &copyValue
			}
		}
	}
	for _, key := range []string{"reset_at", "resetAt", "reset_time"} {
		value := strings.TrimSpace(credits.Get(key).String())
		if value == "" {
			continue
		}
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			result.ResetAt = &parsed
			break
		}
	}
	if result.Remaining == nil && result.Used == nil && result.ResetAt == nil {
		return nil
	}
	return result
}

func redactProxyRiskJSON(body []byte) string {
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return ""
	}
	var redact func(any) any
	redact = func(input any) any {
		switch typed := input.(type) {
		case map[string]any:
			out := make(map[string]any, len(typed))
			for key, value := range typed {
				lower := strings.ToLower(key)
				if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") || lower == "key" || strings.HasSuffix(lower, "_key") {
					out[key] = "[redacted]"
					continue
				}
				out[key] = redact(value)
			}
			return out
		case []any:
			out := make([]any, len(typed))
			for index, value := range typed {
				out[index] = redact(value)
			}
			return out
		default:
			return input
		}
	}
	encoded, err := json.Marshal(redact(value))
	if err != nil {
		return ""
	}
	return boundedProxyScoringText(string(encoded), proxyRiskScoringMaxResponseBytes)
}

type proxyRiskScoringJob struct {
	mu        sync.RWMutex
	ID        string    `json:"job_id"`
	ProfileID int64     `json:"profile_id"`
	Status    string    `json:"status"`
	Total     int       `json:"total"`
	Done      int       `json:"done"`
	Success   int       `json:"success"`
	Failed    int       `json:"failed"`
	Skipped   int       `json:"skipped"`
	CacheHits int       `json:"cache_hits"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// Current 是正在检测的代理标签（host:port），Items 是逐条结果（按 Seq 递增），
	// 供前端轮询增量渲染：检测完一条就能在表格里看到一条。
	Current string                    `json:"current,omitempty"`
	Items   []proxyRiskScoringJobItem `json:"-"`
	cancel  context.CancelFunc
}

// proxyRiskScoringJobItem 是任务里一条代理的检测结果。
type proxyRiskScoringJobItem struct {
	Seq       int                              `json:"seq"`
	ProxyID   int64                            `json:"proxy_id"`
	Label     string                           `json:"label"`
	Status    string                           `json:"status"` // success | error | skipped | cached
	Error     string                           `json:"error,omitempty"`
	Snapshot  *database.ProxyRiskScoreSnapshot `json:"snapshot,omitempty"`
	CheckedAt time.Time                        `json:"checked_at"`
}

type proxyRiskScoringJobSnapshot struct {
	ID        string    `json:"job_id"`
	ProfileID int64     `json:"profile_id"`
	Status    string    `json:"status"`
	Total     int       `json:"total"`
	Done      int       `json:"done"`
	Success   int       `json:"success"`
	Failed    int       `json:"failed"`
	Skipped   int       `json:"skipped"`
	CacheHits int       `json:"cache_hits"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Current   string    `json:"current,omitempty"`
	// Items 只包含 Seq > after 的增量；LastSeq 供下次轮询作为 after。
	Items   []proxyRiskScoringJobItem `json:"items"`
	LastSeq int                       `json:"last_seq"`
}

func (job *proxyRiskScoringJob) snapshot() proxyRiskScoringJobSnapshot {
	return job.snapshotAfter(0)
}

func (job *proxyRiskScoringJob) snapshotAfter(after int) proxyRiskScoringJobSnapshot {
	job.mu.RLock()
	defer job.mu.RUnlock()
	items := make([]proxyRiskScoringJobItem, 0)
	for _, item := range job.Items {
		if item.Seq > after {
			items = append(items, item)
		}
	}
	return proxyRiskScoringJobSnapshot{
		ID: job.ID, ProfileID: job.ProfileID, Status: job.Status, Total: job.Total,
		Done: job.Done, Success: job.Success, Failed: job.Failed, Skipped: job.Skipped,
		CacheHits: job.CacheHits, Error: job.Error, CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt,
		Current: job.Current, Items: items, LastSeq: len(job.Items),
	}
}

// appendItem 追加一条逐条结果（调用方须持有 job.mu）。

// proxyRiskScoringLabel 是进度里展示的代理标签：优先备注，否则 host:port（不带凭据）。

type proxyRiskScoringJobRequest struct {
	ProfileID int64   `json:"profile_id"`
	ProxyIDs  []int64 `json:"proxy_ids"`
	Force     bool    `json:"force"`
}

// Stop the parent watcher goroutine once the job has finished. The
// cancellation function is intentionally cleared so a completed job
// cannot be cancelled again from the admin API.
