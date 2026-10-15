import assert from 'node:assert/strict'
import {spawn} from 'node:child_process'
import {chromium} from 'playwright'
import {setTimeout as delay} from 'node:timers/promises'
const origin='http://127.0.0.1:4177'
// Own Vite directly so cleanup terminates the server, not just an npm wrapper.
const server=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','4177','--strictPort'],{stdio:'pipe'})
let output='',browser
server.stdout.on('data',chunk=>output+=chunk);server.stderr.on('data',chunk=>output+=chunk)
try{
 let ready=false
 for(let i=0;i<100;i++){if(server.exitCode!==null)throw new Error(output);try{if((await fetch(`${origin}/admin/`)).ok){ready=true;break}}catch{}await delay(100)}
 if(!ready)throw new Error(output)
 browser=await chromium.launch({channel:'chrome',headless:true})
 const page=await browser.newPage({viewport:{width:1440,height:1000}}),failures=[],patches=[],settingsPatches=[],imports=[],consumes=[]
 page.on('pageerror',error=>failures.push(error.message))
 let account={id:1,name:'主账号',email:'operator@example.com',plan_type:'plus',status:'normal',enabled:true,locked:false,active_requests:1,dynamic_concurrency_limit:2,health_tier:'healthy',scheduler_score:105,usage_percent_5h:12,usage_percent_7d:38,usage_window_7d_kind:'monthly',usage_window_7d_seconds:2592000,reset_5h_at:'2026-10-15T08:00:00Z',reset_7d_at:'2026-11-01T00:00:00Z',rate_limit_reset_credits:2,subscription_expires_at:'2026-11-02T00:00:00Z',access_token_expires_at:'2026-10-16T10:00:00Z',codex_last_refresh_at:'2026-10-14T08:00:00Z',codex_usage_updated_at:'2026-10-14T08:05:00Z',subscription:{business_status:'active',sync_state:'confirmed',expires_at:'2026-11-02T00:00:00Z',last_checked_at:'2026-10-14T08:00:00Z'},auto_pause_5h_threshold:0.8,model_cooldowns:[{model:'limited-model',reason:'rate_limited',reset_at:'2099-10-15T08:00:00Z'}],codex_fingerprint_mode:'single_machine_multi_window',proxy_url:'',custom_headers:{'User-Agent':'codex-tui/0.153.3'}}
 let settings={max_concurrency:2,auto_pause_5h_threshold:0.95,background_refresh_interval_minutes:2,scheduler_mode:'remaining_quota',scheduler_engine:'indexed',affinity_mode:'bounded',codex_fingerprint_default_mode:'off',codex_user_agent_config:'{"client_name":"codex-tui"}',codex_unified_client_identity_enabled:false,codex_force_websocket:true,usage_log_mode:'full',response_cache_config_generation:5,codex_client_versions:[]},expire=false,subscriptionFailed=true,cardsFailed=false,proxiesFailed=false
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
  if(path==='/accounts/1/usage/refresh'&&method==='POST'){account={...account,usage_percent_5h:26};return json({refreshed:true})}
  if(path==='/accounts/1/reset-credits'&&method==='GET'){
   if(cardsFailed)return route.fulfill({status:502,contentType:'application/json',body:JSON.stringify({error:'重置卡查询失败'})})
   return json({available_count:account.rate_limit_reset_credits,credits:[{id:'card-1',granted_at:'2026-10-13T00:00:00Z',expires_at:'2026-10-20T00:00:00Z'}]})
  }
  if(path==='/accounts/1/reset-credits'&&method==='POST'){consumes.push(body);account={...account,rate_limit_reset_credits:1};return json({usage_refreshed:false,rate_limit_reset_credits:1})}
  if(path==='/accounts/1/subscription/refresh'&&method==='POST')return json(subscriptionFailed?{outcome:'failed',error:'订阅查询失败，保留上次信息'}:{outcome:'unchanged'})
  if(path==='/accounts/1/usage'&&method==='GET')return json({total_requests:11,total_tokens:12000,input_tokens:10000,output_tokens:2000,cached_tokens:1000,error_requests:1})
  if(path==='/accounts/1/scheduler'&&method==='PATCH'){patches.push(body);account={...account,...body};return json({message:'已保存'})}
  if(path==='/accounts'&&method==='POST'){imports.push(body);return route.fulfill({status:200,contentType:'text/event-stream',body:'data: {"type":"progress","current":1}\n\ndata: {"type":"complete","success":1,"failed":0}\n\n'})}
  if(path==='/settings'&&method==='GET')return json(settings)
  if(path==='/settings'&&method==='PUT'){settingsPatches.push(body);settings={...settings,...body};return json(settings)}
  if(path==='/core-status'){if(expire)return route.fulfill({status:401,contentType:'application/json',body:'{"error":"管理密钥无效"}'});return json({available:1,total:1,active_requests:1,scheduler:{engine:'indexed',waiters:0,max_waiters:100,max_waiters_per_key:50}})}
  if(path==='/proxies'){
   if(proxiesFailed)return route.fulfill({status:502,contentType:'application/json',body:JSON.stringify({error:'测试代理列表加载失败'})})
   return json({proxies:[{id:1,url:'http://proxy.example:8080',label:'测试代理',enabled:true,test_status:'untested',bound_count:0},{id:2,url:'socks5://disabled.example:1080',label:'停用代理',enabled:false,test_status:'untested',bound_count:0}]})
  }
  if(path==='/usage/logs'||path==='/usage/requests')return json({logs:[],total:0})
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
 const detail=page.getByRole('dialog')
 await detail.getByRole('heading',{name:'上游额度',exact:true}).waitFor()
 await detail.getByRole('heading',{name:'当前可用性与恢复条件',exact:true}).waitFor()
 await detail.getByText('模型 limited-model 冷却',{exact:true}).waitFor();await detail.getByText('仅影响此模型',{exact:true}).waitFor()
 assert.equal(await detail.getByRole('progressbar',{name:'5 小时额度已用比例'}).getAttribute('value'),'12')
 await detail.getByText('月周期',{exact:true}).waitFor();await detail.getByText('Access Token 到期',{exact:true}).waitFor()
 await detail.getByRole('button',{name:'刷新额度',exact:true}).click()
 await detail.getByRole('progressbar',{name:'5 小时额度已用比例'}).waitFor()
 await page.getByText('额度已刷新',{exact:true}).waitFor()
 assert.equal(await detail.getByRole('progressbar',{name:'5 小时额度已用比例'}).getAttribute('value'),'26')
 await detail.getByRole('button',{name:'查询订阅有效期',exact:true}).click()
 await detail.getByRole('alert').filter({hasText:'订阅查询失败，保留上次信息'}).waitFor()
 subscriptionFailed=false;await detail.getByRole('button',{name:'查询订阅有效期',exact:true}).click()
 await detail.getByText('订阅信息已查询。',{exact:true}).waitFor()
 await detail.getByRole('button',{name:'查看重置卡',exact:true}).click()
 await detail.getByText('card-1',{exact:true}).waitFor();assert.equal(consumes.length,0)
 page.once('dialog',dialog=>dialog.dismiss());await detail.getByRole('button',{name:'使用 1 张重置卡',exact:true}).click();assert.equal(consumes.length,0)
 page.once('dialog',dialog=>dialog.accept());await detail.getByRole('button',{name:'使用 1 张重置卡',exact:true}).click()
 await detail.getByText(/重置请求已成功，剩余 1 张/).waitFor();assert.equal(consumes.length,1)
 await detail.getByRole('button',{name:'查看近 30 天用量',exact:true}).click();await detail.getByText('12000',{exact:true}).waitFor()
 cardsFailed=true;await detail.getByRole('button',{name:'查看重置卡',exact:true}).click()
 await detail.getByRole('alert').filter({hasText:'重置卡查询失败'}).waitFor()
 assert.equal(await detail.getByRole('button',{name:'使用 1 张重置卡',exact:true}).count(),0)
 await page.screenshot({path:'/tmp/codex2api-core-account-details.png',fullPage:true})
 await page.setViewportSize({width:390,height:844});await page.screenshot({path:'/tmp/codex2api-core-account-details-mobile.png',fullPage:true})
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true,'mobile account detail overflow')
 await page.setViewportSize({width:1440,height:1000})
 await detail.getByRole('tab',{name:'账号配置',exact:true}).click()
 const proxySelect=page.getByLabel('独立代理 URL',{exact:true}),timezoneSelect=page.getByLabel('账号时区',{exact:true})
 await proxySelect.selectOption('http://proxy.example:8080');await timezoneSelect.selectOption('Asia/Shanghai')
 assert.equal(await proxySelect.getByRole('option',{name:/停用代理/}).isDisabled(),true)
 await page.getByLabel('设备与会话指纹收敛').selectOption('device')
 await page.getByLabel('账号并发覆盖（留空继承）').fill('3')
 assert.equal(await page.getByLabel('5h 暂停阈值（%）',{exact:true}).inputValue(),'80')
 await page.getByLabel('5h 暂停阈值（%）',{exact:true}).fill('90')
 await page.getByLabel('账号自定义请求头（JSON）').fill('{"User-Agent":"codex-tui/0.153.4"}')
 await page.getByRole('button',{name:'保存配置',exact:true}).click();await page.getByText('账号配置已保存').waitFor()
 assert.deepEqual(patches[0],{proxy_url:'http://proxy.example:8080',timezone:'Asia/Shanghai',codex_fingerprint_mode:'device',base_concurrency_override:3,auto_pause_5h_threshold:0.9,custom_headers:{'User-Agent':'codex-tui/0.153.4'}})
 await proxySelect.selectOption('__custom__');await page.getByLabel('手动填写独立代理 URL',{exact:true}).fill('socks5://custom.example:1080')
 await timezoneSelect.selectOption('__custom__');await page.getByLabel('手动填写账号时区',{exact:true}).fill('Etc/GMT-8')
 await page.getByRole('button',{name:'保存配置',exact:true}).click();await page.getByText('账号配置已保存').waitFor()
 assert.deepEqual(patches[1],{proxy_url:'socks5://custom.example:1080',timezone:'Etc/GMT-8'})
 await page.getByRole('button',{name:'关闭',exact:true}).click();await page.getByRole('button',{name:'管理',exact:true}).click()
 proxiesFailed=true;await page.getByRole('tab',{name:'账号配置',exact:true}).click()
 await page.getByRole('alert').filter({hasText:'代理列表加载失败'}).waitFor()
 assert.equal(await page.getByLabel('手动填写独立代理 URL',{exact:true}).inputValue(),'socks5://custom.example:1080')
 assert.equal(await page.getByLabel('手动填写账号时区',{exact:true}).inputValue(),'Etc/GMT-8');assert.equal(patches.length,2)
 proxiesFailed=false;await page.getByRole('button',{name:'重新加载代理',exact:true}).click()
 await page.getByText('正在加载代理列表…',{exact:true}).waitFor({state:'hidden'})
 await page.getByLabel('独立代理 URL',{exact:true}).selectOption('http://proxy.example:8080')
 await page.getByLabel('账号时区',{exact:true}).selectOption('America/New_York')
 await page.screenshot({path:'/tmp/codex2api-core-selectors-desktop.png',fullPage:true})
 await page.setViewportSize({width:390,height:844});await page.screenshot({path:'/tmp/codex2api-core-selectors-mobile.png',fullPage:true})
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true,'mobile selectors overflow')
 await page.setViewportSize({width:1440,height:1000})
 await page.getByLabel('独立代理 URL',{exact:true}).selectOption('');await page.getByLabel('账号时区',{exact:true}).selectOption('')
 await page.getByRole('button',{name:'保存配置',exact:true}).click();await page.getByText('账号配置已保存').waitFor()
 assert.deepEqual(patches[2],{proxy_url:'',timezone:''})

 await page.getByRole('button',{name:'关闭',exact:true}).click()
 await page.getByRole('button',{name:'导入账号',exact:true}).click();await page.getByLabel('凭据',{exact:true}).fill('test-refresh-token')
 await page.getByRole('button',{name:'导入',exact:true}).click();await page.getByRole('dialog').waitFor({state:'hidden'});assert.equal(imports[0].skip_refresh,true)
 await page.getByRole('link',{name:'运行设置',exact:true}).click();const fingerprint=page.getByLabel('新账号默认收敛模式');await fingerprint.selectOption('device')
 assert.equal(await fingerprint.inputValue(),'device');await fingerprint.locator('..').getByText('当前选择：同一账号使用固定设备标识；不同客户端的会话与线程仍独立。',{exact:true}).waitFor()
 await fingerprint.locator('..').getByText('查看全部选项说明',{exact:true}).click();await fingerprint.locator('..').getByText('统一设备、会话与线程',{exact:true}).waitFor()
 await page.screenshot({path:'/tmp/codex2api-core-settings-help.png',fullPage:true})
 await page.getByRole('button',{name:/保存 1 项修改/}).click();await page.getByText('运行设置已保存').waitFor();assert.deepEqual(settingsPatches[0],{codex_fingerprint_default_mode:'device'})
 await page.getByRole('button',{name:'编辑高级 JSON 配置'}).click();const editor=page.getByLabel('高级运行配置 JSON'),advanced=JSON.parse(await editor.inputValue());assert.equal(advanced.response_cache_config_generation,undefined)
 advanced.codex_user_agent_config='{"client_name":"codex-tui","client_version":"0.153.4"}'
 await editor.fill(JSON.stringify(advanced,null,2));assert.equal(await editor.evaluate(element=>document.activeElement===element),true)
 await page.getByRole('button',{name:'保存修改',exact:true}).click();await page.getByRole('dialog').waitFor({state:'hidden'});assert.deepEqual(settingsPatches[1],{codex_user_agent_config:advanced.codex_user_agent_config})
 const threshold=page.getByLabel('5h 配额暂停阈值（%）',{exact:true});assert.equal(await threshold.inputValue(),'95');await threshold.fill('92')
 await page.getByRole('button',{name:/保存 1 项修改/}).click();await page.getByText('运行设置已保存').waitFor();assert.deepEqual(settingsPatches[2],{auto_pause_5h_threshold:0.92})
 for(const name of ['调度状态','代理出口','请求日志','渠道密钥']){await page.getByRole('link',{name,exact:true}).click();await page.getByRole('heading',{name,exact:true}).waitFor();if(name==='代理出口')await page.getByText('未测试',{exact:true}).first().waitFor()}
 await page.getByRole('link',{name:'账号池',exact:true}).click();await page.getByText('operator@example.com').waitFor()
 await page.screenshot({path:'/tmp/codex2api-core-desktop.png',fullPage:true});await page.setViewportSize({width:390,height:844})
 await page.screenshot({path:'/tmp/codex2api-core-mobile.png',fullPage:true});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true,'mobile page overflow')
 expire=true;await page.getByRole('link',{name:'调度状态',exact:true}).click();await page.getByRole('button',{name:'登录',exact:true}).waitFor();assert.equal(await page.evaluate(()=>sessionStorage.getItem('codex-account-service:admin-key')),null)
 assert.deepEqual(failures,[])
 console.log('Browser checks passed: login, proxy and timezone selection, custom values, load failure preservation, clear overrides, real quota fields, month window, subscription failure, reset card confirmation, card query failure, usage history, Chinese help, paged search, fingerprint patch, SSE import, partial settings, editor focus, navigation, mobile layout, expired authentication.')
}finally{await browser?.close();server.kill('SIGTERM')}
