export type LogFilters = {
 range:string; start:string; end:string; q:string; status:string; model:string;
 account_id:string; api_key_id:string; endpoint:string; stream:string; retry:string; via_websocket:string; error_kind:string
}
export const defaultLogFilters:LogFilters = {
 range:'24', start:'', end:'', q:'', status:'all', model:'', account_id:'', api_key_id:'', endpoint:'', stream:'', retry:'', via_websocket:'', error_kind:''
}
export function logTimeRange(filters:LogFilters, now=Date.now()) {
 const hours=Number(filters.range)
 const start=filters.range==='custom'?new Date(filters.start).getTime():now-hours*3600000
 const end=filters.range==='custom'?new Date(filters.end).getTime():now
 if(!Number.isFinite(start)||!Number.isFinite(end)||start>=end)throw new Error('请填写有效的起止时间，结束时间须晚于开始时间。')
 return {start:new Date(start).toISOString(),end:new Date(end).toISOString()}
}
export function logQuery(filters:LogFilters, range:{start:string;end:string}, page:number, pageSize:number) {
 const query=new URLSearchParams({...range,page:String(page),page_size:String(pageSize),include_canceled:'true'})
 if(filters.status==='error')query.set('error_only','true')
 else if(filters.status!=='all')query.set('status',filters.status)
 for(const key of ['q','model','account_id','api_key_id','endpoint','stream','retry','via_websocket','error_kind'] as const){
  const value=filters[key].trim()
  if((key==='account_id'||key==='api_key_id')&&value&&!/^[1-9]\d*$/.test(value))throw new Error('账号 ID 和渠道密钥 ID 必须为正整数。')
  if(value)query.set(key,value)
 }
 return query.toString()
}
export function logStatus(value:unknown) {
 const code=Number(value)
 if(code===499)return {label:'499 · 客户端取消',tone:'neutral',help:'客户端取消请求或连接断开。'}
 if(code===598)return {label:'598 · 流执行失败',tone:'bad',help:'服务已建立响应连接，但流执行未成功完成；查看错误详情定位原因。'}
 if(code>=200&&code<300)return {label:`${code} · 成功`,tone:'good',help:'请求执行成功。'}
 if(code>=400)return {label:`${code} · 失败`,tone:'bad',help:'请求执行失败；查看错误详情定位原因。'}
 return {label:code>0?String(code):'未知',tone:'neutral',help:'未记录成功或失败状态。'}
}
export function logValue(value:unknown, suffix='') {
 return value===null||value===undefined||value===''?'—':`${String(value)}${suffix}`
}
