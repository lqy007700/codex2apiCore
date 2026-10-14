import test from 'node:test'
import assert from 'node:assert/strict'
import {jsonObject,writableSettings,request,ApiError,secretKey} from '../src/core/client.ts'

test('advanced settings accept objects and reject other JSON values',()=>{
 assert.deepEqual(jsonObject('{"max_concurrency":3}'),{max_concurrency:3})
 for(const value of ['null','[]','1','"hello"'])assert.throws(()=>jsonObject(value),/JSON 对象/)
})
test('read-only settings are excluded without mutating the snapshot',()=>{
 const snapshot={max_concurrency:2,response_cache_config_generation:4,codex_client_versions:{version:'x'},codex_egress:{mode:'direct'},codex_synced_cli_version:'0.153.4',codex_synced_desktop_mac_build:'123'}
 assert.deepEqual(writableSettings(snapshot),{max_concurrency:2})
 assert.equal(snapshot.response_cache_config_generation,4)
})
test('internal admin authentication and error handling',async()=>{
 const originalFetch=globalThis.fetch,originalStorage=globalThis.sessionStorage,originalWindow=globalThis.window
 let expired=false
 globalThis.sessionStorage={getItem:key=>key===secretKey?'test-admin':null}
 globalThis.window={dispatchEvent:()=>{expired=true}}
 try{
  globalThis.fetch=async(url,options)=>{assert.equal(url,'/api/admin/settings');assert.equal(options.headers.get('X-Admin-Key'),'test-admin');assert.equal(options.headers.get('Content-Type'),'application/json');return new Response('{"ok":true}')}
  assert.deepEqual(await request('/settings',{method:'PUT',body:'{"max_concurrency":3}'}),{ok:true})
  globalThis.fetch=async()=>new Response('{"error":"expired"}',{status:401})
  await assert.rejects(request('/settings'),error=>error instanceof ApiError&&error.status===401&&error.message==='expired')
  assert.equal(expired,true)
 }finally{globalThis.fetch=originalFetch;globalThis.sessionStorage=originalStorage;globalThis.window=originalWindow}
})

test('SSE batch imports require a terminal event across arbitrary chunk boundaries',async()=>{
 const {readEventResult}=await import('../src/core/client.ts')
 const payload='data: {"type":"progress","current":1}\n\ndata: {"type":"complete","success":2,"failed":0}\n\n'
 const chunks=new ReadableStream({start(controller){for(let i=0;i<payload.length;i+=7)controller.enqueue(new TextEncoder().encode(payload.slice(i,i+7)));controller.close()}})
 assert.deepEqual(await readEventResult(new Response(chunks)),{type:'complete',success:2,failed:0})
 await assert.rejects(readEventResult(new Response('data: {"type":"progress"}\n\n')),/提前结束/)
})

test('quota displays backend fields without inventing zero for unknown values',async()=>{
 const {usagePercent,longWindowLabel,finiteNumber,accountState,stateInfo}=await import('../src/core/accountView.ts')
 assert.equal(usagePercent({usage_percent_5h:12},'5h'),12)
 assert.equal(usagePercent({usage_percent_5h:null},'5h'),null)
 for(const value of ['',false,'invalid',Infinity])assert.equal(finiteNumber(value),null)
 assert.equal(usagePercent({usage_percent_5h:0},'5h'),0)
 assert.equal(longWindowLabel({usage_window_7d_kind:'monthly'}),'月周期')
 assert.equal(accountState({enabled:false,status:'active'}).label,'已禁用')
 assert.equal(accountState({enabled:true,locked:true,status:'active'}).label,'已锁定')
 assert.equal(stateInfo('usage_exhausted').label,'额度耗尽')
 assert.equal(stateInfo('unauthorized').label,'鉴权失败')
})
