package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/compress"
	"aranea-agents/internal/data"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// --- v4 双段化压缩 × L1 task_board 全流程集成测试（真实 PG） ---
//
// 模拟完整业务流程：L1 长任务 active → 会话水位触发压缩 → LLM v4 双段产出
// （叙事摘要 + task_state 围栏块）→ ExtractTaskState 拆段 → 摘要行落库
// （TaskStateJSON）→ 会话快照重写（状态块 + as of turn N）→ L1 task_board
// 回写真实 PG → 第二轮压缩状态演进（latest wins 整体替换）。
// 会话存储侧用内存 stub（其契约由 data 层用例覆盖），L1 回写侧为真实 PG
// 仓库（NewL1TaskBoardWriterFromRawDB），验证的正是两层的真实接缝。

// l1FlowTestDDL 镜像 data/sql/memory_chain.sql 的 memory_l1_tasks 定义。
const l1FlowTestDDL = `CREATE TABLE memory_l1_tasks (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  run_id TEXT NOT NULL DEFAULT '',
  team_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  task_key TEXT NOT NULL DEFAULT '',
  task_title TEXT NOT NULL DEFAULT '',
  task_goal TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  schema_version INTEGER NOT NULL DEFAULT 1,
  budget_tokens INTEGER NOT NULL DEFAULT 8192,
  used_tokens INTEGER NOT NULL DEFAULT 0,
  parent_task_id TEXT NOT NULL DEFAULT '',
  shared_with_json TEXT NOT NULL DEFAULT '[]',
  started_at TEXT NOT NULL,
  ended_at TEXT NOT NULL DEFAULT '',
  archived_at TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(session_id, task_key, agent_id)
)`

// flowTimeline 构造 6 轮对话（每轮 user+assistant 各约 500 中文字符）：
// 保证 body（turn 1-2）transcript ≥1000 runes 触发真实质量门，且叙事摘要
// 体量既过退化判定又过减量守卫（不写迷你消息绕过门控）。
func flowTimeline() []biz.ChatMessage {
	chunk := strings.Repeat("障", 500)
	var out []biz.ChatMessage
	for i := 1; i <= 6; i++ {
		ts := fmt.Sprintf("2026-08-16T10:0%d:00Z", i)
		out = append(out,
			biz.ChatMessage{ID: fmt.Sprintf("fu%d", i), Role: "user", ContentMarkdown: "排查 " + chunk, TurnNumber: i, CreatedAt: ts},
			biz.ChatMessage{ID: fmt.Sprintf("fa%d", i), Role: "assistant", ContentMarkdown: "处置 " + chunk, TurnNumber: i, CreatedAt: ts},
		)
	}
	return out
}

func readFlowMetadataRaw(t *testing.T, db *sql.DB) string {
	t.Helper()
	var raw string
	if err := db.QueryRowContext(context.Background(),
		`SELECT metadata_json FROM memory_l1_tasks WHERE id = 'task-flow'`).Scan(&raw); err != nil {
		t.Fatalf("read task-flow metadata: %v", err)
	}
	return raw
}

func readFlowBoard(t *testing.T, db *sql.DB) map[string]any {
	t.Helper()
	var meta struct {
		Board map[string]any `json:"task_board"`
	}
	if err := json.Unmarshal([]byte(readFlowMetadataRaw(t, db)), &meta); err != nil {
		t.Fatalf("metadata not valid json: %v", err)
	}
	if meta.Board == nil {
		t.Fatal("task_board 缺失（回写未生效）")
	}
	return meta.Board
}

