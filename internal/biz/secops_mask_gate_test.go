package biz

// D6 L-2 脱敏用例测试：规则热副本 TTL、冷缓存 fail-closed 信号、严格审计
// （wb F3 审计失败即拒）。

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

type maskFakeRuleRepo struct {
	rules []MaskRule
	err   error
	calls int
}

func (r *maskFakeRuleRepo) ListEnabledMaskRules(context.Context) ([]MaskRule, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	return r.rules, nil
}

type maskFakeAudit struct {
	entries []AuditLog
	err     error
}

func (a *maskFakeAudit) InsertAuditLog(_ context.Context, entry AuditLog) error {
	if a.err != nil {
		return a.err
	}
	a.entries = append(a.entries, entry)
	return nil
}

func maskTestClock(t *testing.T, uc *SecopsMaskUsecase) *time.Time {
	t.Helper()
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	uc.now = func() time.Time { return now }
	return &now
}

func TestSecopsMaskUsecase_EngineTTLCache(t *testing.T) {
	repo := &maskFakeRuleRepo{rules: []MaskRule{maskTestRule("mr_02_phone", "手机号", `\b1[3-9]\d{9}\b`, MaskOperatorPlaceholder)}}
	uc := NewSecopsMaskUsecase(repo, loggateway.NewNoop())
	now := maskTestClock(t, uc)

	eng1 := uc.Engine(context.Background())
	if eng1 == nil || eng1.RuleCount() != 1 {
		t.Fatalf("engine = %v, want 1 rule", eng1)
	}
	if repo.calls != 1 {
		t.Fatalf("repo calls = %d, want 1", repo.calls)
	}
	_ = uc.Engine(context.Background())
	if repo.calls != 1 {
		t.Fatalf("TTL 内必须打缓存不重复读库，calls = %d", repo.calls)
	}

	// 规则变更 + TTL 过期 → 热副本刷新。
	repo.rules = append(repo.rules, maskTestRule("mr_04_email", "邮箱", `\w+@\w+`, MaskOperatorPlaceholder))
	*now = now.Add(secopsMaskRulesTTL + time.Second)
	eng2 := uc.Engine(context.Background())
	if eng2 == nil || eng2.RuleCount() != 2 {
		t.Fatalf("TTL 过期后应刷新到 2 条规则，got %v", eng2)
	}
	if repo.calls != 2 {
		t.Fatalf("repo calls = %d, want 2", repo.calls)
	}
}

func TestSecopsMaskUsecase_ColdErrorYieldsNilEngine(t *testing.T) {
	repo := &maskFakeRuleRepo{err: errors.New("db down")}
	uc := NewSecopsMaskUsecase(repo, loggateway.NewNoop())
	maskTestClock(t, uc)
	if eng := uc.Engine(context.Background()); eng != nil {
		t.Fatal("冷缓存 + 读库失败必须返回 nil（调用方 fail-closed）")
	}
}

func TestSecopsMaskUsecase_WarmErrorKeepsStaleEngine(t *testing.T) {
	repo := &maskFakeRuleRepo{rules: []MaskRule{maskTestRule("mr_02_phone", "手机号", `\b1[3-9]\d{9}\b`, MaskOperatorPlaceholder)}}
	uc := NewSecopsMaskUsecase(repo, loggateway.NewNoop())
	now := maskTestClock(t, uc)
	first := uc.Engine(context.Background())
	if first == nil {
		t.Fatal("priming engine failed")
	}
	repo.err = errors.New("db down")
	*now = now.Add(secopsMaskRulesTTL + time.Second)
	if got := uc.Engine(context.Background()); got != first {
		t.Fatal("有上次好快照时读库失败必须沿用旧引擎（旧规则仍可执行）")
	}
}

func TestSecopsMaskUsecase_ColdCompileErrorYieldsNil(t *testing.T) {
	repo := &maskFakeRuleRepo{rules: []MaskRule{maskTestRule("bad", "X", `[unclosed`, MaskOperatorPlaceholder)}}
	uc := NewSecopsMaskUsecase(repo, loggateway.NewNoop())
	maskTestClock(t, uc)
	if eng := uc.Engine(context.Background()); eng != nil {
		t.Fatal("规则编译失败且冷缓存必须返回 nil")
	}
}

func TestNewSecopsMaskUsecase_NilRepo(t *testing.T) {
	if uc := NewSecopsMaskUsecase(nil, loggateway.NewNoop()); uc != nil {
		t.Fatal("nil repo 必须返回 nil（wire 据此停用 L-2 钩子）")
	}
	var uc *SecopsMaskUsecase
	if eng := uc.Engine(context.Background()); eng != nil {
		t.Fatal("nil usecase Engine 必须安全返回 nil")
	}
	if err := uc.RecordMaskAudit(context.Background(), "x", "s", nil, "info"); err != nil {
		t.Fatal("nil usecase 审计必须安全 no-op")
	}
}

func TestSecopsMaskUsecase_RecordMaskAuditStrict(t *testing.T) {
	repo := &maskFakeRuleRepo{}
	uc := NewSecopsMaskUsecase(repo, loggateway.NewNoop())
	// 未注入审计口：no-op 不报错。
	if err := uc.RecordMaskAudit(context.Background(), SecopsMaskActionCloudSend, "sess-1", map[string]any{"messages": 1}, "info"); err != nil {
		t.Fatalf("nil audit recorder must be a no-op, got %v", err)
	}

	rec := &maskFakeAudit{}
	uc.SetAuditRecorder(rec)
	if err := uc.RecordMaskAudit(context.Background(), SecopsMaskActionReject, "sess-1", map[string]any{"findings": 2}, "warning"); err != nil {
		t.Fatalf("audit write: %v", err)
	}
	if len(rec.entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(rec.entries))
	}
	if got := rec.entries[0].Action; got != SecopsMaskActionReject {
		t.Fatalf("action = %q", got)
	}
	if !strings.Contains(rec.entries[0].Detail, `"findings":2`) {
		t.Fatalf("detail = %q, want findings count only", rec.entries[0].Detail)
	}

	// 严格语义：审计写失败必须向上抛（wb F3 审计失败即拒）。
	rec.err = errors.New("audit down")
	if err := uc.RecordMaskAudit(context.Background(), SecopsMaskActionCloudSend, "s", nil, "info"); err == nil {
		t.Fatal("audit failure must propagate (strict)")
	}
}
