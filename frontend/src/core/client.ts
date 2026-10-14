export type JsonRecord = Record<string, unknown>
export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) { super(message); this.status = status }
}
export const secretKey = 'codex-account-service:admin-key'
export function adminKey() { return sessionStorage.getItem(secretKey) ?? '' }
export async function request<T = JsonRecord>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers)
  if (adminKey()) headers.set('X-Admin-Key', adminKey())
  if (options.body && !(options.body instanceof FormData)) headers.set('Content-Type', 'application/json')
  const response = await fetch(`/api/admin${path}`, { ...options, headers, signal: options.signal ?? AbortSignal.timeout(180_000) })
  if (response.ok && response.headers.get('Content-Type')?.includes('text/event-stream')) return await readEventResult(response) as T
  const text = await response.text()
  let value: unknown
  try { value = text ? JSON.parse(text) : {} } catch { throw new ApiError(response.status, '服务返回了无法解析的响应') }
  if (!response.ok) {
    const error = (value as {error?: unknown}).error
    const message = typeof error === 'string' ? error : (error as {message?: string} | undefined)?.message
    if (response.status === 401) window.dispatchEvent(new Event('admin-auth-expired'))
    throw new ApiError(response.status, message ?? `请求失败 (${response.status})`)
  }
  return value as T
}
export const send = <T = JsonRecord>(path: string, method = 'POST', body?: unknown) =>
  request<T>(path, { method, ...(body === undefined ? {} : {body: JSON.stringify(body)}) })

export function jsonObject(text: string): JsonRecord {
  const value: unknown = JSON.parse(text)
  if (value === null || typeof value !== 'object' || Array.isArray(value)) throw new Error('必须填写 JSON 对象')
  return value as JsonRecord
}
export function formatTime(value: unknown) {
  if (!value) return '—'
  const date = new Date(String(value))
  return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString('zh-CN', {hour12: false})
}
export function writableSettings(settings: JsonRecord) {
  const result = {...settings}
  for (const key of ['response_cache_config_generation', 'codex_client_versions', 'codex_egress']) delete result[key]
  for (const key of Object.keys(result)) if (key.startsWith('codex_synced_')) delete result[key]
  return result
}

// Import and large batch endpoints finish with a complete event. Never treat a truncated stream as success.
export async function readEventResult(response: Response): Promise<JsonRecord> {
  if (!response.body) throw new Error('导入响应没有正文')
  const reader = response.body.getReader(), decoder = new TextDecoder()
  let pending = '', complete: JsonRecord | null = null
  const accept = (frame: string) => {
    const payload = frame.split('\n').filter(line => line.startsWith('data:')).map(line => line.slice(5).trimStart()).join('\n')
    if (!payload) return
    const event = jsonObject(payload)
    if (event.type === 'complete') complete = event
    if (event.type === 'error') throw new Error(String(event.message ?? event.error ?? '导入失败'))
  }
  try {
    while (true) {
      const {value, done} = await reader.read()
      pending += decoder.decode(value, {stream: !done})
      pending = pending.replace(/\r\n/g, '\n')
      let end: number
      while ((end = pending.indexOf('\n\n')) >= 0) { accept(pending.slice(0, end)); pending = pending.slice(end + 2) }
      if (pending.length > 1024 * 1024) throw new Error('导入进度事件过大')
      if (done) break
    }
    if (pending.trim()) accept(pending)
    if (!complete) throw new Error('导入连接提前结束，请刷新账号列表检查实际结果')
    return complete
  } catch (error) { await reader.cancel().catch(() => {}); throw error }
  finally { reader.releaseLock() }
}
