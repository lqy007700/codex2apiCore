package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/codex2api/database"
	"github.com/gin-gonic/gin"
)

// Only execution, account-pool and identity settings are writable in this service.
var coreSettingKeys = map[string]bool{
	"max_concurrency":                          true,
	"global_rpm":                               true,
	"test_model":                               true,
	"test_content":                             true,
	"test_concurrency":                         true,
	"usage_probe_max_age_minutes":              true,
	"background_refresh_interval_minutes":      true,
	"usage_probe_concurrency":                  true,
	"usage_probe_responses_fallback_enabled":   true,
	"recovery_probe_interval_minutes":          true,
	"lazy_mode":                                true,
	"codex_oauth_keepalive_enabled":            true,
	"proxy_url":                                true,
	"pg_max_conns":                             true,
	"redis_pool_size":                          true,
	"auto_clean_unauthorized":                  true,
	"auto_clean_rate_limited":                  true,
	"auto_clean_full_usage":                    true,
	"auto_clean_error":                         true,
	"auto_clean_expired":                       true,
	"auto_reset_credits_enabled":               true,
	"auto_reset_credits_on_exhaustion_enabled": true,
	"auto_reset_credits_before_expiry_min":     true,
	"auto_activate_5h_window_enabled":          true,
	"proxy_pool_enabled":                       true,
	"fast_scheduler_enabled":                   true,
	"scheduler_engine":                         true,
	"codex_force_websocket":                    true,
	"codex_request_compression":                true,
	"codex_ws_weak_network_mode":               true,
	"codex_ws_keepalive_enabled":               true,
	"codex_ws_keepalive_interval_sec":          true,
	"codex_ws_hide_upstream_errors":            true,
	"codex_ws_silent_retry_enabled":            true,
	"codex_ws_silent_max_retries":              true,
	"codex_ws_size_router_enabled":             true,
	"codex_ws_busy_acquire_max_wait_sec":       true,
	"codex_ws_busy_overflow_enabled":           true,
	"codex_ws_busy_patience_sec":               true,
	"codex_ws_stateless_slots":                 true,
	"codex_ws_downstream_keepalive_slots":      true,
	"github_token":                             true,
	"github_proxy_url":                         true,
	"codex_overload_pause_enabled":             true,
	"codex_overload_threshold_percent":         true,
	"codex_overload_pause_minutes":             true,
	"codex_overload_window_minutes":            true,
	"overflow_auto_compact_enabled":            true,
	"compact_via_responses_enabled":            true,
	"codex_preflight_sse_passthrough_enabled":  true,
	"first_token_excludes_ws_acquire":          true,
	"codex_continue_thinking_enabled":          true,
	"codex_continue_max_rounds":                true,
	"utls_shutdown_timeout_minutes":            true,
	"codex_cli_version_sync_enabled":           true,
	"codex_cli_version_sync_interval_hours":    true,
	"scheduler_mode":                           true,
	"affinity_mode":                            true,
	"session_affinity_spread":                  true,
	"session_slot_buffer_enabled":              true,
	"session_slot_buffer_seconds":              true,
	"max_retries":                              true,
	"max_rate_limit_retries":                   true,
	"retry_interval_ms":                        true,
	"transport_retry_policy":                   true,
	"continuous_retry_enabled":                 true,
	"continuous_retry_catch_all":               true,
	"continuous_retry_categories":              true,
	"continuous_retry_status_codes":            true,
	"continuous_retry_error_codes":             true,
	"continuous_retry_max_duration_seconds":    true,
	"codex_fingerprint_default_mode":           true,
	"allow_remote_migration":                   true,
	"model_mapping":                            true,
	"codex_model_mapping":                      true,
	"payload_rules":                            true,
	"reasoning_effort_models":                  true,
	"resin_url":                                true,
	"resin_platform_name":                      true,
	"client_compat_mode":                       true,
	"codex_min_cli_version":                    true,
	"codex_user_agent_config":                  true,
	"codex_unified_client_identity_enabled":    true,
	"usage_log_mode":                           true,
	"usage_log_batch_size":                     true,
	"usage_log_flush_interval_seconds":         true,
	"stream_flush_policy":                      true,
	"stream_flush_interval_ms":                 true,
	"first_token_mode":                         true,
	"first_token_timeout_seconds":              true,
	"billing_tier_policy":                      true,
	"models_list_read_max_bytes":               true,
	"show_upstream_model_mismatch":             true,
	"auto_pause_5h_threshold":                  true,
	"auto_pause_7d_threshold":                  true,
	"auto_pause_5h_guard_band_percent":         true,
	"auto_pause_5h_guard_concurrency":          true,
	"smart_pacing_enabled":                     true,
	"smart_pacing_min_concurrency":             true,
	"smart_pacing_windows":                     true,
	"ignore_usage_limit_status":                true,
	"response_cache_local_max_bytes":           true,
	"response_cache_local_max_entry_bytes":     true,
	"response_cache_reconstruct_max_bytes":     true,
	"response_cache_write_policy":              true,
	"response_cache_config_generation":         true,
	"relay_model_cooldown_mode":                true,
	"relay_model_cooldown_seconds":             true,
	"relay_model_cooldown_backoff_enabled":     true,
	"oauth_model_cooldown_mode":                true,
	"oauth_model_cooldown_seconds":             true,
	"oauth_model_cooldown_backoff_enabled":     true,
	"oauth_unsupported_model_cooldown_seconds": true,
}

