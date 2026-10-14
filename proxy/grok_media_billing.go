package proxy

const (
	// grokVideoSettleNamespace 记录已结算的视频任务:SETNX 语义保证重复轮询、
	// 下载与多实例并发只结算一次。TTL 与任务绑定一致,绑定过期后任务也查不到了。
	grokVideoSettleNamespace = "grok_video_settle"
	grokVideoSettleOwner     = "settled"

	// xAI 以 USD ticks 自报成本:1 USD = 1e10 ticks。
	grokUSDTicksPerDollar = 1e10

	// 上游缺省时长:生成 8 秒、续写 6 秒(docs.x.ai videos API)。编辑沿用源视频
	// 长度,网关无从得知,只能依赖状态体里的 video.duration。
	grokVideoDefaultGenerationSeconds = 8
	grokVideoDefaultExtensionSeconds  = 6
)

// grokUpstreamCostUSD 解析 xAI 响应里的 usage.cost_in_usd_ticks;未自报返回 0。

// grokVideoExplicitSeconds 读取请求显式给出的时长;上游同时接受 duration 与
// OpenAI 兼容的 seconds,数字或数字字符串均可。

// grokVideoRequestedSeconds 返回提交时可推知的视频时长,仅作状态体缺失
// video.duration 时的兜底计费依据。

// grokVideoFailureStatus 把状态体里的 VideoError.code 映射为用量日志状态码。

// settleGrokVideo 在状态查询首次观察到终态时为视频任务记一条结算用量:
//   - done:上游成本按生成秒数(或上游自报成本)计;用户按次/按秒计费只计已交付
//     (有产物 URL)的视频,审核拦截(respect_moderation=false)不向用户收费;
//   - failed / expired:不计费,只留一条失败记录说明原因。
//
// 提交行只代表"提交成功",金额为 0;客户端从不轮询的任务不会结算。
