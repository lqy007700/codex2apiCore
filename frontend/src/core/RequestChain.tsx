import {useEffect, useState} from 'react'
import {ArrowDown, Copy, RefreshCw} from 'lucide-react'
import {formatTime, request} from './client'
import {ErrorPreview} from './LogEntry'
import {Modal} from './Modal'
import {chainTransition, logStatus, logValue, summarizeChain, type UsageLog} from './logView'

type Chain = {request_id:string;linked:boolean;log_mode:string;records:UsageLog[];total:number;truncated:boolean}
export function RequestChain({logID,onClose}:{logID:number;onClose:()=>void}) {
 const [data,setData]=useState<Chain|null>(null),[loading,setLoading]=useState(true),[error,setError]=useState(''),[revision,setRevision]=useState(0),[expanded,setExpanded]=useState<number|null>(null),[notice,setNotice]=useState('')
 useEffect(()=>{
  const controller=new AbortController();setLoading(true);setError('');setNotice('')
  void request<Chain>(`/usage/logs/${logID}/chain`,{signal:AbortSignal.any([controller.signal,AbortSignal.timeout(30000)])}).then(value=>{if(!controller.signal.aborted)setData(value)}).catch(error=>{if(!controller.signal.aborted)setError(error instanceof Error?error.message:String(error))}).finally(()=>{if(!controller.signal.aborted)setLoading(false)})
  return()=>controller.abort()
 },[logID,revision])
 const records=data?.records??[],stats=summarizeChain(records),last=records[records.length-1],status=logStatus(last?.status_code)
 return <Modal title="请求链路" onClose={onClose}>
 <div className="chain-heading"><div><small>网关请求 ID</small><code>{data?.request_id||'未记录关联 ID'}</code></div><div className="actions"><button disabled={!data?.request_id} aria-label="复制链路请求 ID" onClick={async()=>{try{await navigator.clipboard.writeText(data!.request_id);setNotice('已复制请求 ID')}catch{setNotice('复制失败，请选中请求 ID 复制')}}}><Copy size={14}/></button><button disabled={loading} onClick={()=>setRevision(previous=>previous+1)}><RefreshCw size={14}/>刷新链路</button></div></div>
 {notice&&<p role="status">{notice}</p>}
 <p className="section-help">按同一网关请求 ID 查看所有保留记录，不受列表的时间、账号、错误筛选或分页限制。日志异步写入，刚完成或仍在执行时可刷新。</p>
 {error&&<div className="notice error" role="alert">{error}<button onClick={()=>setRevision(previous=>previous+1)}>重试</button></div>}{loading&&<p role="status" className="muted">正在读取请求链路…</p>}
 {data&&<>
 {!data.linked&&<div className="notice error">此记录没有请求关联 ID，只能展示本条记录，无法还原其他尝试。</div>}
 {data.log_mode!=='full'&&<div className="notice error">当前日志模式为{data.log_mode==='errors'?'仅错误':'关闭'}，成功或后续尝试可能未保存；不能据此判断完整链路。</div>}
 {data.truncated&&<div className="notice error">此请求共有 {data.total} 条保留记录，当前仅展示最早 500 条，最后记录不代表最终结果。</div>}
 {(stats.gaps||stats.ambiguous||stats.unindexed>0)&&<div className="chain-note">{stats.gaps&&`尝试序号存在缺失，已记录到第 ${stats.highest} 次。`}{stats.ambiguous&&'同一尝试序号有多条记录，统计按序号去重。'}{stats.unindexed>0&&`${stats.unindexed} 条内部或旧记录没有尝试序号，未计入重试次数。`}以下次数仅按已保存记录计算。</div>}
 <div className="chain-summary"><div><small>已记录尝试</small><strong>{stats.attempts||'未知'}</strong></div><div><small>已确认重试</small><strong>{stats.attempts?stats.retries:'未知'}</strong></div><div><small>已确认换号</small><strong>{stats.attempts?stats.switches:'未知'}</strong></div><div><small>最后记录</small><span className={`badge ${status.tone}`} title={status.help}>{status.label}</span></div></div>
 <ol className="chain-timeline">{records.map((row,index)=>{
 const state=logStatus(row.status_code),attempt=Number(row.attempt_index)
 return <li key={row.id}>
 {index>0&&<div className="chain-transition"><ArrowDown size={14}/>{chainTransition(records[index-1],row)}</div>}
 <article className={`chain-node ${row.id===logID?'chain-selected':''}`}>
 <div className="chain-node-heading"><strong>{attempt>0?attempt===1?'首次尝试':`第 ${attempt} 次尝试`:'内部 / 旧执行记录'}</strong><span className={`badge ${state.tone}`} title={state.help}>{state.label}</span>{row.id===logID&&<small>选中的日志</small>}</div>
 <div className="chain-account"><strong>{String(row.account_name||`账号 #${logValue(row.account_id)}`)}</strong><span>账号 ID：{logValue(row.account_id)}</span><span>{row.via_websocket?'WebSocket':'HTTP'} · {row.stream?'流式':'非流式'}</span></div>
 <div className="chain-meta"><span>记录：{formatTime(row.created_at)}</span><span>模型：{logValue(row.effective_model||row.model)}</span><span>耗时：{logValue(row.duration_ms,' ms')}</span><span>首字：{logValue(row.first_token_ms,' ms')}</span></div>
 <div className="chain-error"><ErrorPreview log={row} onDetail={()=>setExpanded(row.id)}/>{row.is_retry_attempt===true&&<small>此记录标记了重试计划</small>}</div>
 {row.is_retry_attempt===true&&!records.some(next=>Number(next.attempt_index)>attempt)&&<p className="chain-pending">尚无后续尝试记录，不能确认计划的重试已经执行。</p>}
 <details open={expanded===row.id} onToggle={event=>{if(event.currentTarget.open)setExpanded(row.id);else if(expanded===row.id)setExpanded(null)}}><summary>查看本次执行字段</summary><dl className="facts">{[
 ['上游请求 ID',row.upstream_request_id],['父请求 ID',row.parent_request_id],['上游代理',row.upstream_proxy_name],['请求入口',row.endpoint],['上游入口',row.upstream_endpoint],['请求模型',row.model],['实际发送模型',row.effective_model],['输入 / 输出 Token',`${logValue(row.input_tokens)} / ${logValue(row.output_tokens)}`],['缓存 Token',row.cached_tokens],['内部原因',row.internal_reason]
 ].map(([label,value])=><div key={String(label)}><dt>{String(label)}</dt><dd>{logValue(value)}</dd></div>)}</dl>{row.error_message&&<pre>{row.error_message}</pre>}</details>
 </article></li>
 })}</ol>
 {!records.length&&<div className="empty">没有保留的执行记录，请刷新请求列表。</div>}
 </>}
 </Modal>
}
