package proxy

// issue #450：Codex 压缩轮发 tools:[] + tool_choice:"auto"，Grok 上游 400
// "A tool_choice was set on the request but no tools were specified."
// 净化必须在没有工具声明时撤掉 tool_choice，有工具时一字不动。

// 畸形形态（tools 不是数组）不属于本修复范围，保持原样交给上游报错。

// 不得顺手改动其他字段。

// 无 tool_choice 的请求原样返回（零开销快速路径，且不得凭空注入）。

// 主入口也要带上这步，且不影响既有的字段剥离/降级。

// 归一化把唯一的 web_search 工具整体丢弃后（external_web_access:false），
// 残留的 tool_choice:"auto" 也必须被撤掉——否则网关自己就造出同一个 400。
