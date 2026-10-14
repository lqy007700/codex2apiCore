package proxy

// native 直通的是传输形态，不是请求内容：Codex 专有工具形态原样发给 Grok 会
// 400。以下用例锁住"native 的 Responses 路由仍然跑 preflight"。

// Codex 专有工具已降级成 Grok 可接受的 function 形态。

// 别名必须回传，否则响应侧无法把工具名还原成 custom_tool_call。

// Grok 不接受的 Codex 顶层注入已剥离。

// native 的核心语义仍在：不强制 stream=true。

// 账号映射后的模型仍然生效。

// 非 Responses 的 native 路由（Chat / Messages）没有 Responses 语义可降级，
// 保持纯直通不变。
