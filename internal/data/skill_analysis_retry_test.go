package data

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// seedInvocation 直接经 ent 写一条 skill_invocation 行（RecordSkillInvocation
// 不在 biz.SkillRepo 接口上，测试只关心分析队列视图所需的最小列集）。
func seedInvocation(t *testing.T, d *Data, activationID, outcome, createdAt string) {
	t.Helper()
	ctx := context.Background()
	_, err := d.RW().Write(ctx).SkillInvocation.Create().
		SetID("inv-" + activationID).
		SetSkillID("skill-retry").
		SetStatus("failed").
		SetOutcome(outcome).
		SetActivationID(activationID).
		SetSource("runtime").
		SetCreatedAt(createdAt).
		SetUpdatedAt(createdAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed invocation %s: %v", activationID, err)
	}
}

// 审计 W5② 验收（数据层）：RecordAnalysisFailure 自增计数并落错误；attempts 达
// 上限的行被 ListUnanalyzed 死信隔离；空 activationID 空操作；未知行 NotFound。
//
// 说明：maxSkillAnalysisAttempts=5 是 package 常量——若日后调整，本测试的循环
// 次数跟随常量，无需改字面量。
func TestSkillIntelligenceRepo_AnalysisRetryDeadLetter(t *testing.T) {
	d := newTestDataPG(t)
	r := NewSkillIntelligenceRepo(d, loggateway.NewNoop())
	ctx := context.Background()

	// 毒行创建更早——正是它在老实现里永远占据队头的场景。
	seedInvocation(t, d, "act-poison", "failure", time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC).Format(time.RFC3339))
	seedInvocation(t, d, "act-healthy", "failure", time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC).Format(time.RFC3339))

	// 未满上限：两行都应在未分析批次中。
	got, err := r.ListUnanalyzed(ctx, 100)
	if err != nil {
		t.Fatalf("ListUnanalyzed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 unanalyzed rows, got %d", len(got))
	}

	// 对 act-poison 记满上限次失败。
	for i := 0; i < maxSkillAnalysisAttempts; i++ {
		if err := r.RecordAnalysisFailure(ctx, "act-poison", "boom"); err != nil {
			t.Fatalf("RecordAnalysisFailure #%d: %v", i, err)
		}
	}

	// 死信隔离：act-poison 出队，act-healthy 仍在。
	got, err = r.ListUnanalyzed(ctx, 100)
	if err != nil {
		t.Fatalf("ListUnanalyzed after dead-letter: %v", err)
	}
	if len(got) != 1 || got[0].ActivationID != "act-healthy" {
		t.Fatalf("expected only act-healthy, got %+v", got)
	}

	// 计数与错误落库校验（直接 SQL 读回）。
	var attempts int
	var lastErr string
	rows, err := d.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT analysis_attempts, analysis_error FROM skill_invocation WHERE activation_id = $1`, "act-poison")
	if err != nil {
		t.Fatalf("query poison row: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("poison row not found")
	}
	if err := rows.Scan(&attempts, &lastErr); err != nil {
		t.Fatalf("scan poison row: %v", err)
	}
	if attempts != maxSkillAnalysisAttempts {
		t.Fatalf("attempts = %d, want %d", attempts, maxSkillAnalysisAttempts)
	}
	if lastErr != "boom" {
		t.Fatalf("analysis_error = %q, want %q", lastErr, "boom")
	}

	// 边界：空 activationID → nil 空操作；未知 activationID → NotFound。
	if err := r.RecordAnalysisFailure(ctx, "", "x"); err != nil {
		t.Fatalf("empty activationID should be no-op, got %v", err)
	}
	err = r.RecordAnalysisFailure(ctx, "act-nonexistent", "x")
	var ae *apierror.Error
	if !errors.As(err, &ae) || ae.Code != apierror.CodeNotFound {
		t.Fatalf("expected NotFound for unknown activation, got %v", err)
	}
}
