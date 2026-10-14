import type {Account} from './types'
export type StateInfo = {label: string; help: string; tone: 'good' | 'bad' | 'neutral'}
const states: Record<string, StateInfo> = {
 active:{label:'可用',help:'当前凭据可用；实际选号还受并发、模型支持、启停和锁定限制。',tone:'good'},
 normal:{label:'正常',help:'账号状态正常；实际调度仍受并发、额度与锁定限制。',tone:'good'},
 ok:{label:'正常',help:'检查成功。',tone:'good'},
 available:{label:'可用',help:'当前资源可用。',tone:'good'},
 healthy:{label:'健康',help:'健康度良好，按配置并发参与调度。',tone:'good'},
 warm:{label:'预热中',help:'新加入或恢复中的账号，逐步恢复调度能力。',tone:'neutral'},
 risky:{label:'健康度下降',help:'近期错误或限流增加，可能降低并发与调度权重。',tone:'neutral'},
 banned:{label:'鉴权不可用',help:'鉴权相关错误使账号不可调度，不一定代表账号被永久封禁。',tone:'bad'},
 unauthorized:{label:'鉴权失败',help:'上游凭据无效或拒绝访问；查看具体错误，尝试刷新凭据。',tone:'bad'},
 error:{label:'异常',help:'账号发生错误，查看错误原因；修复凭据或网络后再检查。',tone:'bad'},
 expired:{label:'已过期',help:'对应凭据或有效期已结束；Token 过期与订阅到期需分别查看。',tone:'bad'},
 refreshing:{label:'等待刷新',help:'没有可用 Token，但仍有刷新凭据，可等待后台刷新或手动刷新。',tone:'neutral'},
 rate_limited:{label:'上游限流',help:'上游暂时限制请求，通常需等待冷却或额度重置。',tone:'neutral'},
 responses_rate_limited:{label:'Responses 限流',help:'Responses 上游返回限流，等待冷却恢复。',tone:'neutral'},
 rate_limited_5h:{label:'5h 额度限流',help:'5 小时窗口受限，查看重置时间或可用重置卡。',tone:'neutral'},
 rate_limited_7d:{label:'长周期额度限流',help:'长周期窗口受限，通常为 7 天，部分套餐为月周期。',tone:'neutral'},
 usage_exhausted:{label:'额度耗尽',help:'上游用量窗口已达到限制，等待自然重置；使用积分时另行标示。',tone:'neutral'},
 usage_limit:{label:'用量受限',help:'上游用量限制阻止新请求，查看额度快照与重置时间。',tone:'neutral'},
 usage_limited:{label:'用量受限',help:'上游用量限制阻止新请求。',tone:'neutral'},
 quota_paused:{label:'阈值暂停',help:'已用比例达到配置的暂停阈值，不等于上游额度完全耗尽。',tone:'neutral'},
 cooldown:{label:'冷却中',help:'暂时停止选号，冷却结束后再检查可用性。',tone:'neutral'},
 disabled:{label:'已禁用',help:'管理员关闭了此资源，不参与正常分配。',tone:'neutral'},
 locked:{label:'已锁定',help:'管理员锁定了账号，需要解锁才能恢复正常调度。',tone:'neutral'},
 unknown:{label:'未获取',help:'尚未获取有效状态，请刷新检查。',tone:'neutral'},
 untested:{label:'未测试',help:'尚未执行代理连接测试；不代表连接失败。',tone:'neutral'},
}
export function stateInfo(value: unknown): StateInfo {
 const text=String(value??'unknown')
 if(states[text])return states[text]
 if(/^\d{3}$/.test(text)){const code=Number(text);return {label:`${code<400?'成功':'失败'} (${text})`,help:'请求记录中的 HTTP 状态码。',tone:code<400?'good':'bad'}}
 return {label:`未识别状态 (${text})`,help:'服务返回了未识别的状态；保留原值便于排查。',tone:'neutral'}
}
export function accountState(account: Account) {return stateInfo(!account.enabled?'disabled':account.locked?'locked':account.status)}
export function planLabel(plan: unknown) {
 const raw=String(plan??'').trim()
 return ({free:'免费',plus:'Plus',pro:'Pro',team:'Team 团队',business:'Business 企业',enterprise:'Enterprise 企业',edu:'教育',k12:'K12 教育'} as Record<string,string>)[raw]??(raw||'未获取套餐')
}
export function finiteNumber(value: unknown): number | null {
 if(value===null||value===undefined||typeof value==='boolean'||String(value).trim()==='')return null
 const number=Number(value);return Number.isFinite(number)?number:null
}
export function usagePercent(account: Account, window: '5h' | '7d') {
 const value=finiteNumber(account[`usage_percent_${window}`]??account[`codex_${window}_used_percent`])
 return value===null||value<0?null:value
}
export function longWindowLabel(account: Account) {
 if(account.usage_window_7d_kind==='monthly')return '月周期'
 const seconds=finiteNumber(account.usage_window_7d_seconds)
 if(seconds!==null&&seconds>0){const days=seconds/86400;if(days!==7)return `${Number(days.toFixed(1))} 天周期`}
 return '7 天周期'
}
export function subscriptionLabel(value: unknown) {
 return ({active:'订阅有效',expiring_today:'今日到期',expired:'订阅已到期',grace_period:'宽限期',unknown:'有效期未知'} as Record<string,string>)[String(value)]??'有效期未知'
}
export function syncLabel(value: unknown) {
 return ({confirmed:'已确认',pending:'待确认',failed:'查询失败',unsupported:'上游不支持查询',unknown:'未查询'} as Record<string,string>)[String(value)]??'未查询'
}
