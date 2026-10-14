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

test('log queries always use server pagination, anchored dates and explicit diagnostic filters',async()=>{
 const {defaultLogFilters,logTimeRange,logQuery,logValue,logStatus}=await import('../src/core/logView.ts')
 const range=logTimeRange(defaultLogFilters,Date.parse('2026-10-14T12:00:00Z'))
 assert.deepEqual(range,{start:'2026-10-13T12:00:00.000Z',end:'2026-10-14T12:00:00.000Z'})
 const query=new URLSearchParams(logQuery({...defaultLogFilters,status:'error',q:' req & timeout ',account_id:'12',retry:'false',via_websocket:'true'},range,3,50))
 assert.equal(query.get('page'),'3');assert.equal(query.get('page_size'),'50');assert.equal(query.get('start'),range.start)
 assert.equal(query.get('q'),'req & timeout');assert.equal(query.get('error_only'),'true');assert.equal(query.get('include_canceled'),'true')
 assert.equal(query.get('retry'),'false');assert.equal(query.get('account_id'),'12');assert.equal(query.get('status'),null)
 assert.equal(new URLSearchParams(logQuery({...defaultLogFilters,status:'598'},range,1,20)).get('status'),'598')
 for(const account_id of ['0','-1','1.5','NaN'])assert.throws(()=>logQuery({...defaultLogFilters,account_id},range,1,20),/正整数/)
 assert.throws(()=>logTimeRange({...defaultLogFilters,range:'custom',start:'',end:''}),/有效的起止时间/)
 assert.throws(()=>logTimeRange({...defaultLogFilters,range:'custom',start:'2026-10-14T12:00',end:'2026-10-13T12:00'}),/结束时间/)
 assert.equal(logValue(null),'—');assert.equal(logValue(0,' ms'),'0 ms');assert.equal(logStatus(499).tone,'neutral');assert.equal(logStatus(598).tone,'bad')
})

test('request chain counts actual recorded attempts, not planned retries or internal rounds',async()=>{
 const {summarizeChain,chainTransition}=await import('../src/core/logView.ts')
 const initial={id:1,account_id:10,attempt_index:1,is_retry_attempt:true,status_code:429}
 assert.deepEqual(summarizeChain([initial]),{attempts:1,retries:0,switches:0,highest:1,gaps:false,ambiguous:false,unindexed:0})
 const sameAccount={id:2,account_id:10,attempt_index:2,is_retry_attempt:true,status_code:503},switched={id:3,account_id:20,attempt_index:3,status_code:200}
 assert.deepEqual(summarizeChain([initial,sameAccount,switched]),{attempts:3,retries:2,switches:1,highest:3,gaps:false,ambiguous:false,unindexed:0})
 assert.equal(chainTransition(initial,sameAccount),'同账号重试');assert.equal(chainTransition(sameAccount,switched),'换号重试')
 const gap={id:4,account_id:30,attempt_index:5}
 assert.equal(summarizeChain([initial,gap]).gaps,true);assert.equal(summarizeChain([initial,gap]).switches,0)
 assert.equal(chainTransition(initial,gap),'后续尝试 · 中间记录缺失')
 const internal={id:5,account_id:20,attempt_index:0}
 assert.equal(summarizeChain([initial,internal]).unindexed,1);assert.equal(summarizeChain([initial,internal]).retries,0)
 assert.equal(summarizeChain([internal]).attempts,0)
 const duplicate={...sameAccount,id:6,account_id:99}
 assert.equal(summarizeChain([initial,sameAccount,duplicate,switched]).ambiguous,true)
 assert.equal(summarizeChain([initial,sameAccount,duplicate,switched]).switches,0)
})
