import {useEffect, useId, useLayoutEffect, useRef, useState} from 'react'
import {createPortal} from 'react-dom'
import {Copy, X} from 'lucide-react'
import {formatTime} from './client'
import {errorGuide} from './errorGuide'
import {Modal} from './Modal'
import {logStatus, logValue, type UsageLog} from './logView'

type Log = UsageLog
const errorText=(log:Log)=>log.error_message?.trim()||log.upstream_error_kind||String(log.internal_reason||'未记录错误正文')

export function ErrorPreview({log,onDetail}:{log:Log;onDetail:()=>void}) {
 const [open,setOpen]=useState(false),[pinned,setPinned]=useState(false),[copied,setCopied]=useState('')
 const trigger=useRef<HTMLButtonElement>(null),panel=useRef<HTMLDivElement>(null),timer=useRef<ReturnType<typeof setTimeout>|null>(null),skipFocus=useRef(false)
 const [position,setPosition]=useState({left:12,top:12}),id=useId()
 const cancel=()=>{if(timer.current!==null)clearTimeout(timer.current);timer.current=null}
 const close=()=>{cancel();setOpen(false);setPinned(false);setCopied('')}
 const leave=()=>{cancel();if(!pinned)timer.current=setTimeout(()=>setOpen(false),180)}
 useEffect(()=>()=>{if(timer.current!==null)clearTimeout(timer.current)},[])
 useLayoutEffect(()=>{
  if(!open)return
  const anchor=trigger.current?.getBoundingClientRect(),box=panel.current?.getBoundingClientRect()
  if(!anchor||!box)return
  const below=anchor.bottom+8
  setPosition({left:Math.max(12,Math.min(anchor.right-box.width,innerWidth-box.width-12)),top:Math.max(12,Math.min(below+box.height<=innerHeight-12?below:anchor.top-box.height-8,innerHeight-box.height-12))})
 },[open])
 useEffect(()=>{
  if(!open)return
  const outside=(event:PointerEvent)=>{if(!panel.current?.contains(event.target as Node)&&!trigger.current?.contains(event.target as Node))close()}
  const key=(event:KeyboardEvent)=>{if(event.key==='Escape'){event.stopPropagation();close();skipFocus.current=true;trigger.current?.focus();skipFocus.current=false}}
  const scroll=(event:Event)=>{if(!panel.current?.contains(event.target as Node))close()}
  document.addEventListener('pointerdown',outside);document.addEventListener('keydown',key,true)
  window.addEventListener('resize',close);document.addEventListener('scroll',scroll,true)
  return()=>{document.removeEventListener('pointerdown',outside);document.removeEventListener('keydown',key,true);window.removeEventListener('resize',close);document.removeEventListener('scroll',scroll,true)}
 },[open])
 const guide=errorGuide(log)
 const text=errorText(log),hasError=Boolean(log.error_message||log.upstream_error_kind||log.internal_reason)
 if(!hasError)return <span className="muted">—</span>
 return <><button ref={trigger} className="log-error-trigger" aria-label={`查看错误详情 ${log.request_id||log.id}`} aria-expanded={open} aria-controls={open?id:undefined}
 onPointerEnter={event=>{if(event.pointerType==='mouse'){cancel();setOpen(true)}}} onPointerLeave={leave}
 onFocus={()=>{if(!skipFocus.current){cancel();setOpen(true)}}} onBlur={event=>{if(!panel.current?.contains(event.relatedTarget))leave()}}
 onClick={()=>{cancel();setOpen(true);setPinned(true)}}><span>{text.replace(/\s+/g,' ').slice(0,100)}</span></button>
 {open&&createPortal(<div ref={panel} id={id} className="log-popover" role="dialog" aria-label="错误详情" style={position}
 onPointerEnter={cancel} onPointerLeave={leave} onFocus={cancel} onBlur={event=>{if(!panel.current?.contains(event.relatedTarget)&&event.relatedTarget!==trigger.current)leave()}}>
 <div className="log-popover-heading"><strong>{log.upstream_error_kind||'错误详情'}</strong><button aria-label="关闭错误详情" onClick={close}><X size={15}/></button></div>
 <div className="error-guide"><strong>{guide.label}{guide.inferred?'（按错误信号判断）':''}</strong><p>{guide.help}</p><small>{guide.action}</small></div><pre>{text}</pre><div className="log-popover-actions"><button onClick={async()=>{try{await navigator.clipboard.writeText(text);setCopied('已复制错误')}catch{setCopied('复制失败，请选中正文复制')}}}><Copy size={14}/>复制错误</button><button onClick={()=>{close();onDetail()}}>完整请求详情</button></div><small role="status">{copied||'悬停查看 · 点击固定 · Esc 关闭'}</small>
 </div>,document.body)}</>
}
export function LogDetails({log,onClose,onChain}:{log:Log;onClose:()=>void;onChain?:()=>void}) {
 const [notice,setNotice]=useState('')
 const fields:[string,unknown][]=[['记录时间',formatTime(log.created_at)],['请求 ID',log.request_id],['上游请求 ID',log.upstream_request_id],['父请求 ID',log.parent_request_id],['账号',`${logValue(log.account_name)} · #${logValue(log.account_id)}`],['渠道密钥',`${logValue(log.api_key_name)} · #${logValue(log.api_key_id)}`],['请求模型',log.model],['实际发送模型',log.effective_model],['上游返回模型',log.upstream_response_model],['请求入口',log.inbound_endpoint||log.endpoint],['上游入口',log.upstream_endpoint],['客户端 IP',log.client_ip],['首字延迟',logValue(log.first_token_ms,' ms')],['总耗时',logValue(log.duration_ms,' ms')],['输入 / 输出 Token',`${logValue(log.input_tokens??log.prompt_tokens)} / ${logValue(log.output_tokens??log.completion_tokens)}`],['缓存 / 推理 Token',`${logValue(log.cached_tokens)} / ${logValue(log.reasoning_tokens)}`],['流式响应',log.stream===undefined?'—':log.stream?'是':'否'],['上游传输',log.via_websocket===undefined?'—':log.via_websocket?'WebSocket':'HTTP'],['失败后计划重试',log.is_retry_attempt===undefined?'—':log.is_retry_attempt?'是':'否'],['尝试序号',log.attempt_index],['内部原因',log.internal_reason],['客户端 User-Agent',log.client_user_agent],['上游 User-Agent',log.upstream_user_agent]]
 const status=logStatus(log.status_code),guide=errorGuide(log)
 return <Modal title="请求详情" onClose={onClose}><div className="log-detail-heading">{onChain&&<button onClick={onChain}>查看请求链路</button>}<span className={`badge ${status.tone}`} title={status.help}>{status.label}</span><button disabled={!log.request_id} onClick={async()=>{try{await navigator.clipboard.writeText(log.request_id!);setNotice('已复制请求 ID')}catch{setNotice('复制失败，请选中请求 ID 复制')}}}><Copy size={14}/>复制请求 ID</button></div>{notice&&<p role="status">{notice}</p>}<dl className="facts">{fields.map(([label,value])=><div key={label}><dt>{label}</dt><dd>{logValue(value)}</dd></div>)}</dl><div className="error-guide"><strong>{guide.label}</strong><p>{guide.help}</p><small>{guide.action}</small></div><h2>错误正文</h2><pre>{errorText(log)}</pre><details><summary>原始日志字段</summary><pre>{JSON.stringify(log,null,2)}</pre></details></Modal>
}

