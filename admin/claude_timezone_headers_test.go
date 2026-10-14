package admin

// A Claude account whose stored identity headers use Go's canonical casing
// ("X-Stainless-Os") must survive a same-timezone save: the fresh fingerprint
// is keyed "X-Stainless-OS", and a case-insensitive clash with a different
// random OS used to fail the whole update with
// "custom_headers 包含大小写重复且值冲突的请求头: X-Stainless-Os".

// the generated OS is random; every save must succeed
