package service

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

// D6（97-multi-replica-executor-waiters）：capability flag 档位解析钉板。
// 铁律：非法值一律按缺省 off（fail-safe=不认得档=旧行为），非布尔语义
//（"1"/"true"/"on" 不是合法档）。
func TestResolveExecutorWaiterMode(t *testing.T) {
	t.Parallel()
	env := func(v string) func(string) string {
		return func(string) string { return v }
	}
	cases := []struct {
		name string
		val  string
		want string
	}{
		{"unset", "", ExecutorWaiterModeOff},
		{"mem", "mem", ExecutorWaiterModeMem},
		{"mem 大小写与空白容错", " MEM ", ExecutorWaiterModeMem},
		{"pg", "pg", ExecutorWaiterModePG},
		{"pg 大写", "PG", ExecutorWaiterModePG},
		{"off 显式", "off", ExecutorWaiterModeOff},
		{"非法值回落 off", "bogus", ExecutorWaiterModeOff},
		{"布尔风格非法", "true", ExecutorWaiterModeOff},
		{"数字风格非法", "1", ExecutorWaiterModeOff},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveExecutorWaiterMode(env(tc.val)); got != tc.want {
				t.Fatalf("ResolveExecutorWaiterMode(%q)=%q, want %q", tc.val, got, tc.want)
			}
		})
	}
	// nil getenv 回落 os.Getenv（本机/CI 均未设置该新 flag → off）。
	if got := ResolveExecutorWaiterMode(nil); got != ExecutorWaiterModeOff {
		t.Fatalf("nil getenv with unset env must resolve off, got %q", got)
	}
}

// D6 CAS 铁律钉板：先到者赢，0 行命中绝不翻案；键空间 (kind,session,scope)
// 任一不同即不同决议；写入期校验 kind/scope 必填。
func TestInMemExecutorWaiterDecisions_FirstWriterWins(t *testing.T) {
	t.Parallel()
	s := NewInMemExecutorWaiterDecisions()
	ctx := context.Background()
	key := biz.ExecutorWaiterDecisionKey{
		Kind:      biz.WaiterDecisionKindPlaybookConfirm,
		SessionID: "sp-1",
		ScopeID:   "st-1",
	}

	if _, found, err := s.LoadDecision(ctx, key); err != nil || found {
		t.Fatalf("empty load: found=%v err=%v", found, err)
	}

	first := biz.ExecutorWaiterDecision{
		ExecutorWaiterDecisionKey: key,
		Approved:                  true,
		Reason:                    "approve",
		Actor:                     "replica-b",
		DecidedAt:                 time.Now().UTC(),
	}
	recorded, err := s.TryRecordDecision(ctx, first)
	if err != nil || !recorded {
		t.Fatalf("first write: recorded=%v err=%v", recorded, err)
	}

	// 同 key 二次写（冲突值）绝不翻案——recorded=false 且现值保持先到者。
	second := first
	second.Approved = false
	second.Actor = "replica-a"
	recorded, err = s.TryRecordDecision(ctx, second)
	if err != nil || recorded {
		t.Fatalf("second write must lose CAS: recorded=%v err=%v", recorded, err)
	}
	dec, found, err := s.LoadDecision(ctx, key)
	if err != nil || !found {
		t.Fatalf("load after conflict: found=%v err=%v", found, err)
	}
	if !dec.Approved || dec.Actor != "replica-b" {
		t.Fatalf("first-writer-wins violated: approved=%v actor=%q", dec.Approved, dec.Actor)
	}

	// 键空间隔离：scope 不同即另一条决议。
	other := biz.ExecutorWaiterDecision{
		ExecutorWaiterDecisionKey: biz.ExecutorWaiterDecisionKey{
			Kind:      biz.WaiterDecisionKindPlaybookConfirm,
			SessionID: "sp-1",
			ScopeID:   "st-2",
		},
		Approved:  false,
		DecidedAt: time.Now().UTC(),
	}
	if recorded, err := s.TryRecordDecision(ctx, other); err != nil || !recorded {
		t.Fatalf("distinct scope must record: recorded=%v err=%v", recorded, err)
	}

	// 写入期校验：kind 必填。
	if _, err := s.TryRecordDecision(ctx, biz.ExecutorWaiterDecision{
		ExecutorWaiterDecisionKey: biz.ExecutorWaiterDecisionKey{Kind: "", ScopeID: "x"},
	}); err == nil {
		t.Fatal("missing kind must be rejected")
	}
	// 写入期校验：scope 必填。
	if _, err := s.TryRecordDecision(ctx, biz.ExecutorWaiterDecision{
		ExecutorWaiterDecisionKey: biz.ExecutorWaiterDecisionKey{Kind: biz.WaiterDecisionKindBoardStartup},
	}); err == nil {
		t.Fatal("missing scope_id must be rejected")
	}
}