func (h *Handler) codexServiceMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodPost && c.FullPath() == "/api/admin/accounts/import" {
			if err := validateCoreImport(c); err != nil {
				c.AbortWithStatusJSON(400, gin.H{"error": err.Error()})
				return
			}
		}
		channel := c.Query("channel")
		if channel != "" && channel != "codex" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "仅支持 Codex 渠道"})
			return
		}
		if h.db != nil && c.Request.Method == http.MethodPost && strings.HasPrefix(c.FullPath(), "/api/admin/accounts/batch-") {
			if err := h.validateCoreBatchAccountIDs(c); err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		}
		if id, err := strconv.ParseInt(c.Param("id"), 10, 64); err == nil && h.db != nil && strings.HasPrefix(c.FullPath(), "/api/admin/accounts/:id") {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
			row, err := h.db.GetAccountByIDIncludingDeleted(ctx, id)
			cancel()
			if err == nil && !database.IsNativeCodexCredentials(row.Credentials) {
				c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "该账号不属于 Codex 账号池"})
				return
			}
		}
		if c.Request.Method == http.MethodPut && c.FullPath() == "/api/admin/settings" {
			raw, err := io.ReadAll(c.Request.Body)
			if err != nil {
				c.AbortWithStatusJSON(400, gin.H{"error": "无法读取设置"})
				return
			}
			var fields map[string]json.RawMessage
			if json.Unmarshal(raw, &fields) != nil || fields == nil {
				c.AbortWithStatusJSON(400, gin.H{"error": "设置必须是 JSON 对象"})
				return
			}
			for key := range fields {
				if !coreSettingKeys[key] {
					c.AbortWithStatusJSON(400, gin.H{"error": "不支持的设置项: " + key})
					return
				}
			}
			c.Request.Body = io.NopCloser(strings.NewReader(string(raw)))
		}
		c.Next()
	}
}

// Explicit IDs bypass the filtered account selector. Check the whole batch
// before dispatch so legacy provider rows cannot be changed through core routes.
func (h *Handler) validateCoreBatchAccountIDs(c *gin.Context) error {
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return fmt.Errorf("无法读取批量请求")
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	var request struct {
		IDs []int64 `json:"ids"`
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		return fmt.Errorf("批量请求必须包含有效的 JSON 账号 ID 列表")
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	for start := 0; start < len(request.IDs); start += 500 {
		end := min(start+500, len(request.IDs))
		rows, err := h.db.ListActiveByIDs(ctx, request.IDs[start:end])
		if err != nil {
			return fmt.Errorf("无法校验批量账号")
		}
		for _, row := range rows {
			if !database.IsNativeCodexCredentials(row.Credentials) {
				return fmt.Errorf("账号 %d 不属于 Codex 账号池", row.ID)
			}
		}
	}
	return nil
}

func (h *Handler) GetCoreStatus(c *gin.Context) {
	active, totalRequests := h.store.RuntimeRequestCounts()
	available, totalAccounts, complete := h.store.HealthCountsNonBlocking()
	c.JSON(http.StatusOK, gin.H{
		"available": available, "total": totalAccounts, "counts_complete": complete,
		"active_requests": active, "total_requests": totalRequests,
		"scheduler": h.store.GetSchedulerMetrics(),
	})
}

func (h *Handler) writeCoreSettingsResponse(c *gin.Context, status int, response settingsResponse) {
	raw, err := json.Marshal(response)
	if err != nil {
		writeInternalError(c, err)
		return
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		writeInternalError(c, err)
		return
	}
	for key := range fields {
		if !coreSettingKeys[key] && key != "response_cache_config_generation" && key != "codex_client_versions" && key != "codex_egress" && !strings.HasPrefix(key, "codex_synced_") {
			delete(fields, key)
		}
	}
	c.JSON(status, fields)
}

// Reject explicit foreign-provider credentials before a JSON import strips their metadata.
func validateCoreImport(c *gin.Context) error {
	if c.PostForm("format") != "json" && c.PostForm("format") != "json_at" {
		return nil
	}
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		return err
	}
	for _, file := range c.Request.MultipartForm.File["file"] {
		if err := validateImportFileSize(file); err != nil {
			return err
		}
		stream, err := file.Open()
		if err != nil {
			return err
		}
		err = validateNativeCodexJSON(stream)
		stream.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func validateNativeCodexJSON(stream io.Reader) error {
	decoder := json.NewDecoder(stream)
	var scan func(string) error
	scan = func(key string) error {
		token, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("凭据文件不是有效 JSON")
		}
		if value, ok := token.(string); ok {
			value = strings.ToLower(strings.TrimSpace(value))
			foreign := key == "upstream_type" && value != "" && value != "codex"
			foreign = foreign || key == "api_key" && value != ""
			if key == "upstream_type" || key == "type" || key == "platform" {
				for _, provider := range []string{"grok", "claude", "antigravity", "openai_responses", "responses_api", "anthropic", "xai", "google", "gemini"} {
					if value == provider {
						foreign = true
					}
				}
			}
			if foreign {
				return fmt.Errorf("仅支持原生 Codex 账号凭据")
			}
		}
		if delimiter, ok := token.(json.Delim); ok {
			switch delimiter {
			case '{':
				for decoder.More() {
					token, err := decoder.Token()
					if err != nil {
						return err
					}
					key, ok := token.(string)
					if !ok {
						return fmt.Errorf("凭据文件不是有效 JSON")
					}
					if err := scan(key); err != nil {
						return err
					}
				}
				if _, err := decoder.Token(); err != nil {
					return err
				}
			case '[':
				for decoder.More() {
					if err := scan(""); err != nil {
						return err
					}
				}
				if _, err := decoder.Token(); err != nil {
					return err
				}
			default:
				return fmt.Errorf("凭据文件不是有效 JSON")
			}
		}
		return nil
	}
	for decoder.More() {
		if err := scan(""); err != nil {
			return err
		}
	}
	return nil
}
