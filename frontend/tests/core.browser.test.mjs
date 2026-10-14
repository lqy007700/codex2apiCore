import assert from 'node:assert/strict'
import {spawn} from 'node:child_process'
import {chromium} from 'playwright'
import {setTimeout as delay} from 'node:timers/promises'
const origin='http://127.0.0.1:4177'
const server=spawn('npm',['run','dev','--','--host','127.0.0.1','--port','4177','--strictPort'],{stdio:'pipe'})
let output='',browser
server.stdout.on('data',chunk=>output+=chunk);server.stderr.on('data',chunk=>output+=chunk)
try{
 let ready=false
 for(let i=0;i<100;i++){if(server.exitCode!==null)throw new Error(output);try{if((await fetch(`${origin}/admin/`)).ok){ready=true;break}}catch{}await delay(100)}
 if(!ready)throw new Error(output)
 browser=await chromium.launch({channel:'chrome',headless:true})
 const page=await browser.newPage({viewport:{width:1440,height:1000}}),failures=[],patches=[],settingsPatches=[],imports=[]
 page.on('pageerror',error=>failures.push(error.message))
 let account={id:1,name:'主账号',email:'operator@example.com',plan_type:'plus',status:'normal',enabled:true,locked:false,active_requests:1,dynamic_concurrency_limit:2,health_tier:'healthy',scheduler_score:105,codex_5h_used_percent:12,codex_7d_used_percent:38,codex_fingerprint_mode:'single_machine_multi_window',proxy_url:'',custom_headers:{'User-Agent':'codex-tui/0.153.3'}}
 let settings={max_concurrency:2,scheduler_mode:'remaining_quota',scheduler_engine:'indexed',affinity_mode:'bounded',codex_fingerprint_default_mode:'off',codex_user_agent_config:'{"client_name":"codex-tui"}',codex_unified_client_identity_enabled:false,codex_force_websocket:true,usage_log_mode:'full',response_cache_config_generation:5,codex_client_versions:[]},expire=false
 await page.route('**/api/admin/**',async route=>{
  const req=route.request(),path=new URL(req.url()).pathname.replace('/api/admin',''),method=req.method()
  const body=req.postData()&&req.headers()['content-type']?.includes('application/json')?req.postDataJSON():{}
  const json=value=>route.fulfill({status:200,contentType:'application/json',body:JSON.stringify(value)})
  if(path==='/bootstrap-status')return json({needs_bootstrap:false})
  assert.equal(req.headers()['x-admin-key'],'test-admin')
  if(path==='/health')return json({status:'ok'})
  if(path==='/accounts'&&method==='GET'){
   const query=new URL(req.url()).searchParams
   assert.equal(query.get('view'),'page');assert.equal(query.get('page_size'),'50')
   const accounts=query.get('search')==='missing-account'?[]:[account]
   return json({accounts,total:accounts.length})
  }
  if(path==='/accounts/1'&&method==='GET')return json(account)
  if(path==='/accounts/1/scheduler'&&method==='PATCH'){patches.push(body);account={...account,...body};return json({message:'已保存'})}
  if(path==='/accounts'&&method==='POST'){imports.push(body);return route.fulfill({status:200,contentType:'text/event-stream',body:'data: {"type":"progress","current":1}\n\ndata: {"type":"complete","success":1,"failed":0}\n\n'})}
  if(path==='/settings'&&method==='GET')return json(settings)
  if(path==='/settings'&&method==='PUT'){settingsPatches.push(body);settings={...settings,...body};return json(settings)}
  if(path==='/core-status'){if(expire)return route.fulfill({status:401,contentType:'application/json',body:'{"error":"管理密钥无效"}'});return json({available:1,total:1,active_requests:1,scheduler:{engine:'indexed',waiters:0,max_waiters:100,max_waiters_per_key:50}})}
  if(path==='/proxies')return json({proxies:[]})
  if(path==='/usage/logs')return json({logs:[]})
  if(path==='/keys')return json({keys:[]})
  failures.push(`unexpected ${method} ${path}`);return route.fulfill({status:404,body:'{}'})
 })
 await page.goto(`${origin}/admin/`)
 await page.getByLabel('管理密钥').fill('test-admin');await page.getByRole('button',{name:'登录',exact:true}).click()
 await page.getByRole('heading',{name:'账号池',exact:true}).waitFor()
 await page.getByLabel('搜索账号').fill('missing-account');await page.getByRole('button',{name:'搜索',exact:true}).click()
 await page.getByText('尚无匹配账号。导入 Refresh Token 或 Access Token 后开始管理。').waitFor()
 await page.getByLabel('搜索账号').fill('');await page.getByRole('button',{name:'搜索',exact:true}).click()
 await page.getByRole('button',{name:'管理',exact:true}).click()
 await page.getByLabel('设备与会话指纹收敛').selectOption('device')
 await page.getByLabel('账号并发覆盖（留空继承）').fill('3')
 await page.getByLabel('账号自定义请求头（JSON）').fill('{"User-Agent":"codex-tui/0.153.4"}')
 await page.getByRole('button',{name:'保存配置',exact:true}).click();await page.getByText('账号配置已保存').waitFor()
 assert.deepEqual(patches[0],{codex_fingerprint_mode:'device',base_concurrency_override:3,custom_headers:{'User-Agent':'codex-tui/0.153.4'}})
 await page.getByRole('button',{name:'关闭',exact:true}).click()
 await page.getByRole('button',{name:'导入账号',exact:true}).click();await page.getByLabel('凭据',{exact:true}).fill('test-refresh-token')
 await page.getByRole('button',{name:'导入',exact:true}).click();await page.getByRole('dialog').waitFor({state:'hidden'});assert.equal(imports[0].skip_refresh,true)
 await page.getByRole('link',{name:'运行设置',exact:true}).click();await page.getByLabel('新账号默认收敛模式').selectOption('device')
 await page.getByRole('button',{name:/保存 1 项修改/}).click();await page.getByText('运行设置已保存').waitFor();assert.deepEqual(settingsPatches[0],{codex_fingerprint_default_mode:'device'})
 await page.getByRole('button',{name:'编辑高级 JSON 配置'}).click();const editor=page.getByLabel('高级运行配置 JSON'),advanced=JSON.parse(await editor.inputValue());assert.equal(advanced.response_cache_config_generation,undefined)
 advanced.codex_user_agent_config='{"client_name":"codex-tui","client_version":"0.153.4"}'
 await editor.fill(JSON.stringify(advanced,null,2));assert.equal(await editor.evaluate(element=>document.activeElement===element),true)
 await page.getByRole('button',{name:'保存修改',exact:true}).click();await page.getByRole('dialog').waitFor({state:'hidden'});assert.deepEqual(settingsPatches[1],{codex_user_agent_config:advanced.codex_user_agent_config})
 for(const name of ['调度状态','代理出口','请求日志','渠道密钥']){await page.getByRole('link',{name,exact:true}).click();await page.getByRole('heading',{name,exact:true}).waitFor()}
 await page.getByRole('link',{name:'账号池',exact:true}).click();await page.getByText('operator@example.com').waitFor()
 await page.screenshot({path:'/tmp/codex2api-core-desktop.png',fullPage:true});await page.setViewportSize({width:390,height:844})
 await page.screenshot({path:'/tmp/codex2api-core-mobile.png',fullPage:true});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true,'mobile page overflow')
 expire=true;await page.getByRole('link',{name:'调度状态',exact:true}).click();await page.getByRole('button',{name:'登录',exact:true}).waitFor();assert.equal(await page.evaluate(()=>sessionStorage.getItem('codex-account-service:admin-key')),null)
 assert.deepEqual(failures,[])
 console.log('Browser checks passed: login, paged search, fingerprint patch, SSE import, partial settings, editor focus, navigation, mobile layout, expired authentication.')
}finally{await browser?.close();server.kill('SIGTERM')}
