package admin

import (
	"testing"
	"time"

	"github.com/codex2api/auth"
)

func rateLimitedProbeAccount(id int64, now time.Time) *auth.Account {
	return &auth.Account{
		DBID:           id,
		AccessToken:    "token",
		Status:         auth.StatusCooldown,
		CooldownUtil:   now.Add(time.Hour),
		CooldownReason: "rate_limited",
	}
}

// 首轮：没有计时记录，三个账号都要刷。

// 半小时后：正常账号未到 1h，限流账号未到 6h，都不刷。

// 一小时后：正常账号到期，限流账号继续等。

// 六小时后：限流账号也到期了。

// 冷却结束恢复后，按正常 1h 间隔判定，而不是继续背着 6h。

func TestWhamDailyUsageAutoRefreshEligibleSkipsErrorBannedAndNewAccounts(t *testing.T) {
	now := time.Now()
	ready := &auth.Account{DBID: 1, AccessToken: "at"}
	ready.AddedAt.Store(now.Add(-25 * time.Hour).UnixNano())
	if !whamDailyUsageAutoRefreshEligible(ready, now) {
		t.Fatal("account older than one day should auto-refresh")
	}

	fresh := &auth.Account{DBID: 2, AccessToken: "at"}
	fresh.AddedAt.Store(now.Add(-2 * time.Hour).UnixNano())
	if whamDailyUsageAutoRefreshEligible(fresh, now) {
		t.Fatal("account imported within one day should wait for official settlement")
	}

	errored := &auth.Account{DBID: 3, AccessToken: "at", Status: auth.StatusError}
	errored.AddedAt.Store(now.Add(-48 * time.Hour).UnixNano())
	if whamDailyUsageAutoRefreshEligible(errored, now) {
		t.Fatal("error account should not auto-refresh official usage")
	}

	banned := &auth.Account{
		DBID: 4, AccessToken: "at", Status: auth.StatusCooldown,
		CooldownUtil: now.Add(time.Hour), CooldownReason: "unauthorized",
	}
	banned.AddedAt.Store(now.Add(-48 * time.Hour).UnixNano())

	if whamDailyUsageAutoRefreshEligible(banned, now) {
		t.Fatal("banned account should not auto-refresh official usage")
	}
}

func TestWhamDailyUsageBackfillEligibleSkipsRelayGrokAndCodexAT(t *testing.T) {
	if !whamDailyUsageBackfillEligible(&auth.Account{DBID: 1, AccessToken: "at"}) {
		t.Fatal("codex oauth account should be eligible")
	}
	if whamDailyUsageBackfillEligible(&auth.Account{DBID: 2}) {
		t.Fatal("missing access token should be skipped")
	}
	grok := &auth.Account{DBID: 3, AccessToken: "at", UpstreamType: auth.UpstreamGrok, APIKey: "gk"}
	if !grok.IsGrokAPI() {
		t.Fatal("test grok account is not classified as grok")
	}
	if whamDailyUsageBackfillEligible(grok) {
		t.Fatal("grok account should be skipped")
	}
	if whamDailyUsageBackfillEligible(&auth.Account{DBID: 4, AccessToken: "at-opaque"}) {
		t.Fatal("codex_at account should be skipped")
	}
	claude := &auth.Account{DBID: 5, AccessToken: "claude-token", RefreshToken: "claude-refresh", UpstreamType: auth.UpstreamClaude}
	if whamDailyUsageBackfillEligible(claude) {
		t.Fatal("Claude account must not use the ChatGPT WHAM daily usage endpoint")
	}
	antigravity := &auth.Account{DBID: 6, AccessToken: "ya29.google-token", RefreshToken: "google-refresh", UpstreamType: auth.UpstreamAntigravity}
	if !antigravity.IsAntigravityAPI() {
		t.Fatal("test antigravity account is not classified as antigravity")
	}
	if whamDailyUsageBackfillEligible(antigravity) {
		t.Fatal("Antigravity (Google) account must not use the ChatGPT WHAM daily usage endpoint")
	}
	if whamDailyUsageChannelSupported(antigravity) || !whamDailyUsageChannelSupported(&auth.Account{DBID: 7, AccessToken: "at"}) {
		t.Fatal("channel gate must reject antigravity and accept codex oauth")
	}
}
