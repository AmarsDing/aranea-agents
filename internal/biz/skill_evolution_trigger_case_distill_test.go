package biz

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// ── P3 M4: CaseDistillTrigger（case→skill 蒸馏触发器）─────────────────────

type fakeCaseRecallSource struct {
	cases []AgentCase
	err   error
}

func (f *fakeCaseRecallSource) RecallAgentCases(_ context.Context, _, _ string, _ int) ([]AgentCase, error) {
	return f.cases, f.err
}

type fakeCaseDistiller struct {
	name, body string
	err        error
	gotCases   []AgentCase
}

func (f *fakeCaseDistiller) DistillSkillFromCases(_ context.Context, _ string, cases []AgentCase) (string, string, error) {
	f.gotCases = cases
	return f.name, f.body, f.err
}

type fakeEvoSettings struct {
	evolve bool
	err    error
}

func (f *fakeEvoSettings) GetAgentRuntimeSettings(_ context.Context, _ string) (AgentRuntimeSettings, error) {
	return AgentRuntimeSettings{EvolutionSkillEvolve: f.evolve}, f.err
}

func makeCases(n int) []AgentCase {
	out := make([]AgentCase, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, AgentCase{
			ID: string(rune('a' + i)), AgentID: "ag-1",
			Goal: "任务", Approach: "做法", Outcome: AgentCaseOutcomeSuccess, Quality: 0.9,
		})
	}
	return out
}

func TestCaseDistillTrigger_BelowThresholdSkips(t *testing.T) {
	tr := NewCaseDistillTrigger(
		&fakeEvoSettings{evolve: true},
		&fakeCaseRecallSource{cases: makeCases(caseDistillMinCases - 1)},
		&fakeCaseDistiller{name: "x", body: "y"},
		loggateway.NewNoop(),
	)
	got, err := tr.Check(context.Background(), "ag-1")
	if err != nil || len(got) != 0 {
		t.Fatalf("below threshold must skip, got %v %v", got, err)
	}
}

func TestCaseDistillTrigger_OptOutSkips(t *testing.T) {
	distiller := &fakeCaseDistiller{name: "x", body: "y"}
	tr := NewCaseDistillTrigger(
		&fakeEvoSettings{evolve: false},
		&fakeCaseRecallSource{cases: makeCases(caseDistillMinCases + 2)},
		distiller,
		loggateway.NewNoop(),
	)
	got, err := tr.Check(context.Background(), "ag-1")
	if err != nil || len(got) != 0 {
		t.Fatalf("opt-out must skip, got %v %v", got, err)
	}
	if distiller.gotCases != nil {
		t.Fatal("opt-out must not call the LLM distiller")
	}
}

func TestCaseDistillTrigger_DistillFailureSkips(t *testing.T) {
	tr := NewCaseDistillTrigger(
		&fakeEvoSettings{evolve: true},
		&fakeCaseRecallSource{cases: makeCases(caseDistillMinCases + 1)},
		&fakeCaseDistiller{err: errors.New("llm down")},
		loggateway.NewNoop(),
	)
	got, err := tr.Check(context.Background(), "ag-1")
	// LLM 失败是 best-effort 跳过，不向 orchestrator 传播错误（避免刷屏 Warn）。
	if err != nil || len(got) != 0 {
		t.Fatalf("distill failure must skip silently, got %v %v", got, err)
	}
}

func TestCaseDistillTrigger_ProducesSuggestion(t *testing.T) {
	recaller := &fakeCaseRecallSource{cases: makeCases(caseDistillMinCases + 1)}
	tr := NewCaseDistillTrigger(
		&fakeEvoSettings{evolve: true},
		recaller,
		&fakeCaseDistiller{name: "batch-import", body: "# 批量导入\n步骤…"},
		loggateway.NewNoop(),
	)
	if tr.TargetType() != EvolutionTargetAgent || tr.ActionType() != EvolutionActionCreate {
		t.Fatalf("must be agent/create_skill, got %s/%s", tr.TargetType(), tr.ActionType())
	}
	if tr.TriggerSource() != "agent_case_distill" {
		t.Fatalf("trigger source, got %s", tr.TriggerSource())
	}
	got, err := tr.Check(context.Background(), "ag-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 suggestion, got %d", len(got))
	}
	s := got[0]
	if s.TargetID != "ag-1" || s.Status != "pending" || s.DraftName != "batch-import" || s.DraftBody == "" {
		t.Fatalf("suggestion fields wrong: %+v", s)
	}
	// Metadata 必须记录 source_case_ids 供审计追溯。
	ids := s.MetaRaw(EvoMetaSourceCaseIDs)
	if ids == nil {
		t.Fatal("metadata must carry source_case_ids")
	}
}

func TestCaseDistillTrigger_RecallErrorPropagates(t *testing.T) {
	tr := NewCaseDistillTrigger(
		&fakeEvoSettings{evolve: true},
		&fakeCaseRecallSource{err: errors.New("db down")},
		&fakeCaseDistiller{name: "x", body: "y"},
		loggateway.NewNoop(),
	)
	// DB 读失败不同于 LLM 失败：返回 error 让 orchestrator 记 Warn（K2）。
	if _, err := tr.Check(context.Background(), "ag-1"); err == nil {
		t.Fatal("recall error must propagate")
	}
}

func TestCaseDistillTrigger_NilDepsNoop(t *testing.T) {
	tr := NewCaseDistillTrigger(nil, nil, nil, loggateway.NewNoop())
	got, err := tr.Check(context.Background(), "ag-1")
	if err != nil || got != nil {
		t.Fatalf("nil deps must be no-op, got %v %v", got, err)
	}
}