func TestCompressTaskState_Integration_FullFlowRealPG(t *testing.T) {
	ctx := context.Background()
	db := testhelper.SetupTestPGRaw(t)
	if _, err := db.ExecContext(ctx, l1FlowTestDDL); err != nil {
		t.Fatalf("create memory_l1_tasks: %v", err)
	}
	// 前置业务状态：L1 长任务已启动（active），metadata 带无关键（验证保留）。
	if _, err := db.ExecContext(ctx, `INSERT INTO memory_l1_tasks (
		id, session_id, agent_id, task_key, status, metadata_json, started_at, created_at, updated_at
	) VALUES ('task-flow', 'sess-flow', 'ag-flow', 'vpn-incident', 'active', '{"origin":"manual"}',
		'2026-08-16T09:00:00Z', '2026-08-16T09:00:00Z', '2026-08-16T09:00:00Z')`); err != nil {
		t.Fatalf("seed l1 task: %v", err)
	}

	// 会话侧 stub（读-写联动模拟 DB 读后可见）+ fake LLM（两轮 v4 双段产出）。
	read := &stubCompressReadDeps{
		sess: biz.Session{
			ID: "sess-flow", AgentID: "ag-flow",
			ContextUsedTokens: 200000, LastContextWindowTokens: 256000,
			RunnerSnapshotJSON: `{"state":{}}`,
		},
		msgs: flowTimeline(),
	}
	write := &stubCompressWriteDeps{read: read}

	chunk := strings.Repeat("已确认的事实与结论。", 30) // 叙事体量 ~300 runes，过质量门
	md1 := "## 1. User Intent & Goals\n处置 vpn 告警。" + chunk +
		"\n\n```json\n{\"status\":\"取证完成\",\"done\":[\"确认告警\",\"定位R2\"],\"next\":\"执行清除\",\"blockers\":[\"等待审批\"]}\n```"
	md2 := "## 1. User Intent & Goals\n处置 vpn 告警。" + chunk +
		"\n\n## 9. Current Work State\n清除已执行。" + chunk +
		"\n\n```json\n{\"status\":\"清除完成\",\"done\":[\"确认告警\",\"定位R2\",\"故障清除\"],\"next\":\"复核告警\"}\n```"
	fake := &scriptedLLMCompressor{results: []compress.Result{
		{Markdown: md1, Provider: "p", Model: "m"},
		{Markdown: md2, Provider: "p", Model: "m"},
	}}

	c := &Compressor{
		deps: compressDeps{
			sessionReader: read, messageReader: read, summaryReader: read,
			summaryWriter: write, messageWriter: write, contextUpdater: write,
			compressRepo: &stubCompressTxDeps{version: 0},
		},
		Compress:      fake,
		l1BoardWriter: data.NewL1TaskBoardWriterFromRawDB(db, loggateway.NewNoop()),
		lg:            loggateway.NewNoop(),
		flight:        newCompressFlightManager(),
		buf:           newCompressBufferManager(),
		suppress:      newCompressSuppressManager(),
	}
	ag := biz.Agent{ID: "ag-flow"}

	// ---------- 第一轮压缩：取证完成 ----------
	if err := c.runCompress(ctx, "sess-flow", "u", ag, true); err != nil {
		t.Fatalf("round1 runCompress: %v", err)
	}
	if fake.calls != 1 {
		t.Fatalf("round1 LLM calls=%d want 1", fake.calls)
	}
	if len(write.inserted) != 1 {
		t.Fatalf("round1 summaries=%d want 1", len(write.inserted))
	}
	row1 := write.inserted[0]
	if strings.Contains(row1.SummaryMarkdown, "```json") {
		t.Fatalf("叙事摘要不得残留 json 围栏: %s", row1.SummaryMarkdown)
	}
	for _, want := range []string{`"status":"取证完成"`, `"next":"执行清除"`} {
		if !strings.Contains(row1.TaskStateJSON, want) {
			t.Fatalf("摘要行 TaskStateJSON 缺 %s: %s", want, row1.TaskStateJSON)
		}
	}
	// 快照：状态块带时点标注（body=turn1-2 → as of turn 2）且在叙事之前。
	if !strings.Contains(write.snapshotJSON, "Task progress (structured state, as of turn 2)") {
		t.Fatalf("快照缺时点标注状态块: %s", write.snapshotJSON)
	}
	if strings.Index(write.snapshotJSON, "Task progress (structured state") > strings.Index(write.snapshotJSON, "## 1. User Intent") {
		t.Fatal("状态块必须先于叙事摘要")
	}
	// 真实 PG：task_board 已回写；无关 metadata 键保留。
	board1 := readFlowBoard(t, db)
	if board1["status"] != "取证完成" || board1["next"] != "执行清除" {
		t.Fatalf("round1 task_board mismatch: %v", board1)
	}
	if raw := readFlowMetadataRaw(t, db); !strings.Contains(raw, `"origin":"manual"`) {
		t.Fatalf("无关 metadata 键必须保留: %s", raw)
	}

	// ---------- 第二轮压缩：状态演进为清除完成（模拟下一次水位触发） ----------
	if err := c.runCompress(ctx, "sess-flow", "u", ag, true); err != nil {
		t.Fatalf("round2 runCompress: %v", err)
	}
	if fake.calls != 2 {
		t.Fatalf("round2 LLM calls=%d want 2", fake.calls)
	}
	// absorb：旧摘要行被删除归一（read-write 联动 stub 可见）。
	if len(read.listSummaries) != 1 {
		t.Fatalf("absorb 后摘要行=%d want 1（递归滚动归一）", len(read.listSummaries))
	}
	// L1 单一权威源：旧状态零残留，done 不拼接累积。
	rawMeta := readFlowMetadataRaw(t, db)
	if strings.Contains(rawMeta, "取证完成") {
		t.Fatalf("旧状态必须被整体替换: %s", rawMeta)
	}
	board2 := readFlowBoard(t, db)
	if board2["status"] != "清除完成" || board2["next"] != "复核告警" {
		t.Fatalf("round2 task_board mismatch: %v", board2)
	}
	if done, _ := board2["done"].([]any); len(done) != 3 {
		t.Fatalf("round2 done 应为 3 条（整体替换非拼接）: %v", board2["done"])
	}
	// 快照同步最新状态（旧状态零残留）。
	if !strings.Contains(write.snapshotJSON, "清除完成") || strings.Contains(write.snapshotJSON, "取证完成") {
		t.Fatalf("round2 快照状态未同步更新: %s", write.snapshotJSON)
	}
}
