package admin

// 短 Key 曾因前 3 位与后 4 位重叠被整串还原(如 "hunter2"→"hun••••ter2"),
// 多字节 Key 曾被按字节切坏;掩码只允许在长度足够时露出前 3 + 后 4。

// The administrative test remains usable before activation.
