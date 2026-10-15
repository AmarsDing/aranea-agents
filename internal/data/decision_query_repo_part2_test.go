package data

import (
	"aranea-agents/internal/biz/decision"
	"context"
	"testing"
)

// TestDecisionRepo_OutboxDeadLetter_PG covers t-dr-4 死信生命周期：
// enqueue → attempts 递增 → 触达 MaxOutboxAttempts 自动翻 dead 退出扫描；
// MarkOutboxDead 直接翻终态（poison 路径）；dead 行不再被 ListPendingOutbox
// 命中（head-of-line blocking 防线）。
func TestDecisionRepo_OutboxDeadLetter_PG(t *testing.T) {
	ctx := context.Background()
	wr, _ := newDecisionQueryTestRepos(t)
	rec := decision.Record{
		DecisionKey: "dk-dl-1", Category: decision.CategorySystemGuard,
		Scenario: "系统闸", Reasoning: "-", Outcome: "tripped",
		ActorType: decision.ActorSystem, ActorKey: "system:guard",
		RelatedEntities: []decision.EntityRef{},
		SourceRef:       decision.SourceRef{RunID: "run-dl"},
		CreatedAt:       "2026-08-27T00:00:00Z", UpdatedAt: "2026-08-27T00:00:00Z",
	}
	if err := wr.EnqueueOutbox(ctx, []decision.Record{rec}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	rows, err := wr.ListPendingOutbox(ctx, 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("pending after enqueue: len=%d err=%v, want 1", len(rows), err)
	}
	id := rows[0].ID

	for i := 1; i < decision.MaxOutboxAttempts; i++ {
		if err := wr.MarkOutboxAttempt(ctx, id, "boom"); err != nil {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	rows, err = wr.ListPendingOutbox(ctx, 10)
	if err != nil || len(rows) != 1 || rows[0].Attempts != decision.MaxOutboxAttempts-1 {
		t.Fatalf("before limit: rows=%+v err=%v", rows, err)
	}

	if err := wr.MarkOutboxAttempt(ctx, id, "boom final"); err != nil {
		t.Fatalf("final attempt: %v", err)
	}
	rows, err = wr.ListPendingOutbox(ctx, 10)
	if err != nil || len(rows) != 0 {
		t.Fatalf("dead row must leave pending scan: len=%d err=%v", len(rows), err)
	}

	rec2 := rec
	rec2.DecisionKey = "dk-dl-2"
	if err := wr.EnqueueOutbox(ctx, []decision.Record{rec2}); err != nil {
		t.Fatalf("enqueue 2: %v", err)
	}
	rows, err = wr.ListPendingOutbox(ctx, 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("pending 2: len=%d err=%v", len(rows), err)
	}
	if err := wr.MarkOutboxDead(ctx, []int64{rows[0].ID}, "poison: payload undecodable"); err != nil {
		t.Fatalf("mark dead: %v", err)
	}
	rows, err = wr.ListPendingOutbox(ctx, 10)
	if err != nil || len(rows) != 0 {
		t.Fatalf("poison dead row must leave pending scan: len=%d err=%v", len(rows), err)
	}
}

// TestDecisionQueryRepo_GetByKey_PG covers the single-record path including
// JSON column decode and the not-found nil contract.
func TestDecisionQueryRepo_GetByKey_PG(t *testing.T) {
	ctx := context.Background()
	wr, qr := newDecisionQueryTestRepos(t)
	seedDecisionRecords(t, wr)

	rec, err := qr.GetByKey(ctx, "dk-q-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rec == nil {
		t.Fatal("get dk-q-1 returned nil")
	}
	if rec.Category != decision.CategoryHITLApproval || rec.Outcome != "approved" {
		t.Fatalf("roundtrip mismatch: %+v", rec)
	}
	if rec.Confidence == nil || *rec.Confidence != 0.9 {
		t.Fatalf("confidence = %v", rec.Confidence)
	}
	if len(rec.RelatedEntities) != 1 || rec.RelatedEntities[0].Key != "gns3_fault_inject" {
		t.Fatalf("entities = %+v", rec.RelatedEntities)
	}
	if rec.SourceRef.ToolInvocationID != "tc-1" || rec.SourceRef.RunID != "run-1" {
		t.Fatalf("source_ref = %+v", rec.SourceRef)
	}
	if rec.Metadata["decision_reason"] != "policy_danger" {
		t.Fatalf("metadata = %+v", rec.Metadata)
	}

	rec, err = qr.GetByKey(ctx, "dk-nonexistent")
	if err != nil || rec != nil {
		t.Fatalf("miss: rec=%v err=%v, want nil/nil", rec, err)
	}
}
