import {useCallback, useEffect, useId, useRef, useState, type FormEvent, type ReactNode} from 'react'
import {NavLink, Navigate, Route, Routes, useNavigate} from 'react-router-dom'
import {Activity, ArrowDownToLine, Check, ChevronLeft, ChevronRight, Fingerprint, KeyRound, LogOut, Plus, RefreshCw, Server, Settings, Shield, Users} from 'lucide-react'
import {adminKey, formatTime, jsonObject, request, secretKey, send, writableSettings, type JsonRecord} from './client'
import {accountFields, settingsGroups, type Field} from './settings'
import {accountState, planLabel, stateInfo} from './accountView'
import {AccountResources, QuotaWindow} from './AccountResources'
import {ProxySelector, TimezoneSelector} from './AccountSelectors'
import {Modal} from './Modal'
import {RequestLogs} from './RequestLogs'
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
function Empty({children}:{children:ReactNode}) {return <div className="empty">{children}</div>}
function Status({value}:{value:unknown}) {const info=stateInfo(value);return <span className={`badge ${info.tone}`} title={info.help}>{info.label}</span>}
function FieldInput({field,value,onChange}:{field:Field;value:unknown;onChange:(value:unknown)=>void}) {
 const id=useId(),choices=field.choices??[],selected=choices.find(choice=>choice.value===String(value??''))
 const displayed=field.scale&&value!==null&&value!==undefined&&value!==''?Number((Number(value)*field.scale).toFixed(8)):value
 return <div className="field"><label htmlFor={id}>{field.label}</label>{field.type==='boolean'?<div className="field-switch"><input id={id} aria-describedby={`${id}-help`} type="checkbox" checked={Boolean(value)} onChange={event=>onChange(event.target.checked)}/><span>{value?'已开启':'已关闭'}</span></div>:field.type==='select'?<select id={id} aria-describedby={`${id}-help`} value={String(value??'')} onChange={event=>onChange(event.target.value)}>{!selected&&<option value={String(value??'')}>{String(value??'继承默认')}</option>}{choices.map(choice=><option value={choice.value} key={choice.value}>{choice.label} ({choice.value})</option>)}</select>:<input id={id} aria-describedby={`${id}-help`} type={field.type} value={String(displayed??'')} min={field.min} max={field.max} onChange={event=>onChange(field.type==='number'?(event.target.value===''?'':Number(event.target.value)/(field.scale??1)):event.target.value)} step={field.type==='number'?'any':undefined}/>}<small id={`${id}-help`}>{field.help}</small>{selected&&<small className="choice-help">当前选择：{selected.help}</small>}{choices.length>0&&<details className="choice-guide"><summary>查看全部选项说明</summary><dl>{choices.map(choice=><div key={choice.value}><dt>{choice.label}</dt><dd>{choice.help}</dd></div>)}</dl></details>}</div>
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
 return <div className="shell"><aside className="sidebar"><div className="brand"><div className="brand-icon"><Server size={22}/></div><div><strong>Codex 账号服务</strong><small>New API 的上游账号池</small></div></div><nav aria-label="管理导航">{navigation.map(({path,label,icon:Icon})=><NavLink key={path} to={path}><Icon size={18}/>{label}</NavLink>)}</nav><div className="sidebar-footer"><span className="badge good">Codex only</span><small>账号身份 · 连接执行 · 调度</small><button onClick={()=>{sessionStorage.removeItem(secretKey);setSignedIn(false);navigate('/accounts')}}><LogOut size={16}/>退出登录</button></div></aside><main className="content"><Routes><Route path="/accounts" element={<Accounts/>}/><Route path="/scheduler" element={<Scheduler/>}/><Route path="/proxies" element={<Proxies/>}/><Route path="/logs" element={<RequestLogs/>}/><Route path="/keys" element={<Keys/>}/><Route path="/settings" element={<SettingsPage/>}/><Route path="*" element={<Navigate to="/accounts" replace/>}/></Routes></main></div>
}
function Accounts(){
 const [page,setPage]=useState(1),[search,setSearch]=useState(''),[query,setQuery]=useState(''),[selected,setSelected]=useState<number[]>([])
 const data=useData<AccountPage>(`/accounts?view=page&page=${page}&page_size=50&channel=codex&search=${encodeURIComponent(query)}`)
 const [adding,setAdding]=useState(false),[editing,setEditing]=useState<number|null>(null);const task=useTask()
 const accounts=data.value?.accounts??[];const total=data.value?.total??data.value?.total_matched??accounts.length
 const bulk=(path:string,body:JsonRecord={})=>void task.run(async()=>{await send(`/accounts/${path}`,'POST',{ids:selected,...body});setSelected([]);await data.refresh()})
 return <><Header title="账号池" description="管理原生 Codex 凭据、配额、指纹和账号调度。"><button onClick={()=>void data.refresh()} disabled={data.loading}><RefreshCw size={16}/>刷新</button><button className="primary" onClick={()=>setAdding(true)}><Plus size={16}/>导入账号</button></Header><Notice notice={task.notice}/><div className="toolbar"><form onSubmit={event=>{event.preventDefault();setPage(1);setSelected([]);setQuery(search)}}><input aria-label="搜索账号" placeholder="搜索名称或邮箱" value={search} onChange={event=>setSearch(event.target.value)}/><button>搜索</button></form><span className="muted">共 {total} 个账号</span></div>{selected.length>0&&<div className="bulk-bar"><strong>已选 {selected.length} 个</strong><button disabled={task.busy} onClick={()=>bulk('batch-refresh')}>刷新凭据</button><button disabled={task.busy} onClick={()=>bulk('batch-refresh-usage')}>刷新配额</button><button disabled={task.busy} onClick={()=>bulk('batch-reset-status')}>重置状态</button><button disabled={task.busy} onClick={()=>bulk('batch-update',{enabled:false})}>禁用</button></div>}<LoadingError {...data}/><div className="table-card"><table><thead><tr><th><input type="checkbox" aria-label="选择本页账号" checked={accounts.length>0&&accounts.every(account=>selected.includes(account.id))} onChange={event=>setSelected(event.target.checked?accounts.map(account=>account.id):[])}/></th><th>账号</th><th>状态</th><th>套餐</th><th>并发 / 上限</th><th>上游额度（已用）</th><th>时间 / 重置卡</th><th>调度</th><th/></tr></thead><tbody>{accounts.map(account=><tr key={account.id}><td><input type="checkbox" aria-label={`选择账号 ${account.id}`} checked={selected.includes(account.id)} onChange={event=>setSelected(previous=>event.target.checked?[...previous,account.id]:previous.filter(id=>id!==account.id))}/></td><td><strong>{account.name||account.email||`账号 ${account.id}`}</strong><small>{account.email||`#${account.id}`}</small></td><td><Status value={!account.enabled?'disabled':account.locked?'locked':account.status}/><small>{accountState(account).help}</small>{account.using_credits&&<span className="badge neutral">使用积分</span>}{account.error_message&&<small className="error-text" title={account.error_message}>{account.error_message.slice(0,90)}</small>}</td><td>{planLabel(account.plan_type)}</td><td>{account.active_requests??0} / {account.dynamic_concurrency_limit??'—'}</td><td><QuotaWindow account={account} window="5h" compact/><QuotaWindow account={account} window="7d" compact/></td><td><small>订阅到期：{formatTime(account.subscription?.expires_at||account.subscription_expires_at)}</small><small>凭据刷新：{formatTime(account.codex_last_refresh_at)}</small><small>额度刷新：{formatTime(account.codex_usage_updated_at)}</small><small>重置卡：{account.rate_limit_reset_credits??'未获取'}</small><button className="text-button" onClick={()=>setEditing(account.id)}>查看额度与有效期</button></td><td><small title={stateInfo(account.health_tier).help}>{stateInfo(account.health_tier).label}</small>{account.scheduler_score?.toFixed(1)??'—'}</td><td><button onClick={()=>setEditing(account.id)}>管理</button></td></tr>)}</tbody></table>{!data.loading&&!accounts.length&&<Empty>尚无匹配账号。导入 Refresh Token 或 Access Token 后开始管理。</Empty>}</div><div className="pagination"><button disabled={page<=1||data.loading} onClick={()=>{setSelected([]);setPage(page-1)}}><ChevronLeft size={16}/>上一页</button><span>第 {page} 页</span><button disabled={page*50>=total||data.loading} onClick={()=>{setSelected([]);setPage(page+1)}}>下一页<ChevronRight size={16}/></button></div>{adding&&<Modal title="导入 Codex 账号" onClose={()=>setAdding(false)}><ImportAccounts onDone={()=>{setAdding(false);void data.refresh()}}/></Modal>}{editing!==null&&<Modal title={`账号 #${editing}`} onClose={()=>setEditing(null)}><AccountEditor id={editing} onChanged={()=>void data.refresh()} onDeleted={()=>setEditing(null)}/></Modal>}</>
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
 })}}><label className="field">凭据类型<select value={kind} onChange={event=>setKind(event.target.value)}><option value="refresh">Refresh Token（每行一个）</option><option value="access">Access Token（每行一个）</option><option value="file">Codex JSON 凭据文件</option></select></label>{kind==='file'?<label className="field">JSON 文件<input type="file" accept=".json,application/json" required onChange={event=>setFile(event.target.files?.[0]??null)}/></label>:<label className="field">凭据<textarea required rows={7} spellCheck={false} autoComplete="off" value={credential} onChange={event=>setCredential(event.target.value)} placeholder="只在导入时提交，日志不会记录 Token。"/></label>}<div className="form-grid"><label className="field">名称（可选）<input value={name} onChange={event=>setName(event.target.value)}/></label><ProxySelector label="代理 URL（可选）" value={proxy} onChange={setProxy}/></div><label className="check-field"><input type="checkbox" checked={skip} onChange={event=>setSkip(event.target.checked)}/>导入时跳过立即刷新，交给后台刷新任务</label><Notice notice={task.notice}/><button className="primary" disabled={task.busy}>{task.busy?'导入中…':'导入'}</button></form>
}
function AccountEditor({id,onChanged,onDeleted}:{id:number;onChanged:()=>void;onDeleted:()=>void}){
 const data=useData<Account>(`/accounts/${id}`),[patch,setPatch]=useState<JsonRecord>({}),[headers,setHeaders]=useState(''),[headersEdited,setHeadersEdited]=useState(false),[output,setOutput]=useState(''),[tab,setTab]=useState<'overview'|'config'>('overview');const task=useTask()
 const account=data.value
 const reload=async()=>{await data.refresh();onChanged()}
 const act=(path:string,body?:unknown)=>void task.run(async()=>{await send(`/accounts/${id}/${path}`,'POST',body);await reload()})
 const test=()=>void task.run(async()=>{
  const response=await fetch(`/api/admin/accounts/${id}/test`,{headers:{'X-Admin-Key':adminKey()},signal:AbortSignal.timeout(180_000)})
  const text=await response.text();setOutput(text.slice(-12000));if(!response.ok)throw new Error(`连接测试失败 (${response.status})`)
 },'测试已结束；请查看输出中的成功或失败事件')
 return <><LoadingError {...data}/>{account&&<>
  <div className="account-summary"><strong>{account.email||account.name}</strong><Status value={!account.enabled?'disabled':account.locked?'locked':account.status}/><span>{planLabel(account.plan_type)}</span></div>
  <div className="actions wrap"><button disabled={task.busy||account.at_only} title="使用刷新凭据更新 Access Token；不会重置上游额度" onClick={()=>act('refresh')}>刷新凭据</button><button disabled={task.busy} title="向上游执行一次测试请求，可能消耗额度" onClick={test}>测试连接</button><button disabled={task.busy} onClick={()=>act('enable',{enabled:!account.enabled})}>{account.enabled?'禁用':'启用'}</button><button disabled={task.busy} onClick={()=>act('lock',{locked:!account.locked})}>{account.locked?'解锁':'锁定'}</button><button disabled={task.busy} title="清理本地错误与冷却，不恢复上游额度" onClick={()=>act('reset-status')}>重置本地状态</button></div>
  <Notice notice={task.notice}/>
  <div className="account-tabs" role="tablist" aria-label="账号详情"><button id="account-overview-tab" type="button" role="tab" aria-selected={tab==='overview'} aria-controls="account-overview" onClick={()=>setTab('overview')}>额度与有效期</button><button id="account-config-tab" type="button" role="tab" aria-selected={tab==='config'} aria-controls="account-config" onClick={()=>setTab('config')}>账号配置</button></div>
  {tab==='overview'?<div id="account-overview" role="tabpanel" aria-labelledby="account-overview-tab"><AccountResources account={account} busy={task.busy} run={task.run} onChanged={reload}/></div>:<div id="account-config" role="tabpanel" aria-labelledby="account-config-tab"><p className="section-help">这里只修改当前账号；未编辑的配置不会被提交。</p><form onSubmit={event=>{event.preventDefault();void task.run(async()=>{
   const body={...patch}
   for(const field of accountFields)if(field.type==='number'&&body[field.key]==='')body[field.key]=field.key.endsWith('override')||field.key==='scheduler_priority'||field.key.startsWith('auto_pause_')?null:0
   if(headersEdited){const custom=jsonObject(headers);if(Object.values(custom).some(value=>typeof value!=='string'))throw new Error('自定义请求头的值必须是字符串');body.custom_headers=custom}
   if(!Object.keys(body).length)throw new Error('没有待保存的修改')
   await send(`/accounts/${id}/scheduler`,'PATCH',body);setPatch({});setHeadersEdited(false);await reload()
  },'账号配置已保存')}}><div className="form-grid">{accountFields.map(field=>{
   const value=field.key in patch?patch[field.key]:account[field.key]
   const change=(value:unknown)=>setPatch(previous=>({...previous,[field.key]:value}))
   if(field.key==='proxy_url')return <ProxySelector key={field.key} value={String(value??'')} onChange={change}/>
   if(field.key==='timezone')return <TimezoneSelector key={field.key} value={String(value??'')} onChange={change}/>
   return <FieldInput key={field.key} field={field} value={value} onChange={change}/>
  })}</div>
  <label className="field"><span><Fingerprint size={16}/>账号自定义请求头（JSON）</span><textarea rows={5} spellCheck={false} value={headersEdited?headers:JSON.stringify(account.custom_headers??{},null,2)} onChange={event=>{setHeadersEdited(true);setHeaders(event.target.value)}}/><small>按 JSON 对象填写，值必须是字符串。可覆盖 User-Agent、Originator、Version 等出站身份；清空为 {'{}'} 会移除账号覆盖。</small></label><button className="primary" disabled={task.busy}><Check size={16}/>保存配置</button></form></div>}
  {output&&<details open><summary>连接测试输出</summary><pre className="test-output">{output}</pre></details>}
  <details><summary>原始状态（排查用）</summary><pre>{JSON.stringify(account,null,2)}</pre></details>
  <button className="danger" disabled={task.busy} onClick={()=>{if(window.confirm(`删除账号 #${id}？该操作会将账号移入回收状态。`))void task.run(async()=>{await send(`/accounts/${id}`,'DELETE');onChanged();onDeleted()},'账号已删除')}}>删除账号</button>
 </>}</>
}
function Scheduler(){
 const data=useData<JsonRecord>('/core-status'),[auto,setAuto]=useState(true)
 useEffect(()=>{if(!auto)return;const timer=setInterval(()=>{if(!document.hidden)void data.refresh()},10_000);return()=>clearInterval(timer)},[auto,data.refresh])
 const scheduler=(data.value?.scheduler??{}) as JsonRecord
 const descriptions:Record<string,{label:string;help:string}>={engine:{label:'当前调度引擎',help:'传统扫描、影子对照或索引选号。'},max_waiters:{label:'总等待上限',help:'本实例允许排队的请求数。'},max_waiters_per_key:{label:'单渠道等待上限',help:'每把渠道密钥允许排队的请求数。'},wait_rejected:{label:'拒绝排队次数',help:'等待容量不足时拒绝的累计次数。'},wait_canceled:{label:'取消等待次数',help:'客户端取消或请求结束导致的累计取消次数。'},wait_timeouts:{label:'等待超时次数',help:'排队超时的累计次数。'},wait_granted:{label:'排队后获准次数',help:'等待后成功获得账号的累计次数。'},snapshot_account_count:{label:'调度快照账号数',help:'当前调度快照包含的账号数量。'},outbox_backlog:{label:'待同步账号事件',help:'数据库事件尚未应用到本实例索引的数量。'}}
 const metrics=[['账号总数',data.value?.total],['当前可用',data.value?.available],['活跃请求',data.value?.active_requests],['等待请求',scheduler.waiters]]
 return <><Header title="调度状态" description="本实例的账号与队列快照；每 10 秒刷新，隐藏页面时暂停。"><label className="check-field"><input type="checkbox" checked={auto} onChange={event=>setAuto(event.target.checked)}/>自动刷新</label><button disabled={data.loading} onClick={()=>void data.refresh()}><RefreshCw size={16}/>刷新</button></Header><LoadingError {...data}/><div className="metrics">{metrics.map(([label,value])=><div className="metric" key={String(label)}><small>{String(label)}</small><strong>{String(value??'—')}</strong></div>)}</div><section className="card"><h2>队列与调度引擎</h2><div className="form-grid">{['engine','max_waiters','max_waiters_per_key','wait_rejected','wait_canceled','wait_timeouts','wait_granted','snapshot_account_count','outbox_backlog'].map(key=><div className="stat" key={key}><small title={descriptions[key]?.help}>{descriptions[key]?.label??key}</small><strong>{key==='engine'?({legacy:'传统扫描',shadow:'影子对照',indexed:'索引调度'} as Record<string,string>)[String(scheduler[key])]??String(scheduler[key]??'—'):String(scheduler[key]??'—')}</strong></div>)}</div></section><details className="card"><summary>完整调度快照（排查用）</summary><pre>{JSON.stringify(scheduler,null,2)}</pre></details></>
}
function Proxies(){
 const data=useData<{proxies:ProxyRow[]}>('/proxies'),[urls,setUrls]=useState(''),[label,setLabel]=useState('');const task=useTask()
 const add=(event:FormEvent)=>{event.preventDefault();void task.run(async()=>{await send('/proxies','POST',{urls:urls.split('\n').map(value=>value.trim()).filter(Boolean),label});setUrls('');await data.refresh()})}
 return <><Header title="代理出口" description="集中维护代理，账号可绑定独立出口；全局代理池开关在运行设置中。"><button onClick={()=>void data.refresh()} disabled={data.loading}><RefreshCw size={16}/>刷新</button></Header><Notice notice={task.notice}/><form className="card" onSubmit={add}><h2>添加代理</h2><div className="form-grid"><label className="field">代理 URL（每行一个）<textarea required rows={3} value={urls} onChange={event=>setUrls(event.target.value)} placeholder="http://… 或 socks5://…"/></label><label className="field">标签<input value={label} onChange={event=>setLabel(event.target.value)}/></label></div><button className="primary" disabled={task.busy}><Plus size={16}/>添加</button></form><LoadingError {...data}/><div className="table-card"><table><thead><tr><th>代理</th><th>状态</th><th>绑定数</th><th>操作</th></tr></thead><tbody>{data.value?.proxies.map(proxy=><tr key={proxy.id}><td><strong>{proxy.label||`代理 ${proxy.id}`}</strong><small>{proxy.url.replace(/\/\/[^/@]*@/,'//***@')}</small></td><td><Status value={proxy.enabled?(proxy.test_status??proxy.status??'untested'):'disabled'}/></td><td>{proxy.bound_count??0}</td><td><div className="actions"><button disabled={task.busy} onClick={()=>void task.run(async()=>{const result=await send('/proxies/test','POST',{id:proxy.id,url:proxy.url,lang:'zh'});await data.refresh();if(result.success===false||result.status==='error')throw new Error(String(result.error??result.message??'代理连接测试失败'))},'代理测试已结束')}>测试</button><button disabled={task.busy} onClick={()=>void task.run(async()=>{await send(`/proxies/${proxy.id}`,'PATCH',{enabled:!proxy.enabled});await data.refresh()})}>{proxy.enabled?'禁用':'启用'}</button><button disabled={task.busy} onClick={()=>{if(window.confirm('删除该代理？绑定账号的出口可能受影响。'))void task.run(async()=>{await send(`/proxies/${proxy.id}`,'DELETE');await data.refresh()})}}>删除</button></div></td></tr>)}</tbody></table>{data.value?.proxies.length===0&&<Empty>尚未添加代理。</Empty>}</div></>
}
function Keys(){
 const data=useData<{keys:KeyRow[]}>('/keys?view=lite'),[name,setName]=useState('new-api'),[created,setCreated]=useState('');const task=useTask()
 return <><Header title="渠道密钥" description="用于 New API 访问后端。管理密钥和渠道密钥分别使用。"><button disabled={data.loading} onClick={()=>void data.refresh()}><RefreshCw size={16}/>刷新</button></Header><Notice notice={task.notice}/><form className="card inline-form" onSubmit={event=>{event.preventDefault();void task.run(async()=>{const result=await send<{key:string}>('/keys','POST',{name});setCreated(result.key);await data.refresh()},'渠道密钥已创建')}}><label className="field">渠道名称<input required value={name} onChange={event=>setName(event.target.value)}/></label><button className="primary" disabled={task.busy}><Plus size={16}/>创建密钥</button></form>{created&&<section className="card"><h2>新渠道密钥</h2><p>复制到 New API 的渠道密钥配置中。</p><input readOnly aria-label="新渠道密钥" value={created} onFocus={event=>event.target.select()}/><button onClick={()=>setCreated('')}>关闭明文展示</button></section>}<LoadingError {...data}/><div className="table-card"><table><thead><tr><th>名称</th><th>密钥</th><th>状态</th><th>操作</th></tr></thead><tbody>{data.value?.keys.map(key=><tr key={key.id}><td>{key.name}</td><td><code>{key.masked_key??key.key_masked??key.key??'—'}</code></td><td><Status value={key.enabled===false?'disabled':'active'}/></td><td><button disabled={task.busy} onClick={()=>{if(window.confirm(`删除渠道密钥 ${key.name}？对应 New API 渠道将无法访问。`))void task.run(async()=>{await send(`/keys/${key.id}`,'DELETE');await data.refresh()})}}>删除</button></td></tr>)}</tbody></table>{data.value?.keys.length===0&&<Empty>创建一把内部密钥，再配置到 New API 渠道。</Empty>}</div></>
}
function SettingsPage(){
 const data=useData<JsonRecord>('/settings'),[patch,setPatch]=useState<JsonRecord>({}),[advanced,setAdvanced]=useState<string|null>(null);const task=useTask()
 const save=(body:JsonRecord)=>void task.run(async()=>{await send('/settings','PUT',body);setPatch({});setAdvanced(null);await data.refresh()},'运行设置已保存')
 return <><Header title="运行设置" description="只保留账号池、连接、指纹及基础运维配置。"><button disabled={task.busy||!Object.keys(patch).length} className="primary" onClick={()=>save(patch)}><Check size={16}/>保存 {Object.keys(patch).length||''} 项修改</button></Header><Notice notice={task.notice}/><LoadingError {...data}/>{data.value&&<>{settingsGroups.map(group=><section className="card" key={group.name}><h2>{group.name}</h2><p>{group.description}</p><div className="form-grid">{group.fields.filter(field=>field.key in data.value!).map(field=><FieldInput key={field.key} field={field} value={field.key in patch?patch[field.key]:data.value![field.key]} onChange={value=>setPatch(previous=>({...previous,[field.key]:value}))}/>)}</div></section>)}<section className="card"><h2>客户端身份与高级参数</h2><p>普通表单已经提供中文说明。高级 JSON 用于补充 User-Agent、重试预算和缓存等参数；其中暂停阈值使用 0～1 的比例，例如 0.95 表示 95%。</p><button onClick={()=>setAdvanced(JSON.stringify(writableSettings({...data.value,...patch}),null,2))}>编辑高级 JSON 配置</button><button disabled={task.busy} onClick={()=>void task.run(async()=>{await send('/codex-client-versions/sync');await data.refresh()},'客户端版本同步完成')}>同步客户端版本</button></section></>}{advanced!==null&&<Modal title="高级运行配置" onClose={()=>setAdvanced(null)}><p>仅提交修改过的字段。服务端会拒绝已裁剪功能的设置项。</p><textarea aria-label="高级运行配置 JSON" className="code-input" rows={22} value={advanced} onChange={event=>setAdvanced(event.target.value)} spellCheck={false}/><Notice notice={task.notice}/><button className="primary" disabled={task.busy} onClick={()=>{
 try{const next=jsonObject(advanced),previous=writableSettings(data.value??{}),changes:JsonRecord={};for(const [key,value]of Object.entries(next))if(JSON.stringify(value)!==JSON.stringify(previous[key]))changes[key]=value;if(!Object.keys(changes).length)throw new Error('没有待保存的修改');save(changes)}catch(error){task.setNotice({text:message(error),error:true})}
 }}>保存修改</button></Modal>}</>
}
