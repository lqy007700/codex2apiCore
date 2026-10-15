import {useEffect, useState} from 'react'
import {ChevronLeft, ChevronRight, RefreshCw, Search} from 'lucide-react'
import {formatTime, request} from './client'
import {ErrorPreview, LogDetails} from './LogEntry'
import {errorGuide} from './errorGuide'
import {RequestChain} from './RequestChain'
import {defaultLogFilters, logQuery, logStatus, logTimeRange, logValue, type LogFilters, type UsageLog} from './logView'

type LogPage = {logs:UsageLog[];total:number;log_mode?:string}

export function RequestLogs() {
 const [view,setView]=useState<'requests'|'attempts'>('requests')
 const [draft,setDraft]=useState<LogFilters>({...defaultLogFilters})
 const [query,setQuery]=useState(()=>({filters:{...defaultLogFilters},range:logTimeRange(defaultLogFilters),page:1,pageSize:20,revision:0}))
 const [data,setData]=useState<LogPage|null>(null),[loading,setLoading]=useState(true),[error,setError]=useState(''),[validation,setValidation]=useState(''),[detail,setDetail]=useState<UsageLog|null>(null),[chain,setChain]=useState<number|null>(null)
 const path=`/usage/${view==='requests'?'requests':'logs'}?${logQuery(query.filters,query.range,query.page,query.pageSize)}`
 useEffect(()=>{
  const controller=new AbortController();setLoading(true);setData(null);setError('');setDetail(null);setChain(null)
  void request<LogPage>(path,{signal:AbortSignal.any([controller.signal,AbortSignal.timeout(30000)])}).then(result=>{if(!controller.signal.aborted){const last=Math.max(1,Math.ceil(result.total/query.pageSize));if(query.page>last)setQuery(previous=>({...previous,page:last}));else setData(result)}}).catch(error=>{if(!controller.signal.aborted)setError(error instanceof Error?error.message:String(error))}).finally(()=>{if(!controller.signal.aborted)setLoading(false)})
  return()=>controller.abort()
 },[path,query.revision])
 const field=(key:keyof LogFilters,value:string)=>setDraft(previous=>({...previous,[key]:value}))
 const apply=(filters=draft)=>{
  try{const range=logTimeRange(filters);logQuery(filters,range,1,query.pageSize);setValidation('');setDraft({...filters});setQuery({...query,filters:{...filters},range,page:1,revision:query.revision+1})}
  catch(error){setValidation(error instanceof Error?error.message:String(error))}
 }
 const refresh=()=>setQuery(previous=>({...previous,range:logTimeRange(previous.filters),revision:previous.revision+1}))
 const input=(key:keyof LogFilters,label:string,placeholder='',numeric=false)=><label className="field">{label}<input aria-label={label} value={draft[key]} onChange={event=>field(key,event.target.value)} placeholder={placeholder} type={numeric?'number':'text'} min={numeric?1:undefined} step={numeric?1:undefined}/></label>
 const select=(key:keyof LogFilters,label:string,options:[string,string][])=><label className="field">{label}<select aria-label={label} value={draft[key]} onChange={event=>field(key,event.target.value)}>{options.map(([value,name])=><option key={value} value={value}>{name}</option>)}</select></label>
 const logs=data?.logs??[],total=data?.total??0,pages=Math.max(1,Math.ceil(total/query.pageSize))
 return <><header className="page-header"><div><p className="eyebrow">CODEX ACCOUNT SERVICE</p><h1>请求日志</h1><p>按时间与条件查找执行记录；悬停查看错误，点击固定，详情保留完整诊断信息。</p></div><div className="actions"><button disabled={loading} onClick={refresh}><RefreshCw size={16}/>刷新</button></div></header>
 <div className="log-view-switch" role="group" aria-label="日志视图"><button aria-pressed={view==='requests'} onClick={()=>{setView('requests');setQuery(previous=>({...previous,page:1}))}}>按请求合并</button><button aria-pressed={view==='attempts'} onClick={()=>{setView('attempts');setQuery(previous=>({...previous,page:1}))}}>逐次尝试</button></div>
 <form className="card log-filters" onSubmit={event=>{event.preventDefault();apply()}}>
 <div className="log-filter-grid"><label className="field log-search">搜索日志<input value={draft.q} onChange={event=>field('q',event.target.value)} placeholder="请求 ID、模型、账号名、错误关键词…"/></label>
 {select('status','请求状态',[['all','全部状态'],['error','仅错误（含取消）'],['2xx','成功（2xx）'],['4xx','客户端错误（4xx）'],['5xx','服务端 / 上游错误（5xx）'],['401','401 · 凭据无效'],['403','403 · 拒绝访问'],['429','429 · 限流'],['499','499 · 客户端取消'],['598','598 · 流执行失败']])}
 {select('range','时间范围',[['1','最近 1 小时'],['6','最近 6 小时'],['24','最近 24 小时'],['168','最近 7 天'],['720','最近 30 天'],['custom','自定义时间']])}</div>
 {draft.range==='custom'&&<div className="form-grid log-custom-time"><label className="field">开始时间<input type="datetime-local" required value={draft.start} onChange={event=>field('start',event.target.value)}/></label><label className="field">结束时间<input type="datetime-local" required value={draft.end} onChange={event=>field('end',event.target.value)}/></label><small className="muted">时间使用浏览器本地时区。</small></div>}
 <details className="log-advanced"><summary>更多筛选 · 账号 / 模型 / 渠道 / 传输</summary><div className="log-filter-grid">
 {input('account_id','账号 ID','例如 12',true)}{input('model','模型（精确匹配）','例如 gpt-5.1-codex')}{input('api_key_id','渠道密钥 ID','例如 3',true)}{input('error_kind','错误类型（精确匹配）','例如 rate_limit_exceeded')}
 {select('endpoint','请求入口',[['','全部入口'],['/v1/responses','/v1/responses'],['/v1/chat/completions','/v1/chat/completions'],['/v1/messages','/v1/messages']])}
 {select('stream','响应方式',[['','全部响应'],['true','流式'],['false','非流式']])}{select('via_websocket','上游传输',[['','全部传输'],['true','WebSocket'],['false','HTTP']])}{select('retry','失败后计划重试',[['','全部记录'],['true','有重试计划'],['false','无重试计划']])}
 </div></details>
 {validation&&<p role="alert" className="error-text">{validation}</p>}
 <div className="log-filter-actions"><div className="actions"><button className="primary"><Search size={16}/>查询</button><button type="button" onClick={()=>apply({...defaultLogFilters})}>重置筛选</button><button type="button" className={query.filters.status==='error'?'log-quick-active':''} onClick={()=>apply({...draft,status:query.filters.status==='error'?'all':'error'})}>仅看错误</button></div><small className="muted">{view==='requests'?'时间、账号、关键词匹配任一尝试；状态按最后保留记录筛选':'条件填写后点击查询 · 从数据库筛选全部匹配记录'}</small></div>
 </form>
 <div className="log-results-heading"><div><strong>{data?`共 ${total.toLocaleString()} 条匹配记录`:'请求记录'}</strong><small>{formatTime(query.range.start)} — {formatTime(query.range.end)}</small></div><label className="log-page-size">每页<select aria-label="每页日志条数" value={query.pageSize} onChange={event=>setQuery(previous=>({...previous,page:1,pageSize:Number(event.target.value)}))}>{[20,50,100].map(size=><option key={size} value={size}>{size} 条</option>)}</select></label></div>
 {view==='requests'&&<p className="section-help">一行代表同一网关请求；尝试与 Token 汇总全部保留记录，总耗时仅使用新增实测数据。最后记录有重试计划时结果仍待确认。{data?.log_mode&&data.log_mode!=='full'?'当前未开启完整日志，成功与后续记录可能缺失。':''}</p>}
 {error&&<div role="alert" className="notice error">{error}<button onClick={refresh}>重试</button></div>}
 <div className="table-card log-table" aria-busy={loading}><table><thead><tr><th>时间 / 请求 ID</th><th>{view==='requests'?'最后账号 / 模型':'账号 / 模型'}</th><th>{view==='requests'?'最后记录结果':'状态'}</th><th>{view==='requests'?'已观测总耗时':'首字 / 总耗时'}</th><th>输入 / 输出 Token</th><th>错误摘要</th><th>操作</th></tr></thead><tbody>{logs.map(log=>{
 const status=view==='requests'&&log.is_retry_attempt===true?{label:'待后续记录',tone:'neutral',help:'最后一条记录计划重试，尚不能确认请求结果。'}:logStatus(log.status_code),guide=errorGuide(log)
 return <tr key={log.id}><td className="log-time">{formatTime(log.created_at)}<button className="log-id" onClick={()=>setChain(log.id)}>{String(log.request_id||'未记录请求 ID')}</button></td><td className="log-account"><strong>{String(log.account_name||`账号 #${logValue(log.account_id)}`)}</strong><small>{logValue(log.model)}</small>{view==='requests'&&<small>{Number(log.attempt_count)>0?`${logValue(log.attempt_count)} 次已记录尝试 · ${logValue(log.retry_count)} 次重试`:'尝试次数未知'} · {logValue(log.account_count)} 个账号</small>}{log.missing_attempts===true&&<small className="error-text">尝试序号有缺失</small>}<div className="log-tags">{log.via_websocket===true&&<span>WS</span>}{log.stream===true&&<span>流式</span>}{Number(log.attempt_index)>1&&<span>第 {logValue(log.attempt_index)} 次尝试</span>}{log.is_retry_attempt===true&&<span>计划重试</span>}</div></td><td><span className={`badge ${status.tone}`} title={status.help}>{status.label}</span></td><td className="log-numbers">{view==='requests'?logValue(log.elapsed_ms):`${logValue(log.first_token_ms)} / ${logValue(log.duration_ms)}`}<small>ms</small></td><td className="log-numbers">{logValue(log.input_tokens??log.prompt_tokens)} / {logValue(log.output_tokens??log.completion_tokens)}<small>缓存 {logValue(log.cached_tokens)}</small></td><td className="log-error-cell">{Number(log.status_code)>=400&&<small className="error-classification" title={guide.action}>{guide.label}</small>}<ErrorPreview log={log} onDetail={()=>setDetail(log)}/></td><td><div className="actions"><button className="log-detail-button" aria-label={`请求链路 ${log.request_id||log.id}`} onClick={()=>setChain(log.id)}>链路</button>{view==='attempts'&&<button className="log-detail-button" aria-label={`请求详情 ${log.request_id||log.id}`} onClick={()=>setDetail(log)}>详情</button>}</div></td></tr>
 })}</tbody></table>{loading?<div className="empty" role="status">正在查询…</div>:!error&&!logs.length?<div className="empty">此时间范围内没有匹配日志，请调整筛选条件或时间范围。</div>:null}</div>
 <div className="pagination log-pagination"><span>{data?`第 ${query.page} / ${pages} 页 · 本页 ${logs.length} 条`:'—'}</span><button disabled={loading||!data||query.page<=1} onClick={()=>setQuery(previous=>({...previous,page:previous.page-1}))}><ChevronLeft size={16}/>上一页</button><button disabled={loading||!data||query.page>=pages} onClick={()=>setQuery(previous=>({...previous,page:previous.page+1}))}>下一页<ChevronRight size={16}/></button></div>
 {chain!==null&&<RequestChain logID={chain} onClose={()=>setChain(null)}/>}
 {detail&&<LogDetails log={detail} onClose={()=>setDetail(null)} onChain={()=>{setChain(detail.id);setDetail(null)}}/>}</>
}
