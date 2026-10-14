import {useCallback, useEffect, useId, useRef, useState} from 'react'
import {request} from './client'
import {stateInfo} from './accountView'
import type {ProxyRow} from './types'

const customValue='__custom__'
function maskedProxy(url: string) {return url.replace(/\/\/[^/@]*@/,'//***@')}
export function ProxySelector({value,onChange,label='独立代理 URL'}:{value:string;onChange:(value:string)=>void;label?:string}) {
 const id=useId(),[proxies,setProxies]=useState<ProxyRow[]>([]),[loading,setLoading]=useState(true),[error,setError]=useState(''),[manual,setManual]=useState(false)
 const generation=useRef(0)
 const reload=useCallback(async()=>{
  const current=++generation.current;setLoading(true);setError('')
  try{const data=await request<{proxies:ProxyRow[]}>('/proxies');if(current===generation.current)setProxies(data.proxies??[])}
  catch(error){if(current===generation.current)setError(error instanceof Error?error.message:String(error))}
  finally{if(current===generation.current)setLoading(false)}
 },[])
 useEffect(()=>{void reload();return()=>{generation.current++}},[reload])
 const unique=[...new Map(proxies.map(proxy=>[proxy.url,proxy])).values()]
 const listed=unique.some(proxy=>proxy.url===value),custom=manual||(value!==''&&!listed)
 return <div className="field"><label htmlFor={id}>{label}</label><select id={id} aria-describedby={`${id}-help`} value={custom?customValue:value} onChange={event=>{
  if(event.target.value===customValue){setManual(true);return}
  setManual(false);onChange(event.target.value)
 }}><option value="">继承默认出口（不单独绑定）</option>{unique.map(proxy=><option key={proxy.id} value={proxy.url} disabled={!proxy.enabled&&proxy.url!==value}>{proxy.label||`代理 ${proxy.id}`} · {maskedProxy(proxy.url)}{!proxy.enabled?'（已禁用）':proxy.test_status==='error'?'（测试异常）':''}</option>)}<option value={customValue}>手动填写 / 保留自定义代理</option></select>{custom&&<label className="custom-entry">手动填写独立代理 URL<input value={value} onChange={event=>onChange(event.target.value)} autoComplete="off" spellCheck={false} placeholder="http://主机:端口 或 socks5://主机:端口"/></label>}<small id={`${id}-help`}>从“代理出口”中选择已配置的代理，或手动填写。选择继承默认出口会清空账号独立绑定，按分组、代理池和全局规则选择。</small>{loading&&<small role="status">正在加载代理列表…</small>}{error&&<div className="selector-notice" role="alert"><span>代理列表加载失败：{error}。当前绑定仍保留，可手动填写。</span><button type="button" disabled={loading} onClick={()=>void reload()}>重新加载代理</button></div>}{!loading&&!error&&!unique.length&&<small>暂无已配置代理，可先到“代理出口”添加，也可以手动填写。</small>}{listed&&!custom&&<small>当前代理：{unique.find(proxy=>proxy.url===value)?.enabled?'已启用':'已禁用'}；测试状态：{stateInfo(unique.find(proxy=>proxy.url===value)?.test_status??'untested').label}</small>}</div>
}
const commonZones: [string,string][]=[
 ['Asia/Shanghai','中国大陆 · 上海'],['Asia/Hong_Kong','中国香港'],['Asia/Taipei','中国台湾 · 台北'],['Asia/Tokyo','日本 · 东京'],['Asia/Seoul','韩国 · 首尔'],['Asia/Singapore','新加坡'],['Asia/Kolkata','印度 · 加尔各答'],['Asia/Dubai','阿联酋 · 迪拜'],['UTC','协调世界时'],['Europe/London','英国 · 伦敦'],['Europe/Berlin','德国 · 柏林'],['Europe/Paris','法国 · 巴黎'],['America/New_York','美国东部 · 纽约'],['America/Chicago','美国中部 · 芝加哥'],['America/Denver','美国山地 · 丹佛'],['America/Los_Angeles','美国西部 · 洛杉矶'],['Australia/Sydney','澳大利亚 · 悉尼'],['Pacific/Auckland','新西兰 · 奥克兰'],
]
function supportedZones() {
 try{return (Intl as typeof Intl & {supportedValuesOf?: (key:string)=>string[]}).supportedValuesOf?.('timeZone')??[]}
 catch{return []}
}
const allZones=supportedZones().filter(zone=>!commonZones.some(([value])=>value===zone))
export function TimezoneSelector({value,onChange}:{value:string;onChange:(value:string)=>void}) {
 const id=useId(),[manual,setManual]=useState(false)
 const listed=commonZones.some(([zone])=>zone===value)||allZones.includes(value),custom=manual||(value!==''&&!listed)
 return <div className="field"><label htmlFor={id}>账号时区</label><select id={id} aria-describedby={`${id}-help`} value={custom?customValue:value} onChange={event=>{
  if(event.target.value===customValue){setManual(true);return}
  setManual(false);onChange(event.target.value)
 }}><option value="">保留下游时区（不单独设置）</option><optgroup label="常用时区">{commonZones.map(([zone,label])=><option key={zone} value={zone}>{label} ({zone})</option>)}</optgroup>{allZones.length>0&&<optgroup label="其他时区">{allZones.map(zone=><option key={zone} value={zone}>{zone}</option>)}</optgroup>}<option value={customValue}>手动填写 / 保留自定义时区</option></select>{custom&&<label className="custom-entry">手动填写账号时区<input value={value} onChange={event=>onChange(event.target.value)} autoComplete="off" spellCheck={false} placeholder="例如 Asia/Shanghai"/></label>}<small id={`${id}-help`}>用于改写上游请求中的时区与日期，不改变后台页面的时间显示。选择“保留下游时区”会清空账号覆盖。时区使用 IANA 名称，夏令时由时区规则处理。</small></div>
}
