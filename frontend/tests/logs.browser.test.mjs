import assert from 'node:assert/strict'
import {spawn} from 'node:child_process'
import {chromium} from 'playwright'
import {setTimeout as delay} from 'node:timers/promises'
const origin='http://127.0.0.1:4178'
const server=spawn('npm',['run','dev','--','--host','127.0.0.1','--port','4178','--strictPort'],{stdio:'pipe'})
let output='',browser
server.stdout.on('data',chunk=>output+=chunk);server.stderr.on('data',chunk=>output+=chunk)
try{
 for(let i=0;i<100;i++){if(server.exitCode!==null)throw new Error(output);try{if((await fetch(`${origin}/admin/`)).ok)break}catch{}await delay(100)}
 browser=await chromium.launch({channel:'chrome',headless:true})
 const context=await browser.newContext({viewport:{width:1440,height:1000},hasTouch:true,permissions:['clipboard-read','clipboard-write']})
 const page=await context.newPage(),failures=[],queries=[]
 page.on('pageerror',error=>failures.push(error.message))
 const longError='上游请求失败：rate_limit_exceeded\n'+Array.from({length:60},(_,i)=>`诊断行 ${i}: ${'long-diagnostic-fragment-'.repeat(10)}`).join('\n')+'\nERROR_END <script>window.injected=true</script>'
 const records=Array.from({length:45},(_,index)=>({id:index+1,request_id:`req-${String(index+1).padStart(3,'0')}`,upstream_request_id:`upstream-${index+1}`,account_id:index%2+1,account_name:index%2?'备用账号':'主账号',model:'gpt-5.1-codex',status_code:index%3===0?598:index%3===1?200:499,error_message:index%3===0?longError:'',upstream_error_kind:index%3===0?'rate_limit_exceeded':'',created_at:'2026-10-14T08:01:00Z',first_token_ms:index%3===1?null:1200,duration_ms:5200,input_tokens:2200,output_tokens:index%3===1?null:120,cached_tokens:900,stream:true,via_websocket:true,is_retry_attempt:index%3===0,attempt_index:index%3===0?2:1}))
 let failNext=false,chainFail=false
 const chainRecords=[{...records[0],attempt_index:1,status_code:429,upstream_request_id:'upstream-first'},{...records[0],id:100,attempt_index:2,status_code:503,upstream_request_id:'upstream-second'},{...records[0],id:101,account_id:2,account_name:'备用账号',attempt_index:3,status_code:200,is_retry_attempt:false,error_message:'',upstream_error_kind:'',upstream_request_id:'upstream-final'}]
 chainRecords[2].execution_trace=JSON.stringify({started_at:'2026-10-14T08:00:00Z',elapsed_ms:12000,truncated:false,events:[
 {sequence:1,kind:'request_received',at_ms:0},{sequence:2,kind:'selection_started',at_ms:10},{sequence:3,kind:'queue_finished',at_ms:110,duration_ms:100},{sequence:4,kind:'selection_finished',at_ms:120,duration_ms:110},{sequence:5,kind:'account_selected',at_ms:121,account_id:1},{sequence:6,kind:'upstream_started',at_ms:122,account_id:1},{sequence:7,kind:'http_connection_finished',at_ms:222,duration_ms:100},{sequence:8,kind:'request_sent',at_ms:223,account_id:1},{sequence:9,kind:'first_content',at_ms:1223,account_id:1},{sequence:10,kind:'retry_wait_finished',at_ms:1600,duration_ms:200},{sequence:11,kind:'attempt_finished',at_ms:12000,account_id:2}]})
 let chainData={request_id:'req-001',linked:true,log_mode:'full',total:3,truncated:false,records:chainRecords}
 await page.route('**/api/admin/**',async route=>{
  const url=new URL(route.request().url()),path=url.pathname.replace('/api/admin','')
  const json=value=>route.fulfill({status:200,contentType:'application/json',body:JSON.stringify(value)})
  if(path==='/usage/logs/1/chain'){
   assert.equal(url.searchParams.size,0,'chain must not inherit list filters')
   if(chainFail){chainFail=false;return route.fulfill({status:503,contentType:'application/json',body:'{"error":"链路查询失败"}'})}
   return json(chainData)
  }
  if(path==='/usage/requests')return json({logs:[{...chainRecords[2],id:1,request_id:'req-001',attempt_count:3,retry_count:2,account_count:2,record_count:3,elapsed_ms:12000}],total:1,log_mode:'full'})
  if(path==='/usage/logs'){
   const q=url.searchParams;queries.push(q)
   assert(q.has('start')&&q.has('end'));assert.equal(q.get('include_canceled'),'true');assert.equal(q.has('limit'),false)
   if(failNext){failNext=false;return route.fulfill({status:503,contentType:'application/json',body:'{"error":"日志查询暂时失败"}'})}
   let rows=records
   if(q.get('error_only')==='true')rows=rows.filter(log=>log.status_code>=400)
   if(q.get('status'))rows=rows.filter(log=>String(log.status_code)===q.get('status'))
   if(q.get('q'))rows=rows.filter(log=>log.request_id.includes(q.get('q')))
   if(q.get('account_id'))rows=rows.filter(log=>String(log.account_id)===q.get('account_id'))
   const start=(Number(q.get('page'))-1)*Number(q.get('page_size'))
   return json({logs:rows.slice(start,start+Number(q.get('page_size'))),total:rows.length})
  }
  if(path==='/bootstrap-status')return json({needs_bootstrap:false})
  if(path==='/health')return json({status:'ok'})
  failures.push(`Unexpected ${path}`);return route.fulfill({status:404,body:'{}'})
 })
 await page.addInitScript(()=>sessionStorage.setItem('codex-account-service:admin-key','test-admin'))
 await page.goto(`${origin}/admin/logs`)
 await page.getByText('共 1 条匹配记录',{exact:true}).waitFor()
 assert.equal(await page.getByRole('button',{name:'按请求合并',exact:true}).getAttribute('aria-pressed'),'true')
 await page.getByText('3 次已记录尝试 · 2 次重试 · 2 个账号',{exact:true}).waitFor()
 assert.equal(await page.locator('tbody tr').count(),1)
 await page.screenshot({path:'/tmp/codex2api-merged-requests.png',fullPage:true})
 await page.getByRole('button',{name:'逐次尝试',exact:true}).click()
 await page.getByText('共 45 条匹配记录',{exact:true}).waitFor()
 assert.equal(await page.locator('tbody tr').count(),20)
 assert.equal(await page.getByText('ERROR_END',{exact:false}).count(),0,'full error must stay unmounted until preview opens')
 await page.getByRole('button',{name:'请求链路 req-001',exact:true}).click()
 const chain=page.getByRole('dialog',{name:'请求链路',exact:true})
 await chain.getByText('第 3 次尝试',{exact:true}).waitFor()
 assert.equal(await chain.locator('.chain-node').count(),3)
 await chain.getByRole('heading',{name:'事件与分段耗时',exact:true}).waitFor()
 await chain.getByText('1000 ms',{exact:true}).waitFor()
 await chain.getByText('查看全部 11 条事件',{exact:true}).click();await chain.getByText('收到请求',{exact:true}).waitFor()
 await chain.getByRole('button',{name:'查看错误详情 req-001',exact:true}).first().hover();await page.getByRole('dialog',{name:'错误详情',exact:true}).waitFor()
 await page.keyboard.press('Escape');await page.getByRole('dialog',{name:'错误详情',exact:true}).waitFor({state:'hidden'});assert.equal(await chain.isVisible(),true,'Esc on nested preview must keep chain open')
 assert.equal(await chain.locator('.chain-summary>div').nth(0).locator('strong').textContent(),'3')
 assert.equal(await chain.locator('.chain-summary>div').nth(1).locator('strong').textContent(),'2')
 assert.equal(await chain.locator('.chain-summary>div').nth(2).locator('strong').textContent(),'1')
 await chain.getByText('同账号重试',{exact:true}).waitFor();await chain.getByText('换号重试',{exact:true}).waitFor()
 await chain.locator('.chain-node').last().getByText('查看本次执行字段',{exact:true}).click();await chain.getByText('upstream-final',{exact:true}).waitFor()
 await chain.getByRole('button',{name:'复制链路请求 ID',exact:true}).click();assert.equal(await page.evaluate(()=>navigator.clipboard.readText()),'req-001')
 await chain.evaluate(element=>element.scrollTop=0);await delay(200);await page.screenshot({path:'/tmp/codex2api-request-chain.png'})
 await page.setViewportSize({width:390,height:844});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true,'mobile chain overflow')
 await chain.evaluate(element=>element.scrollTop=0);await delay(200);await page.screenshot({path:'/tmp/codex2api-request-chain-mobile.png'});await page.setViewportSize({width:1440,height:1000})
 chainData={...chainData,records:[chainRecords[0]],total:1}
 await chain.getByRole('button',{name:'刷新链路',exact:true}).click();await chain.getByText('尚无后续尝试记录，不能确认计划的重试已经执行。',{exact:true}).waitFor()
 assert.equal(await chain.locator('.chain-summary>div').nth(1).locator('strong').textContent(),'0')
 chainData={...chainData,log_mode:'errors',records:[chainRecords[0],chainRecords[2]],total:2}
 await chain.getByRole('button',{name:'刷新链路',exact:true}).click();await chain.getByText(/当前日志模式为仅错误/).waitFor();await chain.getByText(/尝试序号存在缺失/).waitFor()
 chainData={...chainData,request_id:'',linked:false,records:[{...chainRecords[0],attempt_index:0,request_id:''}],total:1}
 await chain.getByRole('button',{name:'刷新链路',exact:true}).click();await chain.getByText(/此记录没有请求关联 ID/).waitFor()
 assert.equal(await chain.getByRole('button',{name:'复制链路请求 ID',exact:true}).isDisabled(),true)
 chainFail=true;await chain.getByRole('button',{name:'刷新链路',exact:true}).click();await chain.getByRole('alert').filter({hasText:'链路查询失败'}).waitFor()
 chainData={...chainData,request_id:'req-001',linked:true,log_mode:'full',records:chainRecords,total:3}
 await chain.getByRole('button',{name:'重试',exact:true}).click();await chain.getByText('第 3 次尝试',{exact:true}).waitFor()
 await page.keyboard.press('Escape');await chain.waitFor({state:'hidden'})
 const preview=page.getByRole('button',{name:'查看错误详情 req-001',exact:true})
 await preview.hover()
 const popover=page.getByRole('dialog',{name:'错误详情',exact:true})
 await popover.waitFor();assert((await popover.locator('pre').textContent()).includes('ERROR_END'));await popover.getByText('上游限流',{exact:true}).waitFor()
 await page.mouse.move(20,20);await popover.waitFor({state:'hidden'});await preview.hover();await popover.waitFor()
 assert.equal(await page.evaluate(()=>window.injected),undefined,'error HTML must remain plain text')
 await popover.getByRole('button',{name:'复制错误',exact:true}).click();await popover.getByText('已复制错误',{exact:true}).waitFor()
 assert.equal(await page.evaluate(()=>navigator.clipboard.readText()),longError)
 const bounds=await popover.boundingBox();assert(bounds.x>=0&&bounds.y>=0&&bounds.x+bounds.width<=1440&&bounds.y+bounds.height<=1000)
 await page.keyboard.press('Escape');await popover.waitFor({state:'hidden'})
 await preview.click();await page.mouse.move(20,20);await delay(250);assert.equal(await popover.isVisible(),true,'clicked preview remains pinned')
 await page.getByRole('heading',{name:'请求日志',exact:true}).click();await popover.waitFor({state:'hidden'});await preview.click();await popover.waitFor()
 await popover.getByRole('button',{name:'完整请求详情',exact:true}).click()
 const detail=page.getByRole('dialog',{name:'请求详情',exact:true})
 await detail.waitFor();await detail.getByText('upstream-1',{exact:true}).waitFor()
 await detail.getByRole('button',{name:'复制请求 ID',exact:true}).click();await detail.getByText('已复制请求 ID',{exact:true}).waitFor()
 assert.equal(await page.evaluate(()=>navigator.clipboard.readText()),'req-001')
 await page.keyboard.press('Escape');await detail.waitFor({state:'hidden'})
 await page.getByRole('button',{name:'下一页',exact:true}).click();await page.getByText('第 2 / 3 页 · 本页 20 条',{exact:true}).waitFor()
 const anchor=queries[0].get('end');assert.equal(queries.at(-1).get('end'),anchor,'pagination must keep a stable time range')
 const count=queries.length
 await page.getByLabel('搜索日志',{exact:true}).fill('req-045');assert.equal(queries.length,count,'typing must not issue queries')
 await page.getByRole('button',{name:'查询',exact:true}).click();await page.getByText('共 1 条匹配记录',{exact:true}).waitFor()
 assert.equal(queries.at(-1).get('page'),'1');await page.getByRole('button',{name:'请求详情 req-045',exact:true}).waitFor()
 await page.getByRole('button',{name:'重置筛选',exact:true}).click();await page.getByText('共 45 条匹配记录',{exact:true}).waitFor()
 await page.getByRole('button',{name:'仅看错误',exact:true}).click();await page.getByText('共 30 条匹配记录',{exact:true}).waitFor()
 assert.equal(queries.at(-1).get('error_only'),'true')
 await page.getByLabel('请求状态',{exact:true}).selectOption('499');await page.getByRole('button',{name:'查询',exact:true}).click()
 await page.getByText('共 15 条匹配记录',{exact:true}).waitFor();assert.equal(queries.at(-1).get('status'),'499')
 await page.getByText('更多筛选 · 账号 / 模型 / 渠道 / 传输',{exact:true}).click()
 await page.getByLabel('账号 ID',{exact:true}).fill('1');await page.getByLabel('上游传输',{exact:true}).selectOption('true')
 await page.getByRole('button',{name:'查询',exact:true}).click();await page.getByText('共 8 条匹配记录',{exact:true}).waitFor()
 assert.equal(queries.at(-1).get('account_id'),'1');assert.equal(queries.at(-1).get('via_websocket'),'true')
 await page.getByLabel('时间范围',{exact:true}).selectOption('custom')
 await page.getByLabel('开始时间',{exact:true}).fill('2026-10-15T00:00');await page.getByLabel('结束时间',{exact:true}).fill('2026-10-14T00:00')
 const beforeInvalid=queries.length
 await page.getByRole('button',{name:'查询',exact:true}).click();await page.getByRole('alert').filter({hasText:'结束时间须晚于开始时间'}).waitFor();assert.equal(queries.length,beforeInvalid)
 await page.getByLabel('开始时间',{exact:true}).fill('2026-10-13T00:00');await page.getByRole('button',{name:'查询',exact:true}).click()
 await page.getByRole('alert').waitFor({state:'hidden'});await page.getByText('共 8 条匹配记录',{exact:true}).waitFor()
 assert.equal(queries.at(-1).get('start'),await page.evaluate(()=>new Date('2026-10-13T00:00').toISOString()))
 await page.getByRole('button',{name:'重置筛选',exact:true}).click();await page.getByText('共 45 条匹配记录',{exact:true}).waitFor()
 await page.getByLabel('每页日志条数').selectOption('50');await page.getByText('第 1 / 1 页 · 本页 45 条',{exact:true}).waitFor()
 assert.equal(await page.getByRole('button',{name:'下一页',exact:true}).isDisabled(),true)
 failNext=true;await page.getByRole('button',{name:'刷新',exact:true}).click();await page.getByRole('alert').filter({hasText:'日志查询暂时失败'}).waitFor()
 await page.getByRole('button',{name:'重试',exact:true}).click();await page.getByText('共 45 条匹配记录',{exact:true}).waitFor()
 await page.getByText('更多筛选 · 账号 / 模型 / 渠道 / 传输',{exact:true}).click()
 await page.getByLabel('每页日志条数').selectOption('20');await page.getByText('第 1 / 3 页 · 本页 20 条',{exact:true}).waitFor()
 await page.screenshot({path:'/tmp/codex2api-request-logs.png',fullPage:true})
 await page.setViewportSize({width:390,height:844})
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true,'mobile log page overflow')
 await preview.scrollIntoViewIfNeeded();await preview.tap()
 await popover.waitFor()
 const mobileBounds=await popover.boundingBox();assert(mobileBounds.x>=0&&mobileBounds.y>=0&&mobileBounds.x+mobileBounds.width<=390&&mobileBounds.y+mobileBounds.height<=844,'mobile popup must fit viewport')
 await delay(500);await page.screenshot({path:'/tmp/codex2api-request-logs-mobile.png'})
 await page.getByRole('button',{name:'关闭错误详情',exact:true}).click();await popover.waitFor({state:'hidden'})
 assert.deepEqual(failures,[])
 console.log('Request log filters, pagination, request chain actual attempts/account changes, missing history, copy, hover/click and mobile checks passed')
}finally{await browser?.close();server.kill('SIGTERM')}
