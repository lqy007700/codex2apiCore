import {useCallback, useEffect, useRef, useState, type FormEvent, type ReactNode} from 'react'
import {NavLink, Navigate, Route, Routes, useNavigate} from 'react-router-dom'
import {Activity, ArrowDownToLine, Check, ChevronLeft, ChevronRight, Fingerprint, KeyRound, LogOut, Plus, RefreshCw, Server, Settings, Shield, Users, X} from 'lucide-react'
import {adminKey, formatTime, jsonObject, request, secretKey, send, writableSettings, type JsonRecord} from './client'
import {settingsGroups, type Field} from './settings'
import type {Account, AccountPage, KeyRow, ProxyRow} from './types'

type Task = () => Promise<unknown>
const message = (error: unknown) => error instanceof Error ? error.message : String(error)
function useTask() {
 const [busy,setBusy] = useState(false)
 const [notice,setNotice] = useState<{text:string;error:boolean}|null>(null)
 const running = useRef(false)
 const run = useCallback(async (task: Task, success = '操作完成') => {
  if (running.current) return
  running.current = true; setBusy(true); setNotice(null)
  try { const result = await task(); setNotice({text:success,error:false}); return result }
  catch(error) {setNotice({text:message(error),error:true})}
  finally {running.current = false;setBusy(false)}
 },[])
 return {busy,run,notice,setNotice}
}
function Notice({notice}:{notice:ReturnType<typeof useTask>['notice']}) {
 return notice ? <div role={notice.error?'alert':'status'} className={`notice ${notice.error?'error':'success'}`}>{notice.text}</div> : null
}
function useData<T>(path:string) {
 const [value,setValue]=useState<T|null>(null), [error,setError]=useState(''), [loading,setLoading]=useState(true)
 const current=useRef(0)
 const refresh=useCallback(async()=>{
  const generation=++current.current;setLoading(true)
  try {const data=await request<T>(path);if(generation===current.current){setValue(data);setError('')}}
  catch(error){if(generation===current.current)setError(message(error))}
  finally {if(generation===current.current)setLoading(false)}
 },[path])
 useEffect(()=>{setValue(null);void refresh();return()=>{current.current++}},[refresh])
 return {value,error,loading,refresh}
}
function Header({title,description,children}:{title:string;description:string;children?:ReactNode}) {
 return <header className="page-header"><div><p className="eyebrow">CODEX ACCOUNT SERVICE</p><h1>{title}</h1><p>{description}</p></div><div className="actions">{children}</div></header>
}
function LoadingError({error,loading,refresh}:{error:string;loading:boolean;refresh:()=>unknown}) {
 return error?<div role="alert" className="notice error">{error}<button onClick={()=>void refresh()}>重试</button></div>:loading?<p className="muted" role="status">正在读取…</p>:null
}
function Modal({title,children,onClose}:{title:string;children:ReactNode;onClose:()=>void}) {
 const box=useRef<HTMLDivElement>(null), close=useRef(onClose)
 close.current=onClose
 useEffect(()=>{
  const previous=document.activeElement as HTMLElement|null
  const bodyOverflow=document.body.style.overflow;document.body.style.overflow='hidden'
  box.current?.focus()
  const key=(event:KeyboardEvent)=>{
   if(event.key==='Escape')close.current()
   if(event.key==='Tab'){
    const elements=box.current?.querySelectorAll<HTMLElement>('button:not(:disabled),input:not(:disabled),select:not(:disabled),textarea:not(:disabled),a[href]')
    if(!elements?.length)return
    if(event.shiftKey && (document.activeElement===elements[0] || document.activeElement===box.current)){event.preventDefault();elements[elements.length-1].focus()}
    else if(!event.shiftKey && document.activeElement===elements[elements.length-1]){event.preventDefault();elements[0].focus()}
   }
  }
  document.addEventListener('keydown',key)
  return()=>{document.body.style.overflow=bodyOverflow;document.removeEventListener('keydown',key);previous?.focus()}
 },[])
 return <div className="modal-backdrop"><div className="modal" role="dialog" aria-modal="true" aria-label={title} ref={box} tabIndex={-1}><div className="modal-heading"><h2>{title}</h2><button aria-label="关闭" onClick={onClose}><X size={18}/></button></div>{children}</div></div>
}
function Empty({children}:{children:ReactNode}) {return <div className="empty">{children}</div>}
function Status({value}:{value:unknown}) {const text=String(value??'unknown');return <span className={`badge ${['normal','active','ok','healthy','available'].includes(text)?'good':['error','banned','unauthorized','expired'].includes(text)?'bad':'neutral'}`}>{text}</span>}
function FieldInput({field,value,onChange}:{field:Field;value:unknown;onChange:(value:unknown)=>void}) {
 const choices=field.choices??[]
 return <label className="field"><span>{field.label}</span>{field.type==='boolean'?<input type="checkbox" checked={Boolean(value)} onChange={event=>onChange(event.target.checked)}/>:field.type==='select'?<select value={String(value??'')} onChange={event=>onChange(event.target.value)}>{!choices.includes(String(value??''))&&<option value={String(value??'')}>{String(value??'继承默认')}</option>}{choices.map(choice=><option key={choice}>{choice}</option>)}</select>:<input type={field.type} value={String(value??'')} onChange={event=>onChange(field.type==='number'?(event.target.value===''?'':Number(event.target.value)):event.target.value)} step={field.type==='number'?'any':undefined}/>} {field.help&&<small>{field.help}</small>}</label>
}

