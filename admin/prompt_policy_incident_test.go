package admin

import (
	"crypto/sha256"
	"encoding/hex"
)

func adminPolicyFingerprint(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// 管理员主动关闭审查/会话锁属于配置选择,不应报 degraded。

// 审查启用但 key 池为空才是真实故障。
