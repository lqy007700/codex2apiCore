import type {JsonRecord} from './client'
export interface Account extends JsonRecord {
 id: number; name: string; email: string; plan_type: string; status: string; enabled: boolean; locked: boolean
 active_requests?: number; dynamic_concurrency_limit?: number; health_tier?: string; scheduler_score?: number
 codex_5h_used_percent?: number | string; codex_7d_used_percent?: number | string; proxy_url?: string; error_message?: string
}
export interface AccountPage {accounts: Account[]; total?: number; total_matched?: number}
export interface ProxyRow {id: number; url: string; label: string; enabled: boolean; status: string; bound_count: number}
export interface KeyRow {id: number; name: string; masked_key?: string; key_masked?: string; key?: string; enabled: boolean}