function Login({onLogin}:{onLogin:()=>void}) {
 const {value:bootstrap,error,refresh}=useData<{needs_bootstrap:boolean}>('/bootstrap-status')
 const [secret,setSecret]=useState(''),[token,setToken]=useState('');const task=useTask()
 const submit=(event:FormEvent)=>{event.preventDefault();void task.run(async()=>{
  if(bootstrap?.needs_bootstrap)await send('/bootstrap','POST',{admin_secret:secret,bootstrap_token:token})
  sessionStorage.setItem(secretKey,secret)
  try{await request('/health')}catch(error){sessionStorage.removeItem(secretKey);throw error}
  onLogin()
 },'已登录')}
 return <main className="login"><form className="login-card" onSubmit={submit}><div className="brand-icon"><Server size={24}/></div><p className="eyebrow">CODEX ACCOUNT SERVICE</p><h1>{bootstrap?.needs_bootstrap?'初始化账号服务':'管理账号池'}</h1><p>New API 负责分发，这里管理 Codex 账号与上游执行。</p><LoadingError error={error} loading={!bootstrap} refresh={refresh}/>{bootstrap?.needs_bootstrap&&<label className="field">初始化令牌<input required autoComplete="off" value={token} onChange={event=>setToken(event.target.value)}/><small>从当前服务日志中的 bootstrap token 获取。</small></label>}<label className="field">管理密钥<input type="password" required autoComplete={bootstrap?.needs_bootstrap?'new-password':'current-password'} value={secret} onChange={event=>setSecret(event.target.value)}/></label><Notice notice={task.notice}/><button className="primary" disabled={task.busy||!bootstrap||Boolean(error)}>{task.busy?'正在验证…':bootstrap?.needs_bootstrap?'初始化并登录':'登录'}</button><small className="muted">密钥只保存在当前标签页的会话存储中。</small></form></main>
}
const navigation=[{path:'/accounts',label:'账号池',icon:Users},{path:'/scheduler',label:'调度状态',icon:Activity},{path:'/proxies',label:'代理出口',icon:Shield},{path:'/logs',label:'请求日志',icon:ArrowDownToLine},{path:'/keys',label:'渠道密钥',icon:KeyRound},{path:'/settings',label:'运行设置',icon:Settings}]
export default function App(){
 const [signedIn,setSignedIn]=useState(Boolean(adminKey()));const navigate=useNavigate()
 useEffect(()=>{const expired=()=>{sessionStorage.removeItem(secretKey);setSignedIn(false)};window.addEventListener('admin-auth-expired',expired);return()=>window.removeEventListener('admin-auth-expired',expired)},[])
 if(!signedIn)return <Login onLogin={()=>setSignedIn(true)}/>
 return <div className="shell"><aside className="sidebar"><div className="brand"><div className="brand-icon"><Server size={22}/></div><div><strong>Codex 账号服务</strong><small>New API 的上游账号池</small></div></div><nav aria-label="管理导航">{navigation.map(({path,label,icon:Icon})=><NavLink key={path} to={path}><Icon size={18}/>{label}</NavLink>)}</nav><div className="sidebar-footer"><span className="badge good">Codex only</span><small>账号身份 · 连接执行 · 调度</small><button onClick={()=>{sessionStorage.removeItem(secretKey);setSignedIn(false);navigate('/accounts')}}><LogOut size={16}/>退出登录</button></div></aside><main className="content"><Routes><Route path="/accounts" element={<Accounts/>}/><Route path="/scheduler" element={<Scheduler/>}/><Route path="/proxies" element={<Proxies/>}/><Route path="/logs" element={<Logs/>}/><Route path="/keys" element={<Keys/>}/><Route path="/settings" element={<SettingsPage/>}/><Route path="*" element={<Navigate to="/accounts" replace/>}/></Routes></main></div>
}
function Accounts(){
 const [page,setPage]=useState(1),[search,setSearch]=useState(''),[query,setQuery]=useState(''),[selected,setSelected]=useState<number[]>([])
 const data=useData<AccountPage>(`/accounts?view=page&page=${page}&page_size=50&channel=codex&search=${encodeURIComponent(query)}`)
 const [adding,setAdding]=useState(false),[editing,setEditing]=useState<number|null>(null);const task=useTask()
 const accounts=data.value?.accounts??[];const total=data.value?.total??data.value?.total_matched??accounts.length
 const bulk=(path:string,body:JsonRecord={})=>void task.run(async()=>{await send(`/accounts/${path}`,'POST',{ids:selected,...body});setSelected([]);await data.refresh()})
 return <><Header title="账号池" description="管理原生 Codex 凭据、配额、指纹和账号调度。"><button onClick={()=>void data.refresh()} disabled={data.loading}><RefreshCw size={16}/>刷新</button><button className="primary" onClick={()=>setAdding(true)}><Plus size={16}/>导入账号</button></Header><Notice notice={task.notice}/><div className="toolbar"><form onSubmit={event=>{event.preventDefault();setPage(1);setSelected([]);setQuery(search)}}><input aria-label="搜索账号" placeholder="搜索名称或邮箱" value={search} onChange={event=>setSearch(event.target.value)}/><button>搜索</button></form><span className="muted">共 {total} 个账号</span></div>{selected.length>0&&<div className="bulk-bar"><strong>已选 {selected.length} 个</strong><button disabled={task.busy} onClick={()=>bulk('batch-refresh')}>刷新凭据</button><button disabled={task.busy} onClick={()=>bulk('batch-refresh-usage')}>刷新配额</button><button disabled={task.busy} onClick={()=>bulk('batch-reset-status')}>重置状态</button><button disabled={task.busy} onClick={()=>bulk('batch-update',{enabled:false})}>禁用</button></div>}<LoadingError {...data}/><div className="table-card"><table><thead><tr><th><input type="checkbox" aria-label="选择本页账号" checked={accounts.length>0&&accounts.every(account=>selected.includes(account.id))} onChange={event=>setSelected(event.target.checked?accounts.map(account=>account.id):[])}/></th><th>账号</th><th>状态</th><th>套餐</th><th>并发 / 上限</th><th>配额 5h / 7d</th><th>调度</th><th/></tr></thead><tbody>{accounts.map(account=><tr key={account.id}><td><input type="checkbox" aria-label={`选择账号 ${account.id}`} checked={selected.includes(account.id)} onChange={event=>setSelected(previous=>event.target.checked?[...previous,account.id]:previous.filter(id=>id!==account.id))}/></td><td><strong>{account.name||account.email||`账号 ${account.id}`}</strong><small>{account.email||`#${account.id}`}</small></td><td><Status value={!account.enabled?'disabled':account.status}/>{account.error_message&&<small className="error-text" title={account.error_message}>{account.error_message.slice(0,90)}</small>}</td><td>{account.plan_type||'—'}</td><td>{account.active_requests??0} / {account.dynamic_concurrency_limit??'—'}</td><td>{account.codex_5h_used_percent??'—'} / {account.codex_7d_used_percent??'—'}</td><td><small>{account.health_tier||'未采样'}</small>{account.scheduler_score?.toFixed(1)??'—'}</td><td><button onClick={()=>setEditing(account.id)}>管理</button></td></tr>)}</tbody></table>{!data.loading&&!accounts.length&&<Empty>尚无匹配账号。导入 Refresh Token 或 Access Token 后开始管理。</Empty>}</div><div className="pagination"><button disabled={page<=1||data.loading} onClick={()=>{setSelected([]);setPage(page-1)}}><ChevronLeft size={16}/>上一页</button><span>第 {page} 页</span><button disabled={page*50>=total||data.loading} onClick={()=>{setSelected([]);setPage(page+1)}}>下一页<ChevronRight size={16}/></button></div>{adding&&<Modal title="导入 Codex 账号" onClose={()=>setAdding(false)}><ImportAccounts onDone={()=>{setAdding(false);void data.refresh()}}/></Modal>}{editing!==null&&<Modal title={`账号 #${editing}`} onClose={()=>setEditing(null)}><AccountEditor id={editing} onChanged={()=>void data.refresh()}/></Modal>}</>
}
function ImportAccounts({onDone}:{onDone:()=>void}){
 const [kind,setKind]=useState('refresh'),[credential,setCredential]=useState(''),[name,setName]=useState(''),[proxy,setProxy]=useState(''),[skip,setSkip]=useState(true)
 const [file,setFile]=useState<File|null>(null);const task=useTask()
 return <form onSubmit={event=>{event.preventDefault();void task.run(async()=>{
  if(kind==='file'){
   if(!file)throw new Error('请选择凭据文件')
   const body=new FormData();body.append('file',file);body.append('format','json');body.append('proxy_url',proxy);body.append('skip_refresh',String(skip))
   const result=await request('/accounts/import',{method:'POST',body})
   if(Number(result.failed)>0)throw new Error(`部分导入失败，请刷新账号列表检查结果：${JSON.stringify(result)}`)
  }else {
   const result=await send(kind==='access'?'/accounts/at':'/accounts','POST',{name,proxy_url:proxy,skip_refresh:skip,[kind==='access'?'access_token':'refresh_token']:credential})
   if(Number(result.failed)>0)throw new Error(`部分导入失败，请刷新账号列表检查结果：${JSON.stringify(result)}`)
  }
  onDone()
 })}}><label className="field">凭据类型<select value={kind} onChange={event=>setKind(event.target.value)}><option value="refresh">Refresh Token（每行一个）</option><option value="access">Access Token（每行一个）</option><option value="file">Codex JSON 凭据文件</option></select></label>{kind==='file'?<label className="field">JSON 文件<input type="file" accept=".json,application/json" required onChange={event=>setFile(event.target.files?.[0]??null)}/></label>:<label className="field">凭据<textarea required rows={7} spellCheck={false} autoComplete="off" value={credential} onChange={event=>setCredential(event.target.value)} placeholder="只在导入时提交，日志不会记录 Token。"/></label>}<div className="form-grid"><label className="field">名称（可选）<input value={name} onChange={event=>setName(event.target.value)}/></label><label className="field">代理 URL（可选）<input value={proxy} onChange={event=>setProxy(event.target.value)}/></label></div><label className="check-field"><input type="checkbox" checked={skip} onChange={event=>setSkip(event.target.checked)}/>导入时跳过立即刷新，交给后台刷新任务</label><Notice notice={task.notice}/><button className="primary" disabled={task.busy}>{task.busy?'导入中…':'导入'}</button></form>
}
const accountFields:Field[]=[{key:'codex_fingerprint_mode',label:'设备与会话指纹收敛',type:'select',choices:['off','device','session','full','single_machine_multi_window']},{key:'base_concurrency_override',label:'账号并发覆盖（留空继承）',type:'number'},{key:'scheduler_priority',label:'调度优先级（留空继承）',type:'number'},{key:'score_bias_override',label:'调度分偏移（留空继承）',type:'number'},{key:'proxy_url',label:'独立代理 URL',type:'text'},{key:'timezone',label:'账号时区',type:'text'},{key:'keep_concurrency_on_degrade',label:'降级时保持配置并发',type:'boolean'},{key:'skip_warm_tier',label:'跳过 warm 健康层',type:'boolean'},{key:'auto_pause_5h_threshold',label:'5h 暂停阈值（%）',type:'number'},{key:'auto_pause_7d_threshold',label:'7d 暂停阈值（%）',type:'number'}]
function AccountEditor({id,onChanged}:{id:number;onChanged:()=>void}){
 const data=useData<Account>(`/accounts/${id}`),[patch,setPatch]=useState<JsonRecord>({}),[headers,setHeaders]=useState(''),[headersEdited,setHeadersEdited]=useState(false),[output,setOutput]=useState('');const task=useTask()
 const account=data.value
 const act=(path:string,body?:unknown)=>void task.run(async()=>{await send(`/accounts/${id}/${path}`,'POST',body);await data.refresh();onChanged()})
 const test=()=>void task.run(async()=>{
  const response=await fetch(`/api/admin/accounts/${id}/test`,{headers:{'X-Admin-Key':adminKey()},signal:AbortSignal.timeout(180_000)})
  const text=await response.text();setOutput(text.slice(-12000));if(!response.ok)throw new Error(`连接测试失败 (${response.status})`)
 },'测试已结束；请查看输出中的成功或失败事件')
 return <><LoadingError {...data}/>{account&&<><div className="account-summary"><strong>{account.email||account.name}</strong><Status value={account.status}/><span>{account.plan_type||'—'}</span></div><div className="actions wrap"><button disabled={task.busy} onClick={()=>act('refresh')}>刷新凭据</button><button disabled={task.busy} onClick={()=>act('usage/refresh')}>刷新配额</button><button disabled={task.busy} onClick={test}>测试连接</button><button disabled={task.busy} onClick={()=>act('enable',{enabled:!account.enabled})}>{account.enabled?'禁用':'启用'}</button><button disabled={task.busy} onClick={()=>act('lock',{locked:!account.locked})}>{account.locked?'解锁':'锁定'}</button><button disabled={task.busy} onClick={()=>act('reset-status')}>重置状态</button></div><form onSubmit={event=>{event.preventDefault();void task.run(async()=>{
 const body={...patch}
 for(const field of accountFields)if(field.type==='number'&&body[field.key]==='')body[field.key]=field.key.endsWith('override')||field.key==='scheduler_priority'?null:0
 if(headersEdited){const custom=jsonObject(headers);if(Object.values(custom).some(value=>typeof value!=='string'))throw new Error('自定义请求头的值必须是字符串');body.custom_headers=custom}
 if(!Object.keys(body).length)throw new Error('没有待保存的修改')
 await send(`/accounts/${id}/scheduler`,'PATCH',body);setPatch({});setHeadersEdited(false);await data.refresh();onChanged()
 },'账号配置已保存')}}><div className="form-grid">{accountFields.map(field=><FieldInput key={field.key} field={field} value={field.key in patch?patch[field.key]:account[field.key]} onChange={value=>setPatch(previous=>({...previous,[field.key]:value}))}/>)}</div><label className="field"><span><Fingerprint size={16}/>账号自定义请求头（JSON）</span><textarea rows={5} spellCheck={false} value={headersEdited?headers:JSON.stringify(account.custom_headers??{},null,2)} onChange={event=>{setHeadersEdited(true);setHeaders(event.target.value)}}/><small>保留原有覆盖优先级；可配置 User-Agent、Originator、Version 和设备身份。</small></label><button className="primary" disabled={task.busy}><Check size={16}/>保存配置</button></form><details><summary>账号状态详情</summary><pre>{JSON.stringify(account,null,2)}</pre></details><button className="danger" disabled={task.busy} onClick={()=>{if(window.confirm(`删除账号 #${id}？该操作会将账号移入回收状态。`))void task.run(async()=>{await send(`/accounts/${id}`,'DELETE');onChanged()},'账号已删除')}}>删除账号</button></>}<Notice notice={task.notice}/>{output&&<pre className="test-output">{output}</pre>}</>
}
function Scheduler(){
 const data=useData<JsonRecord>('/core-status'),[auto,setAuto]=useState(true)
 useEffect(()=>{if(!auto)return;const timer=setInterval(()=>{if(!document.hidden)void data.refresh()},10_000);return()=>clearInterval(timer)},[auto,data.refresh])
 const scheduler=(data.value?.scheduler??{}) as JsonRecord
 const metrics=[['账号总数',data.value?.total],['当前可用',data.value?.available],['活跃请求',data.value?.active_requests],['等待请求',scheduler.waiters]]
 return <><Header title="调度状态" description="本实例的账号与队列快照；每 10 秒刷新，隐藏页面时暂停。"><label className="check-field"><input type="checkbox" checked={auto} onChange={event=>setAuto(event.target.checked)}/>自动刷新</label><button disabled={data.loading} onClick={()=>void data.refresh()}><RefreshCw size={16}/>刷新</button></Header><LoadingError {...data}/><div className="metrics">{metrics.map(([label,value])=><div className="metric" key={String(label)}><small>{String(label)}</small><strong>{String(value??'—')}</strong></div>)}</div><section className="card"><h2>队列与调度引擎</h2><div className="form-grid">{['engine','max_waiters','max_waiters_per_key','wait_rejected','wait_canceled','wait_timeouts','wait_granted','snapshot_account_count','outbox_backlog'].map(key=><div className="stat" key={key}><small>{key}</small><strong>{String(scheduler[key]??'—')}</strong></div>)}</div></section><details className="card"><summary>完整调度快照</summary><pre>{JSON.stringify(scheduler,null,2)}</pre></details></>
}
function Proxies(){
 const data=useData<{proxies:ProxyRow[]}>('/proxies'),[urls,setUrls]=useState(''),[label,setLabel]=useState('');const task=useTask()
 const add=(event:FormEvent)=>{event.preventDefault();void task.run(async()=>{await send('/proxies','POST',{urls:urls.split('\n').map(value=>value.trim()).filter(Boolean),label});setUrls('');await data.refresh()})}
 return <><Header title="代理出口" description="集中维护代理，账号可绑定独立出口；全局代理池开关在运行设置中。"><button onClick={()=>void data.refresh()} disabled={data.loading}><RefreshCw size={16}/>刷新</button></Header><Notice notice={task.notice}/><form className="card" onSubmit={add}><h2>添加代理</h2><div className="form-grid"><label className="field">代理 URL（每行一个）<textarea required rows={3} value={urls} onChange={event=>setUrls(event.target.value)} placeholder="http://… 或 socks5://…"/></label><label className="field">标签<input value={label} onChange={event=>setLabel(event.target.value)}/></label></div><button className="primary" disabled={task.busy}><Plus size={16}/>添加</button></form><LoadingError {...data}/><div className="table-card"><table><thead><tr><th>代理</th><th>状态</th><th>绑定数</th><th>操作</th></tr></thead><tbody>{data.value?.proxies.map(proxy=><tr key={proxy.id}><td><strong>{proxy.label||`代理 ${proxy.id}`}</strong><small>{proxy.url.replace(/\/\/[^/@]*@/,'//***@')}</small></td><td><Status value={proxy.enabled?proxy.status:'disabled'}/></td><td>{proxy.bound_count??0}</td><td><div className="actions"><button disabled={task.busy} onClick={()=>void task.run(async()=>{const result=await send('/proxies/test','POST',{id:proxy.id,url:proxy.url,lang:'zh'});await data.refresh();if(result.success===false||result.status==='error')throw new Error(String(result.error??result.message??'代理连接测试失败'))},'代理测试已结束')}>测试</button><button disabled={task.busy} onClick={()=>void task.run(async()=>{await send(`/proxies/${proxy.id}`,'PATCH',{enabled:!proxy.enabled});await data.refresh()})}>{proxy.enabled?'禁用':'启用'}</button><button disabled={task.busy} onClick={()=>{if(window.confirm('删除该代理？绑定账号的出口可能受影响。'))void task.run(async()=>{await send(`/proxies/${proxy.id}`,'DELETE');await data.refresh()})}}>删除</button></div></td></tr>)}</tbody></table>{data.value?.proxies.length===0&&<Empty>尚未添加代理。</Empty>}</div></>
}
function Logs(){
 const [limit,setLimit]=useState(100);const data=useData<{logs:JsonRecord[]}>(`/usage/logs?limit=${limit}`)
 return <><Header title="请求日志" description="查看账号执行、Token 用量、首字延迟与上游失败。终端用户计费由 New API 管理。"><select aria-label="日志条数" value={limit} onChange={event=>setLimit(Number(event.target.value))}>{[50,100,200,500].map(value=><option key={value} value={value}>最近 {value} 条</option>)}</select><button disabled={data.loading} onClick={()=>void data.refresh()}><RefreshCw size={16}/>刷新</button></Header><LoadingError {...data}/><div className="table-card"><table><thead><tr><th>时间 / 请求 ID</th><th>账号</th><th>模型</th><th>状态</th><th>首字 / 总耗时</th><th>输入 / 输出 Token</th><th>错误</th></tr></thead><tbody>{data.value?.logs.map((log,index)=><tr key={String(log.id??index)}><td>{formatTime(log.created_at)}<small>{String(log.request_id??'—')}</small></td><td>{String(log.account_name??log.account_id??'—')}</td><td>{String(log.model??'—')}</td><td><Status value={log.status_code}/></td><td>{String(log.first_token_ms??0)} / {String(log.duration_ms??0)} ms</td><td>{String(log.input_tokens??0)} / {String(log.output_tokens??0)}</td><td className="log-error">{String(log.error_message??'—')}</td></tr>)}</tbody></table>{data.value?.logs.length===0&&<Empty>尚无请求日志。</Empty>}</div></>
}
function Keys(){
 const data=useData<{keys:KeyRow[]}>('/keys?view=lite'),[name,setName]=useState('new-api'),[created,setCreated]=useState('');const task=useTask()
 return <><Header title="渠道密钥" description="用于 New API 访问后端。管理密钥和渠道密钥分别使用。"><button disabled={data.loading} onClick={()=>void data.refresh()}><RefreshCw size={16}/>刷新</button></Header><Notice notice={task.notice}/><form className="card inline-form" onSubmit={event=>{event.preventDefault();void task.run(async()=>{const result=await send<{key:string}>('/keys','POST',{name});setCreated(result.key);await data.refresh()},'渠道密钥已创建')}}><label className="field">渠道名称<input required value={name} onChange={event=>setName(event.target.value)}/></label><button className="primary" disabled={task.busy}><Plus size={16}/>创建密钥</button></form>{created&&<section className="card"><h2>新渠道密钥</h2><p>复制到 New API 的渠道密钥配置中。</p><input readOnly aria-label="新渠道密钥" value={created} onFocus={event=>event.target.select()}/><button onClick={()=>setCreated('')}>关闭明文展示</button></section>}<LoadingError {...data}/><div className="table-card"><table><thead><tr><th>名称</th><th>密钥</th><th>状态</th><th>操作</th></tr></thead><tbody>{data.value?.keys.map(key=><tr key={key.id}><td>{key.name}</td><td><code>{key.masked_key??key.key_masked??key.key??'—'}</code></td><td><Status value={key.enabled===false?'disabled':'active'}/></td><td><button disabled={task.busy} onClick={()=>{if(window.confirm(`删除渠道密钥 ${key.name}？对应 New API 渠道将无法访问。`))void task.run(async()=>{await send(`/keys/${key.id}`,'DELETE');await data.refresh()})}}>删除</button></td></tr>)}</tbody></table>{data.value?.keys.length===0&&<Empty>创建一把内部密钥，再配置到 New API 渠道。</Empty>}</div></>
}
function SettingsPage(){
 const data=useData<JsonRecord>('/settings'),[patch,setPatch]=useState<JsonRecord>({}),[advanced,setAdvanced]=useState<string|null>(null);const task=useTask()
 const save=(body:JsonRecord)=>void task.run(async()=>{await send('/settings','PUT',body);setPatch({});setAdvanced(null);await data.refresh()},'运行设置已保存')
 return <><Header title="运行设置" description="只保留账号池、连接、指纹及基础运维配置。"><button disabled={task.busy||!Object.keys(patch).length} className="primary" onClick={()=>save(patch)}><Check size={16}/>保存 {Object.keys(patch).length||''} 项修改</button></Header><Notice notice={task.notice}/><LoadingError {...data}/>{data.value&&<>{settingsGroups.map(group=><section className="card" key={group.name}><h2>{group.name}</h2><p>{group.description}</p><div className="form-grid">{group.fields.filter(field=>field.key in data.value!).map(field=><FieldInput key={field.key} field={field} value={field.key in patch?patch[field.key]:data.value![field.key]} onChange={value=>setPatch(previous=>({...previous,[field.key]:value}))}/>)}</div></section>)}<section className="card"><h2>客户端身份与高级参数</h2><p>使用现有服务端配置字段；User-Agent 配置、重试预算、缓存和连接参数均在此保留。</p><button onClick={()=>setAdvanced(JSON.stringify(writableSettings({...data.value,...patch}),null,2))}>编辑高级 JSON 配置</button><button disabled={task.busy} onClick={()=>void task.run(async()=>{await send('/codex-client-versions/sync');await data.refresh()},'客户端版本同步完成')}>同步客户端版本</button></section></>}{advanced!==null&&<Modal title="高级运行配置" onClose={()=>setAdvanced(null)}><p>仅提交修改过的字段。服务端会拒绝已裁剪功能的设置项。</p><textarea aria-label="高级运行配置 JSON" className="code-input" rows={22} value={advanced} onChange={event=>setAdvanced(event.target.value)} spellCheck={false}/><Notice notice={task.notice}/><button className="primary" disabled={task.busy} onClick={()=>{
 try{const next=jsonObject(advanced),previous=writableSettings(data.value??{}),changes:JsonRecord={};for(const [key,value]of Object.entries(next))if(JSON.stringify(value)!==JSON.stringify(previous[key]))changes[key]=value;if(!Object.keys(changes).length)throw new Error('没有待保存的修改');save(changes)}catch(error){task.setNotice({text:message(error),error:true})}
 }}>保存修改</button></Modal>}</>
}
