package admin

import (
	"time"
)

const automaticModelDiscoveryTimeout = 30 * time.Second

// 自动价格轮询先发现模型，价格同步即可包含本轮新增的型号。
// 发现失败不阻断已有模型的价格更新，具体失败同时写入同步状态。

// 套餐探测并行回调；通过日志报告，汇总使用最终结果避免共享切片竞态。
