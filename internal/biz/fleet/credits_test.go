package fleet

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDeepseekCredit(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"110.50","granted_balance":"10.00","topped_up_balance":"100.50"}]}`))
	}))
	defer srv.Close()
	old := deepseekBalanceURL
	deepseekBalanceURL = srv.URL
	defer func() { deepseekBalanceURL = old }()

	c := deepseekCredit(context.Background(), "sk-test")
	if gotAuth != "Bearer sk-test" {
		t.Fatalf("auth header = %q", gotAuth)
	}
	if c.Remaining != 110.5 || c.Unit != "CNY" || c.Note != "" || c.UpdatedAt == "" {
		t.Fatalf("unexpected credit %+v", c)
	}

	if c := deepseekCredit(context.Background(), ""); c.Remaining != -1 || c.Note == "" {
		t.Fatalf("missing key must be reported, got %+v", c)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(401) }))
	defer bad.Close()
	deepseekBalanceURL = bad.URL
	if c := deepseekCredit(context.Background(), "sk-bad"); c.Remaining != -1 || c.Note != "余额接口返回 401" {
		t.Fatalf("401 must be reported, got %+v", c)
	}
}

// 积分快照来自用量扫描：跨文件取最新一次；最新的会话文件还没有快照时，不能读成「没有」。
func TestCodexCreditFromUsageScan(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	ts := func(d time.Duration) string { return now.Add(d).UTC().Format(time.RFC3339) }
	older := `{"timestamp":"` + ts(-2*time.Hour) + `","payload":{"type":"token_count","info":{"last_token_usage":{"total_tokens":10}},"rate_limits":{"plan_type":"plus","credits":{"has_credits":true,"unlimited":false,"balance":"12.5"}}}}
{"timestamp":"` + ts(-time.Hour) + `","payload":{"type":"token_count","info":{"last_token_usage":{"total_tokens":10}},"rate_limits":{"plan_type":"plus","credits":{"has_credits":true,"unlimited":false,"balance":8}}}}
`
	// 刚开的新会话：有用量，没有 rate_limits
	newer := `{"timestamp":"` + ts(-time.Minute) + `","payload":{"type":"token_count","info":{"last_token_usage":{"total_tokens":5}}}}
`
	if err := os.WriteFile(filepath.Join(dir, "rollout-a.jsonl"), []byte(older), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rollout-b.jsonl"), []byte(newer), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(filepath.Join(dir, "rollout-a.jsonl"), now.Add(-time.Hour), now.Add(-time.Hour))

	r, ok := scanUsage(dir, now, parseCodexLine)
	if !ok || r.codexLimits == nil {
		t.Fatalf("snapshot from the older file must be kept, got %+v", r)
	}
	c := codexCreditFrom(*r.codexLimits, time.Unix(r.codexAt, 0))
	if c.Plan != "plus" || c.Remaining != 8 {
		t.Fatalf("want latest snapshot (plus, 8), got %+v", c)
	}

	var free codexRateLimits
	_ = json.Unmarshal([]byte(`{"plan_type":"free","credits":{"has_credits":false,"unlimited":false,"balance":null}}`), &free)
	if c := codexCreditFrom(free, time.Now()); c.Remaining != 0 || c.Plan != "free" {
		t.Fatalf("no credits must read as 0, got %+v", c)
	}
}

func TestSetCreditManualOnly(t *testing.T) {
	u := NewUsecase(newMemStore(), &fakeProjects{}, Options{}, nil)
	ctx := context.Background()
	// 预填自动部分的缓存：测试不能真的去调 DeepSeek 或读本机 Codex 日志
	creditMu.Lock()
	creditAuto, creditAutoAt = []PlatformCredit{}, time.Now()
	creditMu.Unlock()
	if _, err := u.SetCredit(ctx, PlatformCredit{Platform: "deepseek", Total: 10, Remaining: 5}); err == nil {
		t.Fatal("auto platform must not accept manual credits")
	}
	if _, err := u.SetCredit(ctx, PlatformCredit{Platform: "cursor", Total: 10, Remaining: 20}); err == nil {
		t.Fatal("remaining > total must be rejected")
	}
	if _, err := u.SetCredit(ctx, PlatformCredit{Platform: "cursor", Total: 10, Remaining: 5, Unit: "BTC"}); err == nil {
		t.Fatal("unknown unit must be rejected")
	}
	if _, err := u.SetCredit(ctx, PlatformCredit{Platform: "cursor", Total: 10, Remaining: 5, Note: strings.Repeat("长", 81)}); err == nil {
		t.Fatal("note over 80 chars must be rejected")
	}
	saved, err := u.SetCredit(ctx, PlatformCredit{Platform: "cursor", Plan: " Pro ", Total: 500, Remaining: 320, Unit: "credits"})
	if err != nil || saved.Plan != "Pro" || saved.UpdatedAt == "" {
		t.Fatalf("save failed: %+v %v", saved, err)
	}
	items, err := u.ListCredits(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var cursor, qoder PlatformCredit
	for _, c := range items {
		switch c.Platform {
		case "cursor":
			cursor = c
		case "qoder":
			qoder = c
		}
	}
	if cursor.Remaining != 320 || cursor.Source != "manual" {
		t.Fatalf("cursor entry not listed: %+v", cursor)
	}
	if qoder.Remaining != -1 || qoder.UpdatedAt != "" {
		t.Fatalf("unfilled platform must read unknown: %+v", qoder)
	}
}

// 快照：同一天覆盖、按日期升序返回、超过保留天数的读时删掉；剩余未知不记。
func TestCreditHistory(t *testing.T) {
	store := newMemStore()
	u := NewUsecase(store, &fakeProjects{}, Options{}, nil)
	ctx := context.Background()
	now := time.Now().UTC()
	day := func(d int) string { return now.AddDate(0, 0, d).Format("2006-01-02") }
	put := func(date string, v float64) {
		_ = putDoc(ctx, store, kindCreditSnap, "cursor:"+date, creditSnap{Platform: "cursor", CreditPoint: CreditPoint{Date: date, Remaining: v}})
	}
	put(day(-1), 300)
	put(day(-3), 450)
	put(day(-40), 999) // 超过 30 天，应被删
	u.recordCreditSnap(ctx, PlatformCredit{Platform: "cursor", Remaining: 200, UpdatedAt: "x"})
	u.recordCreditSnap(ctx, PlatformCredit{Platform: "cursor", Remaining: 180, UpdatedAt: "x"}) // 同一天覆盖
	u.recordCreditSnap(ctx, PlatformCredit{Platform: "qoder", Remaining: -1, UpdatedAt: "x"})   // 未知不记

	h := u.creditHistory(ctx, now)
	got := h["cursor"]
	if len(got) != 3 || got[0].Date != day(-3) || got[2].Remaining != 180 {
		t.Fatalf("want 3 ascending points ending at 180, got %+v", got)
	}
	if _, ok := h["qoder"]; ok {
		t.Fatal("unknown remaining must not be recorded")
	}
	if _, ok, _ := store.Get(ctx, kindCreditSnap, "cursor:"+day(-40)); ok {
		t.Fatal("snapshot older than the retention window must be deleted")
	}
}
