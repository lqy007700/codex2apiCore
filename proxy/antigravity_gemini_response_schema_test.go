package proxy

// $ref 里的 JSON Pointer 转义(~1 → /、~0 → ~、百分号编码)要解码后再查定义键,
// 否则 "A/B" 这类键永远命中不了,引用会被清成 {} 丢约束。
