package admin

// TestGrokBatchImportTimeoutScalesWithFiles 批量导入的整体超时必须跟文件数一起涨：
// 写死常量时每个文件分到的预算会被批量摊薄，5000 个文件只剩 12ms/个，
// 数据库稍慢就会中途超时、剩下的文件全部报 context deadline exceeded。

// TestGrokBatchImportTimeoutStaysBounded 上限存在的意义：一个超大请求不能无限期占住连接。

// TestGrokBatchImportTimeoutCoversMaxFiles 满额导入不能一进来就撞上封顶，
// 否则每个文件的预算又会被摊薄回去。

type grokBatchImportTestResponse struct {
	Total    int                   `json:"total"`
	Imported int                   `json:"imported"`
	Failed   int                   `json:"failed"`
	Items    []grokBatchImportItem `json:"items"`
}

// TestGrokBatchImportRevivesRecycledIdentity 覆盖 issue #602:删除过的 Grok 账号
// 仍占着凭据身份键,重新导入同一 refresh_token 不能报"已存在"死胡同,
// 必须把凭据合并回原账号并将其从回收站复活。

// 导入后会起后台探针刷新凭据。测试凭据打到真实 auth.x.ai 只会换回
// invalid_grant——永久性错误,探针会把账号写成 error,与下面复活后的状态
// 断言竞速(CI 上偶发 status="error")。把令牌端点指到本地自签 TLS 桩:
// 刷新只会得到证书校验/503 这类瞬时错误,探针不再改写账号状态,也不再
// 依赖外网。故意不替换 http.DefaultTransport——后台探针可能在 Cleanup
// 恢复它时仍在读,会被 -race 抓到。

// 模拟 UI 删除:软删进回收站并移出运行时池。身份键仍归该账号持有。
