package data

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	bizsession "aranea-agents/internal/biz/session"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// FR-13（89 批次3）：删除守卫三路径的 data 层谓词测试（PG-gated，Ent 表可承载）。
//
//	13a DeleteSessionsByAgentID：running/awaiting_confirmation 会话跳删并返回 skipped
//	13b cascadeDeleteByAgent：级联软删对活跃会话同谓词（纵深防御）
//	13c DeleteAgent：活跃会话计数 > 0 → 409 Conflict（含计数），检测与删除同事务

func insertFR13Session(t *testing.T, ctx context.Context, d *Data, id, agentID, status string) {
	t.Helper()
	_, err := d.RW().Write(ctx).Session.Create().
		SetID(id).
		SetAgentID(agentID).
		SetTitle(id).
		SetStatus(status).
		Save(ctx)
	if err != nil {
		t.Fatalf("create session %s: %v", id, err)
	}
}

func sessionDeletedAt(t *testing.T, ctx context.Context, d *Data, id string) string {
	t.Helper()
	row, err := d.RW().Read(ctx).Session.Get(ctx, id)
	if err != nil {
		t.Fatalf("get session %s: %v", id, err)
	}
	return row.DeletedAt
}

func TestDeleteSessionsByAgentID_FR13_GuardSkipsActiveSessions(t *testing.T) {
	client, db := testhelper.SetupTestPG(t)
	d := &Data{}
	d.SetEntClientForTest(client, db, loggateway.NewNoop())
	repo := NewSessionRepo(d, nil, nil)
	ctx := context.Background()

	const agentA, agentB = "fr13-agent-a", "fr13-agent-b"
	insertFR13Session(t, ctx, d, "fr13-s-running", agentA, string(bizsession.SessionStatusRunning))
	insertFR13Session(t, ctx, d, "fr13-s-awaiting", agentA, string(bizsession.SessionStatusAwaitingConfirmation))
	insertFR13Session(t, ctx, d, "fr13-s-plain", agentA, "active")
	insertFR13Session(t, ctx, d, "fr13-s-empty", agentA, "")
	insertFR13Session(t, ctx, d, "fr13-s-other-agent", agentB, string(bizsession.SessionStatusRunning))

	processed, skipped, err := repo.DeleteSessionsByAgentID(ctx, agentA)
	if err != nil {
		t.Fatalf("DeleteSessionsByAgentID: %v", err)
	}
	if processed != 2 {
		t.Fatalf("processed = %d, want 2 (plain+empty)", processed)
	}
	if len(skipped) != 2 {
		t.Fatalf("skipped = %v, want 2 entries", skipped)
	}
	seen := map[string]bool{}
	for _, id := range skipped {
		seen[id] = true
	}
	if !seen["fr13-s-running"] || !seen["fr13-s-awaiting"] {
		t.Fatalf("skipped missing active ids: %v", skipped)
	}
	if got := sessionDeletedAt(t, ctx, d, "fr13-s-plain"); got == "" {
		t.Error("plain session must be soft-deleted")
	}
	if got := sessionDeletedAt(t, ctx, d, "fr13-s-empty"); got == "" {
		t.Error("empty-status session must be soft-deleted")
	}
	if got := sessionDeletedAt(t, ctx, d, "fr13-s-running"); got != "" {
		t.Error("running session must survive")
	}
	if got := sessionDeletedAt(t, ctx, d, "fr13-s-awaiting"); got != "" {
		t.Error("awaiting_confirmation session must survive")
	}
	if got := sessionDeletedAt(t, ctx, d, "fr13-s-other-agent"); got != "" {
		t.Error("other agent's session must be untouched")
	}
}

