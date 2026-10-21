package data_test

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// FR-11（89 批次3）：UpsertGraphNode 终态谓词守卫。
// sequencer 死信重放携带的陈旧 running 快照不得覆写已终态 GraphNode 行；
// 终态 incoming 保持单调覆写（宕后收账合法路径）。PG-gated：SetupTestPG
// 在隔离 schema 上跑 Ent auto-migration（graph_nodes_v2 为 Ent 表）。

func newFR11Repo(t *testing.T) biz.GraphNodeV2Repo {
	t.Helper()
	client, db := testhelper.SetupTestPG(t)
	d := &data.Data{}
	d.SetEntClientForTest(client, db, loggateway.NewNoop())
	return data.NewGraphNodeV2Repo(d, loggateway.NewNoop())
}

func fr11Node(id, label string, status biz.GraphNodeStatus) biz.GraphNode {
	return biz.GraphNode{
		ID:           id,
		GraphStageID: "gs-fr11",
		DagNodeID:    "dag-" + id,
		Label:        label,
		Status:       status,
	}
}

// 1. 行已终态（completed）+ 非终态 incoming（running，死信陈旧快照）→
// 守卫拦截：行保持 completed，返回现有行（幂等语义）。
func TestUpsertGraphNode_FR11_NonTerminalIncomingDoesNotClobberTerminal(t *testing.T) {
	repo := newFR11Repo(t)
	ctx := context.Background()

	if _, err := repo.CreateGraphNode(ctx, fr11Node("gn-fr11-t1", "orig", biz.GraphNodeStatusCompleted)); err != nil {
		t.Fatalf("seed create: %v", err)
	}

	got, err := repo.UpsertGraphNode(ctx, fr11Node("gn-fr11-t1", "stale-replay", biz.GraphNodeStatusRunning))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if got.Status != biz.GraphNodeStatusCompleted {
		t.Fatalf("FR-11: returned status = %q, want completed (guard must intercept stale running)", got.Status)
	}
	if got.Label != "orig" {
		t.Fatalf("FR-11: returned label = %q, want orig (existing row returned, not overwritten)", got.Label)
	}

	row, err := repo.GetGraphNode(ctx, "gn-fr11-t1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if row.Status != biz.GraphNodeStatusCompleted || row.Label != "orig" {
		t.Fatalf("FR-11: persisted row = (%q,%q), want (completed,orig)", row.Status, row.Label)
	}
}

// 2. 行已终态（completed）+ 终态 incoming（failed）→ 终态单调，合法覆写
// （死信重放宕后收账不设防）。
func TestUpsertGraphNode_FR11_TerminalIncomingOverwritesTerminal(t *testing.T) {
	repo := newFR11Repo(t)
	ctx := context.Background()

	if _, err := repo.CreateGraphNode(ctx, fr11Node("gn-fr11-t2", "orig", biz.GraphNodeStatusCompleted)); err != nil {
		t.Fatalf("seed create: %v", err)
	}

	got, err := repo.UpsertGraphNode(ctx, fr11Node("gn-fr11-t2", "failed-label", biz.GraphNodeStatusFailed))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if got.Status != biz.GraphNodeStatusFailed {
		t.Fatalf("FR-11: returned status = %q, want failed (terminal overwrite allowed)", got.Status)
	}

	row, err := repo.GetGraphNode(ctx, "gn-fr11-t2")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if row.Status != biz.GraphNodeStatusFailed || row.Label != "failed-label" {
		t.Fatalf("FR-11: persisted row = (%q,%q), want (failed,failed-label)", row.Status, row.Label)
	}
}

// 3. 行不存在 → Create 兜底（upsert 语义不变）。
func TestUpsertGraphNode_FR11_MissingRowCreates(t *testing.T) {
	repo := newFR11Repo(t)
	ctx := context.Background()

	got, err := repo.UpsertGraphNode(ctx, fr11Node("gn-fr11-t3", "fresh", biz.GraphNodeStatusRunning))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if got.Status != biz.GraphNodeStatusRunning || got.Label != "fresh" {
		t.Fatalf("FR-11: created row = (%q,%q), want (running,fresh)", got.Status, got.Label)
	}

	row, err := repo.GetGraphNode(ctx, "gn-fr11-t3")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if row.Status != biz.GraphNodeStatusRunning {
		t.Fatalf("FR-11: persisted status = %q, want running", row.Status)
	}
}

// 4. 行非终态（pending）+ 非终态 incoming（running）→ 谓词不影响正常推进，
// 状态与 label 照常更新。
func TestUpsertGraphNode_FR11_NonTerminalToNonTerminalUpdates(t *testing.T) {
	repo := newFR11Repo(t)
	ctx := context.Background()

	if _, err := repo.CreateGraphNode(ctx, fr11Node("gn-fr11-t4", "pending-label", biz.GraphNodeStatusPending)); err != nil {
		t.Fatalf("seed create: %v", err)
	}

	got, err := repo.UpsertGraphNode(ctx, fr11Node("gn-fr11-t4", "running-label", biz.GraphNodeStatusRunning))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if got.Status != biz.GraphNodeStatusRunning || got.Label != "running-label" {
		t.Fatalf("FR-11: returned = (%q,%q), want (running,running-label)", got.Status, got.Label)
	}

	row, err := repo.GetGraphNode(ctx, "gn-fr11-t4")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if row.Status != biz.GraphNodeStatusRunning || row.Label != "running-label" {
		t.Fatalf("FR-11: persisted row = (%q,%q), want (running,running-label)", row.Status, row.Label)
	}
}
