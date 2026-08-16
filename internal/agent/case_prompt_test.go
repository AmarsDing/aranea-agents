package agent

// P3 M3: Agent Case 召回注入测试。Case 块并入既有 memory cue 管线
// （recallParts），与 L2/L3/L4 并列，复用统一预算截断与末尾追加。

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

type fakeCaseRecaller struct {
	cases []biz.AgentCase
	err   error
	// 记录调用参数，验证 query 透传。
	gotQuery   string
	gotAgentID string
	gotLimit   int
}

func (f *fakeCaseRecaller) RecallAgentCases(_ context.Context, agentID, query string, limit int) ([]biz.AgentCase, error) {
	f.gotAgentID, f.gotQuery, f.gotLimit = agentID, query, limit
	return f.cases, f.err
}

// 正常路径：success 案例渲染 目标+做法，failure 案例渲染 目标+教训。
func TestCaseMemoryCue_FormatsOutcomes(t *testing.T) {
	recaller := &fakeCaseRecaller{cases: []biz.AgentCase{
		{Goal: "批量导入用户数据", Approach: "先小批量试跑再分批提交", Outcome: biz.AgentCaseOutcomeSuccess},
		{Goal: "修复缓存穿透", Pitfalls: "直接全量刷新导致雪崩", Outcome: biz.AgentCaseOutcomeFailure},
	}}
	cue := CaseMemoryCue(context.Background(), recaller, "ag-1", "导入数据")
	if cue == "" {
		t.Fatal("expected non-empty cue")
	}
	if !strings.Contains(cue, "批量导入用户数据") || !strings.Contains(cue, "先小批量试跑") {
		t.Fatalf("success case must render goal+approach, got %q", cue)
	}
	if !strings.Contains(cue, "修复缓存穿透") || !strings.Contains(cue, "直接全量刷新导致雪崩") {
		t.Fatalf("failure case must render goal+pitfalls, got %q", cue)
	}
	if !strings.Contains(cue, "SUCCESS") || !strings.Contains(cue, "FAILURE") {
		t.Fatalf("outcome markers missing, got %q", cue)
	}
}

// query 透传：召回关键词来自 RecallKeywordFromMessages（intent 优先，否则末条 user）。
func TestCaseMemoryCue_PassesQueryAndAgent(t *testing.T) {
	recaller := &fakeCaseRecaller{cases: []biz.AgentCase{
		{Goal: "g", Approach: "a", Outcome: biz.AgentCaseOutcomeSuccess},
	}}
	CaseMemoryCue(context.Background(), recaller, "ag-42", "接口超时")
	if recaller.gotAgentID != "ag-42" || recaller.gotQuery != "接口超时" {
		t.Fatalf("recaller got agent=%q query=%q", recaller.gotAgentID, recaller.gotQuery)
	}
	if recaller.gotLimit <= 0 || recaller.gotLimit > 5 {
		t.Fatalf("recall limit must be a small positive cap, got %d", recaller.gotLimit)
	}
}

// 降级：nil recaller / 召回错误 / 空结果 / 空 agentID 均返回 ""，绝不能阻断 turn。
func TestCaseMemoryCue_DegradesToEmpty(t *testing.T) {
	if cue := CaseMemoryCue(context.Background(), nil, "ag-1", "q"); cue != "" {
		t.Fatalf("nil recaller must yield empty cue, got %q", cue)
	}
	if cue := CaseMemoryCue(context.Background(), &fakeCaseRecaller{err: errors.New("db down")}, "ag-1", "q"); cue != "" {
		t.Fatalf("recall error must yield empty cue, got %q", cue)
	}
	if cue := CaseMemoryCue(context.Background(), &fakeCaseRecaller{}, "ag-1", "q"); cue != "" {
		t.Fatalf("no cases must yield empty cue, got %q", cue)
	}
	if cue := CaseMemoryCue(context.Background(), &fakeCaseRecaller{cases: []biz.AgentCase{{Goal: "g"}}}, "  ", "q"); cue != "" {
		t.Fatalf("blank agentID must yield empty cue, got %q", cue)
	}
	// 无实质内容的 case（goal 为空）不渲染。
	if cue := CaseMemoryCue(context.Background(), &fakeCaseRecaller{cases: []biz.AgentCase{{Outcome: "success"}}}, "ag-1", "q"); cue != "" {
		t.Fatalf("goal-less case must be skipped, got %q", cue)
	}
}

// 超长做法/教训按行截断，防止单条 case 吃掉整个记忆预算。
func TestCaseMemoryCue_TruncatesLongFields(t *testing.T) {
	long := strings.Repeat("很", 500)
	recaller := &fakeCaseRecaller{cases: []biz.AgentCase{
		{Goal: "g", Approach: long, Outcome: biz.AgentCaseOutcomeSuccess},
	}}
	cue := CaseMemoryCue(context.Background(), recaller, "ag-1", "q")
	if cue == "" {
		t.Fatal("expected non-empty cue")
	}
	if strings.Contains(cue, long) {
		t.Fatal("approach must be truncated")
	}
	if !strings.Contains(cue, "…") {
		t.Fatal("truncation marker missing")
	}
}

// 集成：buildRuntimeMemoryCue 把 case 块并入 recallParts（L2/L3 之后、L4 之前）。
func TestBuildRuntimeMemoryCue_IncludesCaseBlock(t *testing.T) {
	ag := biz.Agent{ID: "ag-1", Settings: &biz.AgentRuntimeSettings{MemoryEnabled: true}}
	deps := TRPCBuilderDeps{TRPCMemoryKnowledgeDeps: TRPCMemoryKnowledgeDeps{
		AgentCaseRecaller: &fakeCaseRecaller{cases: []biz.AgentCase{
			{Goal: "定位接口超时", Approach: "先查慢查询日志", Outcome: biz.AgentCaseOutcomeSuccess},
		}},
	}}
	inv := &trpcagent.Invocation{Session: &trpcsession.Session{ID: "s1", UserID: "u1"}}
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)
	result, _ := buildRuntimeMemoryCue(ctx, deps, ag, nil)
	if result.IsEmpty() {
		t.Fatal("case block must make the cue non-empty")
	}
	joined := result.JoinCues()
	if !strings.Contains(joined, "定位接口超时") {
		t.Fatalf("joined cue must contain case goal, got %q", joined)
	}
}