func TestDeleteAgent_FR13_ConflictOnActiveSessions(t *testing.T) {
	client, db := testhelper.SetupTestPG(t)
	d := &Data{}
	d.SetEntClientForTest(client, db, loggateway.NewNoop())
	repo := NewAgentRepo(d)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	base := biz.Agent{
		AgentKey: "fr13_guard", DisplayName: "FR13 Guard",
		Status: "active", Kind: "ecosystem_preset", Source: "imported",
		CreatedAt: now, UpdatedAt: now,
	}

	// ── 409 路径：running + awaiting 两活跃会话，删除整体被拒、agent 原样 ──
	agBlock := base
	agBlock.ID = "fr13-agent-blocked"
	agBlock.AgentKey = "fr13_guard_blocked"
	if _, err := repo.CreateAgent(ctx, agBlock); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	insertFR13Session(t, ctx, d, "fr13-block-run", agBlock.ID, string(bizsession.SessionStatusRunning))
	insertFR13Session(t, ctx, d, "fr13-block-await", agBlock.ID, string(bizsession.SessionStatusAwaitingConfirmation))
	insertFR13Session(t, ctx, d, "fr13-block-plain", agBlock.ID, "active")

	err := repo.DeleteAgent(ctx, agBlock.ID)
	var apiErr *apierror.Error
	if err == nil || !errors.As(err, &apiErr) || apiErr.Code != apierror.CodeConflict {
		t.Fatalf("DeleteAgent with active sessions must return 409 Conflict, got %v", err)
	}
	still, getErr := repo.GetAgentByID(ctx, agBlock.ID)
	if getErr != nil {
		t.Fatalf("blocked delete must keep agent row: %v", getErr)
	}
	if still.DeletedAt != "" || still.Status == string(biz.AgentStatusArchived) {
		t.Fatalf("blocked delete must leave agent untouched, got deleted_at=%q status=%q", still.DeletedAt, still.Status)
	}
	if got := sessionDeletedAt(t, ctx, d, "fr13-block-plain"); got != "" {
		t.Error("blocked delete must not cascade any session")
	}

	// ── 放行路径：仅有非活跃会话 → 删除成功，级联保留跑着的（此时无）并软删其余 ──
	agOK := base
	agOK.ID = "fr13-agent-ok"
	agOK.AgentKey = "fr13_guard_ok"
	if _, err := repo.CreateAgent(ctx, agOK); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	insertFR13Session(t, ctx, d, "fr13-ok-plain", agOK.ID, "active")
	insertFR13Session(t, ctx, d, "fr13-ok-completed", agOK.ID, "completed")
	if err := repo.DeleteAgent(ctx, agOK.ID); err != nil {
		t.Fatalf("DeleteAgent without active sessions must succeed: %v", err)
	}
	// GetAgentByID 过滤 deleted_at≠''，软删后需走原生 SQL 核验 tombstone。
	var delAt, status string
	if err := db.QueryRowContext(ctx, `SELECT deleted_at, status FROM agents WHERE id=$1`, agOK.ID).Scan(&delAt, &status); err != nil {
		t.Fatalf("read agent tombstone: %v", err)
	}
	if delAt == "" || status != string(biz.AgentStatusArchived) {
		t.Fatalf("agent must be soft-deleted+archived, got deleted_at=%q status=%q", delAt, status)
	}
	if got := sessionDeletedAt(t, ctx, d, "fr13-ok-plain"); got == "" {
		t.Error("non-active session must be cascade soft-deleted")
	}
	if got := sessionDeletedAt(t, ctx, d, "fr13-ok-completed"); got == "" {
		t.Error("completed session must be cascade soft-deleted")
	}
}

// TestDeleteAgent_FR13_CascadeDepth — 13a 的失败路径断言：DeleteAgent 前置 409 并
// 非唯一防线；cascadeDeleteByAgent 自身也带谓词（纵深防御），直接单测级联语义。
func TestDeleteAgent_FR13_CascadeDepthDefensePredicate(t *testing.T) {
	client, db := testhelper.SetupTestPG(t)
	d := &Data{}
	d.SetEntClientForTest(client, db, loggateway.NewNoop())
	ctx := context.Background()

	const agentID = "fr13-agent-cascade"
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.RW().Write(ctx).Agent.Create().
		SetID(agentID).
		SetAgentKey("fr13_cascade").
		SetDisplayName("FR13 Cascade").
		SetStatus("active").
		SetProvider("deepseek").
		SetModel("deepseek-chat").
		SetKind("ecosystem_preset").
		SetSource("imported").
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	insertFR13Session(t, ctx, d, "fr13-casc-running", agentID, string(bizsession.SessionStatusRunning))
	insertFR13Session(t, ctx, d, "fr13-casc-plain", agentID, "active")

	if err := cascadeDeleteByAgent(ctx, d, agentID); err != nil {
		t.Fatalf("cascadeDeleteByAgent: %v", err)
	}
	if got := sessionDeletedAt(t, ctx, d, "fr13-casc-running"); got != "" {
		t.Error("cascade must NOT touch running session (depth-defense predicate)")
	}
	if got := sessionDeletedAt(t, ctx, d, "fr13-casc-plain"); got == "" {
		t.Error("cascade must still soft-delete non-active session")
	}
}
