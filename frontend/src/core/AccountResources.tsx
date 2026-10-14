import {useState} from 'react'
import {formatTime, request, send, type JsonRecord} from './client'
import {accountState, finiteNumber, longWindowLabel, planLabel, stateInfo, subscriptionLabel, syncLabel, usagePercent} from './accountView'
import type {Account, ResetCards} from './types'

type Run = (task: () => Promise<unknown>, success?: string) => Promise<unknown>
export function QuotaWindow({account,window,compact=false}:{account:Account;window:'5h'|'7d'|'spark';compact?:boolean}) {
 const percent=window==='spark'?finiteNumber(account.usage_percent_spark):usagePercent(account,window)
 const title=window==='5h'?'5 小时':window==='spark'?'Spark':longWindowLabel(account)
 const reset=account[window==='5h'?'reset_5h_at':window==='spark'?'reset_spark_at':'reset_7d_at']
 return <div className={`quota-window ${compact?'compact':''}`}><div className="quota-heading"><strong>{title}</strong><span>{percent===null?'未获取':`已用 ${percent.toFixed(1)}%`}</span></div>{percent!==null&&<progress aria-label={`${title}额度已用比例`} max={100} value={Math.min(100,Math.max(0,percent))}/>}<small>重置：{reset?formatTime(reset):'未获取'}</small>{!compact&&<small>{percent===null?'点击“刷新额度”查询上游快照。':`剩余约 ${Math.max(0,100-percent).toFixed(1)}%；以最近一次上游快照为准。`}</small>}</div>
}
export function AccountResources({account,busy,run,onChanged}:{account:Account;busy:boolean;run:Run;onChanged:()=>Promise<void>}) {
 const [cards,setCards]=useState<ResetCards|null>(null),[queriedAt,setQueriedAt]=useState(''),[usage,setUsage]=useState<JsonRecord|null>(null)
 const [resetNotice,setResetNotice]=useState(''),[subscriptionNotice,setSubscriptionNotice]=useState('')
 const state=accountState(account),subscription=account.subscription
 const queryCards=()=>void run(async()=>{
  setCards(null);const result=await request<ResetCards>(`/accounts/${account.id}/reset-credits`)
  setCards(result);setQueriedAt(new Date().toISOString());await onChanged()
 },'重置卡查询完成')
 const consume=()=>{
  if(!cards||cards.available_count<=0||busy)return
  if(!window.confirm(`为账号 ${account.email||account.name||account.id} 使用 1 张重置卡？这会消耗一次上游重置机会，无法撤销。`))return
  void run(async()=>{
   setCards(null);setResetNotice('')
   const result=await send(`/accounts/${account.id}/reset-credits`)
   setResetNotice(`重置请求已成功${result.rate_limit_reset_credits===undefined?'':`，剩余 ${result.rate_limit_reset_credits} 张`}。${result.usage_refreshed===false?'额度快照尚未同步，请稍后刷新额度；请勿重复使用卡。':'额度快照已同步。'}`)
   await onChanged()
  },'已使用 1 张重置卡')
 }
 const refreshSubscription=()=>void run(async()=>{
  setSubscriptionNotice('');const result=await send(`/accounts/${account.id}/subscription/refresh`)
  await onChanged()
  if(result.outcome==='failed')throw new Error(String(result.error??'订阅查询失败，保留上次已知信息'))
  setSubscriptionNotice(result.outcome==='unsupported'?'上游不支持查询此套餐的订阅到期时间。':result.outcome==='no_subscription'?'上游未返回订阅记录；不能据此假定永久有效。':'订阅信息已查询。')
 },'订阅查询已结束')
 return <>
  <section className="account-section"><h3>账号状态与原因</h3><div className="account-summary"><span className={`badge ${state.tone}`} title={state.help}>{state.label}</span><strong>{planLabel(account.plan_type)}</strong>{account.using_credits&&<span className="badge neutral">正在使用积分</span>}</div><p className="section-help">{state.help}</p>{account.error_message&&<div role="alert" className="notice error">上游错误：{account.error_message}</div>}<dl className="facts"><div><dt>健康档位</dt><dd title={stateInfo(account.health_tier).help}>{stateInfo(account.health_tier).label}</dd></div><div><dt>冷却原因</dt><dd>{account.cooldown_reason?stateInfo(account.cooldown_reason).label:'无已知冷却'}</dd></div><div><dt>冷却结束</dt><dd>{formatTime(account.cooldown_until)}</dd></div><div><dt>当前并发 / 实际上限</dt><dd>{account.active_requests??0} / {account.dynamic_concurrency_limit??'未获取'}</dd></div></dl><p className="section-help">健康档位：{stateInfo(account.health_tier).help}</p><p className="section-help">“重置状态”只清理本地错误与冷却记录，不会刷新 Token、恢复上游额度或消耗重置卡。</p>
  </section>
  <section className="account-section"><div className="section-heading"><h3>上游额度</h3><button disabled={busy} onClick={()=>void run(async()=>{await send(`/accounts/${account.id}/usage/refresh`);await onChanged()},'额度已刷新')}>刷新额度</button></div><p className="section-help">百分比表示已用额度。额度自然重置、订阅到期和 Token 到期是不同时间；显示时间使用浏览器所在时区。</p><div className="quota-grid"><QuotaWindow account={account} window="5h"/><QuotaWindow account={account} window="7d"/>{account.usage_percent_spark!=null&&<QuotaWindow account={account} window="spark"/>}</div><dl className="facts"><div><dt>额度快照刷新时间</dt><dd>{formatTime(account.codex_usage_updated_at)}</dd></div><div><dt>5h 快照刷新时间</dt><dd>{formatTime(account.codex_5h_usage_updated_at)}</dd></div><div><dt>已知重置卡张数</dt><dd>{account.rate_limit_reset_credits??'未获取'}</dd></div><div><dt>已知积分余额</dt><dd>{account.credits_valid?(account.credits_balance??'未返回余额'):'未获取'}</dd></div></dl>
  </section>
  <section className="account-section"><div className="section-heading"><h3>有效期与刷新时间</h3><button disabled={busy} onClick={refreshSubscription}>查询订阅有效期</button></div><dl className="facts"><div><dt>订阅到期时间</dt><dd>{formatTime(subscription?.expires_at||account.subscription_expires_at)}</dd></div><div><dt>订阅状态</dt><dd>{subscription?subscriptionLabel(subscription.business_status):'未获取'}</dd></div><div><dt>订阅同步状态</dt><dd>{syncLabel(subscription?.sync_state)}</dd></div><div><dt>订阅最近查询</dt><dd>{formatTime(subscription?.last_checked_at)}</dd></div><div><dt>Access Token 到期</dt><dd>{formatTime(account.access_token_expires_at)}</dd></div><div><dt>凭据最近刷新</dt><dd>{formatTime(account.codex_last_refresh_at)}</dd></div><div><dt>最近使用时间</dt><dd>{formatTime(account.last_used_at)}</dd></div><div><dt>账号导入时间</dt><dd>{formatTime(account.created_at)}</dd></div></dl><p className="section-help">{account.at_only?'当前仅有 Access Token，过期后通常需重新导入有效凭据。':'后台按 Token 到期时间和刷新策略巡检；不会把订阅到期时间当作 Token 到期时间。'}未返回的时间显示“—”，不表示永不过期。</p>{account.codex_refresh_error&&<div className="notice error">最近凭据刷新提示：{account.codex_refresh_error}</div>}{subscription?.error&&<div className="notice error">订阅查询提示：{subscription.error}</div>}{subscriptionNotice&&<p className="section-help" role="status">{subscriptionNotice}</p>}
  </section>
  <section className="account-section"><div className="section-heading"><h3>额度重置卡</h3><button disabled={busy} onClick={queryCards}>{cards?'重新查询重置卡':'查看重置卡'}</button></div><p className="section-help">查询会访问上游获取可用张数与每张卡的可消费截止时间，不会消耗重置卡。只有确认“使用 1 张重置卡”才会消耗。若请求结果不确定，先重新查询，不要重复点击使用。</p>{cards&&<><p>当前可用 <strong>{cards.available_count}</strong> 张 <small className="muted">查询时间：{formatTime(queriedAt)}</small></p>{cards.credits.length>0?<div className="table-card"><table><thead><tr><th>卡片</th><th>发放时间</th><th>可消费截止时间</th></tr></thead><tbody>{cards.credits.map((card,index)=><tr key={card.id}><td>重置卡 {index+1}<small>{card.id}</small></td><td>{formatTime(card.granted_at)}</td><td>{formatTime(card.expires_at)}</td></tr>)}</tbody></table></div>:<p className="section-help">{cards.available_count>0?'上游返回了可用张数，但未返回卡片明细。':'当前没有可用重置卡。'}</p>}<button className="primary" disabled={busy||cards.available_count<=0} onClick={consume}>使用 1 张重置卡</button></>}{resetNotice&&<p role="status" className="reset-notice">{resetNotice}</p>}
  </section>
  <section className="account-section"><div className="section-heading"><h3>后端记录的用量统计</h3><button disabled={busy} onClick={()=>void run(async()=>{setUsage(await request(`/accounts/${account.id}/usage?days=30`))},'近 30 天用量已读取')}>查看近 30 天用量</button></div><p className="section-help">这是此后端记录的请求与 Token 统计，和上游套餐剩余额度不同。日志关闭或只记录错误时，历史明细可能不完整。</p>{usage&&<dl className="facts">{[['请求数','total_requests'],['总 Token','total_tokens'],['输入 Token','input_tokens'],['输出 Token','output_tokens'],['缓存 Token','cached_tokens'],['错误请求数','error_requests']].map(([label,key])=><div key={key}><dt>{label}</dt><dd>{usage[key]==null?'未返回':String(usage[key])}</dd></div>)}</dl>}
  </section>
 </>
}
