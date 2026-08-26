package biz

import (
	"context"
	"encoding/json"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// ADR-79-V V3（2026-08-26）：VerificationReceipt 证据回执台账
// ---------------------------------------------------------------------------

func TestExecuteGateScoped_RecordsReceipt(t *testing.T) {
	inv := &stubAssertionInvoker{raw: json.RawMessage(`{"enabled":true}`)}
	e := NewVerificationGateExecutor(nil, nil, loggateway.NewNoop(), WithToolAssertionInvoker(inv))
	gate := newToolAssertionGate("enabled", "true")

	ok, reason, r1, err := e.ExecuteGateScoped(context.Background(), "team-1", gate, "", 0)
	if err != nil || !ok {
		t.Fatalf("expected pass, got ok=%v err=%v", ok, err)
	}
	if r1.Scope != "team-1" || r1.GateType != GateTypeToolAssertion || r1.Target == "" {
		t.Fatalf("receipt identity wrong: %+v", r1)
	}
	if !r1.Approved || r1.Reason != reason {
		t.Fatalf("receipt verdict mismatch: %+v vs reason=%q", r1, reason)
	}
	if r1.EvidenceHash == "" || r1.DecidedAt == 0 || r1.Invalidated {
		t.Fatalf("receipt fingerprint wrong: %+v", r1)
	}

	// 同 (scope, target) 新裁决取代旧回执（supersede）。
	inv.raw = json.RawMessage(`{"enabled":false}`)
	ok, _, r2, err := e.ExecuteGateScoped(context.Background(), "team-1", gate, "", 0)
	if err != nil || ok {
		t.Fatalf("expected rejection, got ok=%v err=%v", ok, err)
	}
	if r2.Approved || r2.EvidenceHash == r1.EvidenceHash {
		t.Fatalf("new verdict must supersede with a fresh fingerprint: r1=%+v r2=%+v", r1, r2)
	}
	if got := e.InvalidatedReceipts("team-1"); len(got) != 0 {
		t.Fatalf("no write recorded → nothing invalidated, got %+v", got)
	}
}

func TestExecuteGateScoped_ErrorPathRecordsNoReceipt(t *testing.T) {
	e := NewVerificationGateExecutor(nil, nil, loggateway.NewNoop(), WithToolAssertionInvoker(&stubAssertionInvoker{}))
	gate := newToolAssertionGate("enabled", "true")
	gate.Tool = "exec_command" // 非白名单 → 配置错误路径
	if _, _, _, err := e.ExecuteGateScoped(context.Background(), "team-err", gate, "", 0); err == nil {
		t.Fatal("expected configuration error")
	}
	// error 路径不记回执：写后误标也无回执可失效。
	if n := e.RecordScopeWrite("team-err", "deliverables_output"); n != 0 {
		t.Fatalf("error path must not record receipts, invalidated %d", n)
	}
}

func TestRecordScopeWrite_InvalidatesOnlySameScope(t *testing.T) {
	inv := &stubAssertionInvoker{raw: json.RawMessage(`{"enabled":true}`)}
	e := NewVerificationGateExecutor(nil, nil, loggateway.NewNoop(), WithToolAssertionInvoker(inv))
	gate := newToolAssertionGate("enabled", "true")
	ctx := context.Background()

	if _, _, _, err := e.ExecuteGateScoped(ctx, "team-a", gate, "", 0); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := e.ExecuteGateScoped(ctx, "team-b", gate, "", 0); err != nil {
		t.Fatal(err)
	}

	if n := e.RecordScopeWrite("team-a", "deliverables_output"); n != 1 {
		t.Fatalf("expected 1 invalidated receipt in team-a, got %d", n)
	}
	// 重复写不重复计数（已失效回执跳过）。
	if n := e.RecordScopeWrite("team-a", "deliverables_output"); n != 0 {
		t.Fatalf("invalidation must be idempotent, got %d", n)
	}
	if got := e.InvalidatedReceipts("team-a"); len(got) != 1 || !got[0].Invalidated {
		t.Fatalf("team-a receipt must stay invalidated: %+v", got)
	}
	if got := e.InvalidatedReceipts("team-b"); len(got) != 0 {
		t.Fatalf("team-b must be untouched, got %+v", got)
	}

	// 失效后对活证据重验 → 新回执取代失效回执（V3 重验语义）。
	if _, _, r, err := e.ExecuteGateScoped(ctx, "team-a", gate, "", 0); err != nil || r.Invalidated {
		t.Fatalf("revalidation must produce a fresh valid receipt, got r=%+v err=%v", r, err)
	}
	if got := e.InvalidatedReceipts("team-a"); len(got) != 0 {
		t.Fatalf("revalidated receipt must clear the invalidated set, got %+v", got)
	}
}

func TestGateReceiptEvidenceHash_BindsVerdictAndEvidence(t *testing.T) {
	target := gateReceiptTarget(newToolAssertionGate("enabled", "true"))
	h1 := gateReceiptEvidenceHash(target, "", true, "ok")
	h2 := gateReceiptEvidenceHash(target, "", false, "ok")
	h3 := gateReceiptEvidenceHash(target, "output-x", true, "ok")
	if h1 == h2 {
		t.Fatal("different verdicts on same evidence must produce different hashes")
	}
	if h1 == h3 {
		t.Fatal("different evidence must produce different hashes")
	}
	if h1 != gateReceiptEvidenceHash(target, "", true, "ok") {
		t.Fatal("hash must be deterministic")
	}
}
