import type {UsageLog} from './logView'
type Event={sequence:number;kind:string;at_ms:number;duration_ms?:number;account_id?:number}
type Trace={started_at:string;elapsed_ms:number;events:Event[];truncated:boolean}
const labels:Record<string,string>={request_received:'收到请求',selection_started:'开始选号',selection_finished:'选号结束（含排队）',queue_started:'进入账号等待队列',queue_finished:'账号排队结束',account_selected:'使用账号',upstream_started:'开始上游执行',utls_connection_started:'开始获取 uTLS 连接',utls_connection_finished:'uTLS 连接获取结束',http_connection_started:'开始获取 HTTP 连接',http_connection_finished:'HTTP 连接就绪',ws_connection_finished:'WS 连接获取结束',request_sent:'已发送上游请求',response_headers:'收到上游首字节',first_content:'收到首个有效内容',attempt_failed:'本次执行失败',attempt_finished:'本次执行结束',retry_planned:'决定继续重试',retry_wait_started:'开始重试等待',retry_wait_finished:'重试等待结束'}
export function parseExecutionTrace(records:UsageLog[]):Trace|null {
 let result:Trace|null=null
 for(const row of records){
  if(typeof row.execution_trace!=='string'||!row.execution_trace)continue
  try{const value=JSON.parse(row.execution_trace) as Trace;if(Array.isArray(value.events)&&typeof value.elapsed_ms==='number'&&value.events.every(event=>typeof event.kind==='string'&&Number.isFinite(event.at_ms))&&(!result||value.elapsed_ms>result.elapsed_ms))result=value}catch{}
 }
 return result
}
export function ExecutionTimeline({records}:{records:UsageLog[]}) {
 const trace=parseExecutionTrace(records)
 if(!trace)return <div className="chain-note">旧日志没有阶段事件打点，无法还原排队、连接和重试等待耗时；升级后新请求会记录。原有耗时仍保留在下方执行记录中。</div>
 const totals=new Map<string,number>()
 for(const event of trace.events)if(event.duration_ms!==undefined)totals.set(event.kind,(totals.get(event.kind)??0)+event.duration_ms)
 const metrics=[['选号（含排队）','selection_finished'],['其中账号排队','queue_finished'],['HTTP 取连','http_connection_finished'],['uTLS 取连','utls_connection_finished'],['WS 取连','ws_connection_finished'],['重试等待','retry_wait_finished']]
 const waits:number[]=[];let sent:number|undefined
 for(const event of trace.events){if(event.kind==='upstream_started')sent=undefined;if(event.kind==='request_sent')sent=event.at_ms;if(event.kind==='first_content'&&sent!==undefined){waits.push(Math.max(0,event.at_ms-sent));sent=undefined}}
 return <section className="execution-section"><h3>事件与分段耗时</h3><p className="section-help">已观测至最近落库点：{trace.elapsed_ms} ms。选号包含账号排队，各阶段可能嵌套，不能直接相加；连接获取可能复用已有连接。</p><dl className="execution-metrics">{metrics.map(([label,key])=><div key={key}><dt>{label}</dt><dd>{totals.has(key)?`${totals.get(key)} ms`:'未采集'}</dd></div>)}<div><dt>发送至首内容（各次）</dt><dd>{waits.length?waits.map(ms=>`${ms} ms`).join(' / '):'未采集'}</dd></div></dl>{trace.truncated&&<div className="chain-note">事件已达到 128 条上限，后续事件缺失，耗时与重试记录可能不完整。</div>}<details className="execution-details"><summary>查看全部 {trace.events.length} 条事件</summary><ol className="execution-events">{trace.events.map(event=><li key={event.sequence}><code>+{event.at_ms} ms</code><span>{labels[event.kind]??event.kind}{event.account_id?` · 账号 #${event.account_id}`:''}</span>{event.duration_ms!==undefined&&<small>{event.duration_ms} ms</small>}</li>)}</ol></details></section>
}
