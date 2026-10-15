import type {UsageLog} from './logView'
export type ErrorGuide={label:string;help:string;action:string;tone:'bad'|'neutral'|'good';inferred?:boolean}
export function errorGuide(log:UsageLog):ErrorGuide {
 const code=Number(log.status_code),kind=String(log.upstream_error_kind??'').toLowerCase(),text=String(log.error_message??'').toLowerCase()
 const signal=kind+' '+text
 if(code===499)return {label:'客户端取消',help:'客户端断开或取消了请求。',action:'检查调用端超时、主动取消记录和外层网关超时。',tone:'neutral'}
 if(code===401)return {label:'鉴权失败',help:'请求被上游以 401 拒绝。',action:'检查该账号凭据，尝试刷新 Token；不要仅凭此认定永久封禁。',tone:'bad'}
 if(/insufficient_quota|usage_limit_reached|usage_exhausted|quota_exceeded/.test(signal))return {label:'额度不足',help:'错误中包含上游额度限制信号。',action:'查看该账号额度与重置时间；有其他可用账号时检查调度。',tone:'bad',inferred:true}
 if(code===429||/rate_limit_exceeded|too_many_requests/.test(signal))return {label:'上游限流',help:code===429?'上游返回 429；可能是速率、容量或用量窗口限制。':'错误中包含上游限流信号，具体限制类型需要查看原文。',action:'查看原始错误、Retry-After 或冷却信息，再判断等待还是换号。',tone:'neutral'}
 if(/first.?token.?timeout|first.?output.?timeout|首字超时|首包超时/.test(signal))return {label:'首字等待超时',help:'错误中包含首次输出等待超时信号。',action:'检查上游响应、连接与首字超时配置；结合链路分段耗时定位。',tone:'bad',inferred:true}
 if(/ws_busy_acquire|queue.*timeout|scheduler.*timeout|排队超时/.test(signal))return {label:'排队或取连超时',help:'错误中包含等待账号或上游连接的超时信号。',action:'检查活跃并发、队列和 WS 连接获取耗时。',tone:'bad',inferred:true}
 if(/dial tcp|connection refused|no such host|tls handshake|proxyconnect|network is unreachable/.test(signal))return {label:'连接失败',help:'错误中包含网络、DNS、TLS 或代理连接失败信号。',action:'检查该账号代理和服务器到上游的连通性。',tone:'bad',inferred:true}
 if(/context deadline|timeout|timed out/.test(signal))return {label:'请求超时',help:'错误中包含超时信号，单凭此无法确定发生阶段。',action:'结合链路耗时和调用端超时设置排查。',tone:'bad',inferred:true}
 if(code===403)return {label:'访问被拒绝',help:'上游返回 403；不能仅凭状态码确定是权限、风控还是地域问题。',action:'查看原始错误，核对账号访问权限和出口。',tone:'bad'}
 if(code===598)return {label:'流执行失败',help:'响应流未成功完成，HTTP 连接建立不代表推理成功。',action:'查看失败事件、断流原因及是否已交付有效内容。',tone:'bad'}
 if(code>=500)return {label:'服务端或上游异常',help:'记录为 5xx，具体原因需要结合错误正文判断。',action:'查看连接与上游错误；确认安全重试边界后再调整策略。',tone:'bad'}
 if(code>=400)return {label:'请求被拒绝',help:'记录为 4xx，通常需要检查请求内容或访问条件。',action:'查看原始错误与请求入口、模型配置。',tone:'bad'}
 if(log.error_message||log.upstream_error_kind)return {label:'未分类错误',help:'存在错误信息，当前规则无法可靠分类。',action:'结合原始错误和请求链路排查。',tone:'neutral'}
 return {label:code>=200&&code<300?'执行成功':'结果未知',help:'',action:'',tone:code>=200&&code<300?'good':'neutral'}
}
