import {useEffect, useState} from 'react'
import {formatTime} from './client'
import {accountAvailability} from './accountView'
import type {Account} from './types'
export function AccountAvailability({account,compact=false}:{account:Account;compact?:boolean}) {
 const [now,setNow]=useState(Date.now())
 useEffect(()=>{const timer=setInterval(()=>setNow(Date.now()),30000);return()=>clearInterval(timer)},[])
 const items=accountAvailability(account,now)
 const first=items[0]
 if(compact)return <div className="availability-compact"><span className={`badge ${first.tone}`}>{first.label}</span><small>{first.until?`预计检查：${formatTime(first.until)}`:first.action}</small>{items.length>1&&<small>另有 {items.length-1} 项限制，点击管理查看</small>}</div>
 return <section className="account-section"><h3>当前可用性与恢复条件</h3><p className="section-help">按服务快照列出阻塞条件。多个限制需要分别解除；倒计时仅表示下次检查时间。</p><div className="availability-list">{items.map((item,index)=><article key={index}><div><span className={`badge ${item.tone}`}>{item.label}</span><small>{({account:'账号限制',model:'仅影响此模型',capacity:'临时容量限制',unknown:'调度快照'})[item.scope]}</small></div><p>{item.reason}</p>{item.until&&<p>预计检查：{formatTime(item.until)} · {Math.max(0,Math.ceil((Date.parse(item.until)-now)/60000))} 分钟后</p>}<small>{item.action}</small></article>)}</div>{account.error_message&&<details><summary>最近原始错误</summary><pre>{account.error_message}</pre></details>}</section>
}
