package data

import (
	"context"
	"math"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

// TestToolRepo_ToolInvocations_WorkspaceScoping covers A2 (tools 端到端审计
// 2026-10-19)：tool_invocations 工作区化。
//  1. 写入方未提供 WorkspaceID 时 data 层自 agents 表按 AgentID 派生；
//  2. 显式提供的 WorkspaceID 原样落库（AgentID 为空时不派生）；
//  3. KPI 24h 聚合按 q.WorkspaceID 过滤：tenant 只见本租户行，'' = 系统全局视图。
func TestToolRepo_ToolInvocations_WorkspaceScoping(t *testing.T) {
	ctx := context.Background()
	repo, d := newToolTestRepo(t)

	// 种两个租户 Agent（ent 直建，绕开 biz 层）。
	for _, a := range []struct{ id, key, ws string }{
		{"ag-ws-a", "key-ws-a", "ws-a"},
		{"ag-ws-b", "key-ws-b", "ws-b"},
	} {
		if err := d.entClient.Agent.Create().
			SetID(a.id).
			SetAgentKey(a.key).
			SetDisplayName("ws probe " + a.id).
			SetProvider("fake").
			SetModel("fake").
			SetWorkspaceID(a.ws).
			Exec(ctx); err != nil {
			t.Fatalf("seed agent %s: %v", a.id, err)
		}
	}

	now := time.Now().UTC()
	mk := func(toolKey, callID, agentID, ws, status string) biz.ToolInvocationWrite {
		return biz.ToolInvocationWrite{
			ToolKey: toolKey, AgentID: agentID, WorkspaceID: ws,
			Status: status, ToolCallID: callID,
			StartedAt: now.Add(-time.Hour).Format(time.RFC3339),
			EndedAt:   now.Format(time.RFC3339),
			Source:    biz.ToolInvocationSourceRuntime,
		}
	}
	for _, w := range []biz.ToolInvocationWrite{
		mk("tool_a", "tc-a-1", "ag-ws-a", "", "success"),
		mk("tool_a", "tc-a-2", "ag-ws-a", "", "blocked"),
		mk("tool_b", "tc-b-1", "ag-ws-b", "", "failed"),
		mk("tool_x", "tc-x-1", "", "ws-explicit", "success"),
	} {
		if err := repo.RecordToolInvocation(ctx, w); err != nil {
			t.Fatalf("RecordToolInvocation %s: %v", w.ToolCallID, err)
		}
	}

	// 1) 未提供 WorkspaceID 的行按 agents.workspace_id 派生。
	var wsA string
	if err := d.rawDB.QueryRowContext(ctx,
		`SELECT workspace_id FROM tool_invocations WHERE id = 'tinv-tc-a-1'`).Scan(&wsA); err != nil {
		t.Fatalf("select derived workspace_id: %v", err)
	}
	if wsA != "ws-a" {
		t.Fatalf("derived workspace_id = %q, want ws-a", wsA)
	}

	// 2) 显式提供的 WorkspaceID 原样落库，无 AgentID 不触发派生。
	var wsX string
	if err := d.rawDB.QueryRowContext(ctx,
		`SELECT workspace_id FROM tool_invocations WHERE id = 'tinv-tc-x-1'`).Scan(&wsX); err != nil {
		t.Fatalf("select explicit workspace_id: %v", err)
	}
	if wsX != "ws-explicit" {
		t.Fatalf("explicit workspace_id = %q, want ws-explicit", wsX)
	}

	// 3) KPI 24h 按租户过滤。
	summaryOf := func(ws string) biz.ToolSummary {
		res, err := repo.SearchTools(ctx, biz.ToolListQuery{WorkspaceID: ws, Limit: 10})
		if err != nil {
			t.Fatalf("SearchTools(ws=%q): %v", ws, err)
		}
		return res.Summary
	}
	sA := summaryOf("ws-a")
	if sA.Calls24h != 2 {
		t.Fatalf("ws-a Calls24h = %d, want 2", sA.Calls24h)
	}
	if math.Abs(sA.FailureRate24h-0.5) > 1e-9 { // blocked 计入失败口径（1/2）
		t.Fatalf("ws-a FailureRate24h = %v, want 0.5", sA.FailureRate24h)
	}
	sB := summaryOf("ws-b")
	if sB.Calls24h != 1 || math.Abs(sB.FailureRate24h-1.0) > 1e-9 {
		t.Fatalf("ws-b summary = %+v, want Calls24h=1 FailureRate24h=1", sB)
	}
	sSys := summaryOf("")
	if sSys.Calls24h != 4 { // 系统视图无过滤（含 ws-explicit 行）
		t.Fatalf("system Calls24h = %d, want 4", sSys.Calls24h)
	}
}
