import type {JsonRecord} from './client'
export interface Account extends JsonRecord {
 model_cooldowns?:{model:string;reason:string;reset_at:string;remaining_seconds?:number}[]
 id: number; name: string; email: string; plan_type: string; status: string; enabled: boolean; locked: boolean
 active_requests?: number; dynamic_concurrency_limit?: number; health_tier?: string; scheduler_score?: number
 codex_5h_used_percent?: number | string; codex_7d_used_percent?: number | string; proxy_url?: string; error_message?: string
 usage_percent_5h?: number | null; usage_percent_7d?: number | null; usage_percent_spark?: number | null
 reset_5h_at?: string; reset_7d_at?: string; reset_spark_at?: string; usage_window_7d_kind?: string; usage_window_7d_seconds?: number
 rate_limit_reset_credits?: number | null; applicable_reset_credits?: number | null; using_credits?: boolean; credits_balance?: string | null; credits_valid?: boolean
 access_token_expires_at?: string; subscription_expires_at?: string; codex_last_refresh_at?: string; codex_refresh_error?: string
 codex_usage_updated_at?: string; codex_5h_usage_updated_at?: string; cooldown_until?: string; cooldown_reason?: string; last_used_at?: string; created_at?: string; updated_at?: string; at_only?: boolean
 subscription?: {business_status: string; sync_state: string; last_checked_at?: string; error?: string; expires_at?: string; auto_renew?: string} | null
}
export interface ResetCards {available_count: number; credits: {id: string; granted_at?: string; expires_at?: string}[]}
export interface AccountPage {accounts: Account[]; total?: number; total_matched?: number}
export interface ProxyRow {id: number; url: string; label: string; enabled: boolean; test_status?: string; status?: string; bound_count: number}
export interface KeyRow {id: number; name: string; masked_key?: string; key_masked?: string; key?: string; enabled: boolean}
