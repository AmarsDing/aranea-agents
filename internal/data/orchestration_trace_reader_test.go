package data

import (
	"context"
	"fmt"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// ── OrchestrationTraceReader (P3-1) PG 集成测试 ─────────────────────────────

// setupTraceReader creates the raw-DDL orchestrations + flow_log_events tables
// (both outside Ent auto-migration) over a throwaway test schema.
func setupTraceReader(t *testing.T) (biz.OrchestrationTraceReader, *Data, context.Context) {
	t.Helper()
	client, rawDB := testhelper.SetupTestPG(t)
	d := &Data{}
	d.SetEntClientForTest(client, rawDB, loggateway.NewNoop())
	ctx := context.Background()
	if err := EnsureOrchestrationSchema(ctx, rawDB, d.Dialect(), loggateway.NewNoop()); err != nil {
		t.Fatal(err)
	}
	_, err := rawDB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS flow_log_events (
		id TEXT PRIMARY KEY,
		trace_id TEXT NOT NULL DEFAULT '',
		session_id TEXT NOT NULL DEFAULT '',
		run_id TEXT NOT NULL DEFAULT '',
		team_id TEXT NOT NULL DEFAULT '',
		domain TEXT NOT NULL DEFAULT '',
		agent_key TEXT NOT NULL DEFAULT '',
		step_id TEXT NOT NULL DEFAULT '',
		flow_phase TEXT NOT NULL DEFAULT '',
		severity TEXT NOT NULL DEFAULT 'info',
		title TEXT NOT NULL DEFAULT '',
		message TEXT NOT NULL DEFAULT '',
		payload_json TEXT NOT NULL DEFAULT '{}',
		created_at TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rawDB.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_flow_log_trace_created ON flow_log_events(trace_id, created_at)`); err != nil {
		t.Fatal(err)
	}
	return NewOrchestrationTraceReader(d), d, ctx
}

func insertTraceOrchestration(t *testing.T, d *Data, ctx context.Context, id, traceID, status, cancelReason, teamIDsJSON string, createdAt, updatedAt time.Time) {
	t.Helper()
	_, err := d.RWDB().WriteDB(ctx).ExecContext(ctx,
		`INSERT INTO orchestrations (id, spirit_session_id, trace_id, strategy, status, cancel_reason, team_ids_json, created_at, updated_at)
		 VALUES ($1, $2, $3, 'coordinator', $4, $5, $6, $7, $8)`,
		id, "ss-"+id, traceID, status, cancelReason, teamIDsJSON,
		createdAt.UTC().Format(time.RFC3339), updatedAt.UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
}

func insertTraceFlowLog(t *testing.T, d *Data, ctx context.Context, id, traceID, stepID, severity, message string, at time.Time) {
	t.Helper()
	_, err := d.RWDB().WriteDB(ctx).ExecContext(ctx,
		`INSERT INTO flow_log_events (id, trace_id, step_id, severity, message, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		id, traceID, stepID, severity, message, at.UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
}

func TestOrchestrationTraceReader_ListTerminal(t *testing.T) {
	reader, d, ctx := setupTraceReader(t)
	now := time.Now().UTC()

	// 窗口内：failed 编排（2 team，持续 5s）+ doom_loop cancelled 编排。
	insertTraceOrchestration(t, d, ctx, "orch-fail", "tr-fail", "failed", "", `["t1","t2"]`, now.Add(-time.Hour), now.Add(-time.Hour).Add(5*time.Second))
	insertTraceOrchestration(t, d, ctx, "orch-doom", "tr-doom", "cancelled", "doom_loop", `["t3"]`, now.Add(-30*time.Minute), now.Add(-29*time.Minute))
	// 窗口内但非终态（running）与成功（completed）→ 排除。
	insertTraceOrchestration(t, d, ctx, "orch-running", "tr-run", "running", "", `["t4"]`, now.Add(-time.Hour), now.Add(-time.Minute))
	insertTraceOrchestration(t, d, ctx, "orch-ok", "tr-ok", "completed", "", `["t5"]`, now.Add(-time.Hour), now.Add(-time.Minute))
	// 终态但窗口外（48h 前）→ 排除。
	insertTraceOrchestration(t, d, ctx, "orch-old", "tr-old", "failed", "", `["t6"]`, now.Add(-49*time.Hour), now.Add(-48*time.Hour))

	// flow-log：tr-fail 上 step-execute ×3 error + ×1 critical + ×2 warn；
	// 两条 error 消息，最新者应胜为 LastError。
	base := now.Add(-50 * time.Minute)
	for i := 0; i < 3; i++ {
		insertTraceFlowLog(t, d, ctx, fmt.Sprintf("fl-e%d", i), "tr-fail", "spirit.team.execute", "error", fmt.Sprintf("boom #%d", i), base.Add(time.Duration(i)*time.Minute))
	}
	insertTraceFlowLog(t, d, ctx, "fl-crit", "tr-fail", "spirit.planner.assess", "critical", "latest failure", base.Add(10*time.Minute))
	insertTraceFlowLog(t, d, ctx, "fl-w1", "tr-fail", "spirit.team.execute", "warn", "degraded", base.Add(time.Minute))
	insertTraceFlowLog(t, d, ctx, "fl-w2", "tr-fail", "spirit.team.execute", "warn", "retrying", base.Add(2*time.Minute))
	// info 严重度不聚合；其他 trace 的事件不串扰。
	insertTraceFlowLog(t, d, ctx, "fl-info", "tr-fail", "spirit.team.execute", "info", "ok", base.Add(3*time.Minute))
	insertTraceFlowLog(t, d, ctx, "fl-other", "tr-doom", "spirit.graph.node", "error", "doom boom", base.Add(4*time.Minute))

	traces, err := reader.ListTerminalOrchestrationTraces(ctx, now.Add(-24*time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(traces) != 2 {
		t.Fatalf("len = %d, want 2 (failed + cancelled, 窗口内)", len(traces))
	}
	byID := map[string]biz.OrchestrationTrace{}
	for _, tr := range traces {
		byID[tr.OrchestrationID] = tr
	}

	fail := byID["orch-fail"]
	if fail.TeamCount != 2 {
		t.Errorf("TeamCount = %d, want 2", fail.TeamCount)
	}
	if fail.DurationMS != 5000 {
		t.Errorf("DurationMS = %d, want 5000", fail.DurationMS)
	}
	if fail.ErrorSteps["spirit.team.execute"] != 3 {
		t.Errorf("ErrorSteps[execute] = %d, want 3", fail.ErrorSteps["spirit.team.execute"])
	}
	if fail.ErrorSteps["spirit.planner.assess"] != 1 {
		t.Errorf("ErrorSteps[assess] = %d, want 1 (critical 计为 error)", fail.ErrorSteps["spirit.planner.assess"])
	}
	if fail.WarnCount != 2 {
		t.Errorf("WarnCount = %d, want 2", fail.WarnCount)
	}
	if fail.LastError != "latest failure" {
		t.Errorf("LastError = %q, want latest failure (created_at DESC first-wins)", fail.LastError)
	}

	doom := byID["orch-doom"]
	if doom.CancelReason != "doom_loop" {
		t.Errorf("CancelReason = %q, want doom_loop", doom.CancelReason)
	}
	if doom.ErrorSteps["spirit.graph.node"] != 1 {
		t.Errorf("doom ErrorSteps = %v", doom.ErrorSteps)
	}
}

func TestOrchestrationTraceReader_EmptyWindow(t *testing.T) {
	reader, d, ctx := setupTraceReader(t)
	now := time.Now().UTC()
	insertTraceOrchestration(t, d, ctx, "orch-old", "tr-old", "failed", "", `["t1"]`, now.Add(-49*time.Hour), now.Add(-48*time.Hour))
	traces, err := reader.ListTerminalOrchestrationTraces(ctx, now.Add(-24*time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(traces) != 0 {
		t.Errorf("len = %d, want 0 (窗口外终态不进观测)", len(traces))
	}
}

// reader 输出必须可直接被 biz 标注规则消费（跨层契约冒烟）。
func TestOrchestrationTraceReader_AnnotationContract(t *testing.T) {
	reader, d, ctx := setupTraceReader(t)
	now := time.Now().UTC()
	insertTraceOrchestration(t, d, ctx, "orch-doom", "tr-doom", "cancelled", "doom_loop", `["t1"]`, now.Add(-time.Hour), now.Add(-time.Minute))
	insertTraceFlowLog(t, d, ctx, "fl-1", "tr-doom", "spirit.graph.node", "error", "loop", now.Add(-time.Minute))

	traces, err := reader.ListTerminalOrchestrationTraces(ctx, now.Add(-24*time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(traces) != 1 {
		t.Fatalf("len = %d, want 1", len(traces))
	}
	a := biz.AnnotateOrchestrationTrace(traces[0])
	if a == nil {
		t.Fatal("doom_loop trace should be annotated")
	}
	if a.Mode != biz.MASTStepRepetition {
		t.Errorf("mode = %q, want FM-1.3", a.Mode)
	}
}
