package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// mergeTaskBoardMetadata：task_board 键合并的纯函数契约（P0 回写路径核心）。
func TestMergeTaskBoardMetadata(t *testing.T) {
	board := `{"status":"取证完成","next":"执行清除"}`

	t.Run("empty metadata starts from {}", func(t *testing.T) {
		got, err := mergeTaskBoardMetadata("", board)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal([]byte(got), &m); err != nil {
			t.Fatalf("output must be valid json: %v", err)
		}
		if string(m["task_board"]) != board {
			t.Fatalf("task_board mismatch: %s", m["task_board"])
		}
	})

	t.Run("existing keys preserved, board replaced", func(t *testing.T) {
		meta := `{"origin":"manual","task_board":{"status":"旧状态"}}`
		got, err := mergeTaskBoardMetadata(meta, board)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal([]byte(got), &m); err != nil {
			t.Fatal(err)
		}
		if string(m["origin"]) != `"manual"` {
			t.Fatalf("unrelated key must be preserved: %s", m["origin"])
		}
		if string(m["task_board"]) != board {
			t.Fatalf("task_board must be replaced: %s", m["task_board"])
		}
	})

	t.Run("broken existing metadata resets to {}", func(t *testing.T) {
		got, err := mergeTaskBoardMetadata("{oops", board)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal([]byte(got), &m); err != nil {
			t.Fatal(err)
		}
		if string(m["task_board"]) != board {
			t.Fatalf("task_board mismatch: %s", m["task_board"])
		}
	})

	t.Run("invalid board json errors", func(t *testing.T) {
		if _, err := mergeTaskBoardMetadata("{}", "{not-json"); err == nil {
			t.Fatal("expected error for invalid board json")
		}
	})
}

// --- UpdateL1TaskBoard（L1TaskBoardWriter）仓库级契约：双状态源冲突 ---
//
// 双状态源冲突的两种形态：
//  a) 键级冲突：metadata_json 已有旧 task_board（上一次压缩产出），新 board 写入时
//     两源不得合并（done/next 禁止拼接），最新压缩产出为唯一权威。
//  b) 行级冲突：L1 prompt cue 渲染 ListL1TaskRows 首行（status IN active/paused,
//     updated_at 最新）；回写若命中其他行，cue 与快照注入各渲染一份状态即分裂。

// l1BoardTestDDL 镜像 data/sql/memory_chain.sql 的 memory_l1_tasks 定义。
const l1BoardTestDDL = `CREATE TABLE memory_l1_tasks (
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

func setupL1TaskBoardRepo(t *testing.T) (*l1WorkingMemoryRepo, *sql.DB) {
	t.Helper()
	db := testhelper.SetupTestPGRaw(t)
	if _, err := db.ExecContext(context.Background(), l1BoardTestDDL); err != nil {
		t.Fatalf("create memory_l1_tasks: %v", err)
	}
	d := &Data{rawDB: db, readDB: db, rwDB: NewReadWriteDB(db, db), lg: loggateway.NewNoop(), dialect: DialectPostgres}
	return newL1WorkingMemoryRepo(d), db
}

func insertL1BoardTask(t *testing.T, db *sql.DB, id, taskKey, status, metadata, updatedAt string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `INSERT INTO memory_l1_tasks (
		id, session_id, agent_id, task_key, status, metadata_json, started_at, created_at, updated_at
	) VALUES ($1, 'sess-board', 'ag-board', $2, $3, $4, '2026-08-16T00:00:00Z', '2026-08-16T00:00:00Z', $5)`,
		id, taskKey, status, metadata, updatedAt)
	if err != nil {
		t.Fatalf("insert l1 task %s: %v", id, err)
	}
}

func readL1TaskMetadata(t *testing.T, db *sql.DB, id string) map[string]json.RawMessage {
	t.Helper()
	var raw string
	if err := db.QueryRowContext(context.Background(),
		`SELECT metadata_json FROM memory_l1_tasks WHERE id = $1`, id).Scan(&raw); err != nil {
		t.Fatalf("read metadata %s: %v", id, err)
	}
	m := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("metadata %s not valid json: %v", id, err)
	}
	return m
}

// 键级冲突：旧 task_board（旧状态源）必须被新 board 整体替换——
// 最新压缩产出为唯一权威，两源不得合并；其他 metadata 键保留。
func TestUpdateL1TaskBoard_StaleBoardReplacedLatestWins(t *testing.T) {
	repo, db := setupL1TaskBoardRepo(t)
	insertL1BoardTask(t, db, "t1", "k1", "active",
		`{"origin":"manual","task_board":{"status":"旧状态","done":["旧步骤"],"next":"旧下一步"}}`,
		"2026-08-16T00:00:01Z")

	ok, err := repo.UpdateL1TaskBoard(context.Background(), "sess-board", "ag-board",
		`{"status":"新状态","done":["新步骤A","新步骤B"],"next":"新下一步"}`)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	m := readL1TaskMetadata(t, db, "t1")
	if string(m["origin"]) != `"manual"` {
		t.Fatalf("unrelated key must survive: %s", m["origin"])
	}
	var board struct {
		Status string   `json:"status"`
		Done   []string `json:"done"`
		Next   string   `json:"next"`
	}
	if err := json.Unmarshal(m["task_board"], &board); err != nil {
		t.Fatal(err)
	}
	if board.Status != "新状态" || board.Next != "新下一步" {
		t.Fatalf("stale board must be fully replaced: %+v", board)
	}
	if len(board.Done) != 2 || board.Done[0] != "新步骤A" {
		t.Fatalf("done must not accumulate stale entries: %v", board.Done)
	}
}

// 行级冲突：写入行必须就是 cue 渲染行。paused 行 updated_at 最新 →
// cue 渲染它，回写也必须命中它；命中其他行即双源分裂。
func TestUpdateL1TaskBoard_HitsCueRenderedRow(t *testing.T) {
	repo, db := setupL1TaskBoardRepo(t)
	insertL1BoardTask(t, db, "t-active", "k1", "active", `{"task_board":{"status":"旧A"}}`, "2026-08-16T00:00:01Z")
	insertL1BoardTask(t, db, "t-paused", "k2", "paused", `{"task_board":{"status":"旧P"}}`, "2026-08-16T00:00:02Z")

	ok, err := repo.UpdateL1TaskBoard(context.Background(), "sess-board", "ag-board", `{"status":"最新"}`)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if got := readL1TaskMetadata(t, db, "t-paused")["task_board"]; !strings.Contains(string(got), "最新") {
		t.Fatalf("newest active/paused row must receive board: %s", got)
	}
	if got := readL1TaskMetadata(t, db, "t-active")["task_board"]; !strings.Contains(string(got), "旧A") {
		t.Fatalf("older row must stay untouched: %s", got)
	}

	// 镜像 l1_prompt.go 的取行方式：cue 渲染 taskRows[0] 的 metadata_json["task_board"]。
	rows, err := repo.ListL1TaskRows(context.Background(), "sess-board", "ag-board", "", "")
	if err != nil || len(rows) == 0 {
		t.Fatalf("list rows err=%v n=%d", err, len(rows))
	}
	var task map[string]any
	if err := json.Unmarshal(rows[0], &task); err != nil {
		t.Fatal(err)
	}
	if meta, _ := task["metadata_json"].(string); !strings.Contains(meta, "最新") {
		t.Fatalf("cue-rendered row must carry the fresh board, meta=%s", meta)
	}
}

// 行级冲突变体：ended 行带陈旧 board 且 updated_at 更新，回写仍必须命中
// active 行——否则 cue（只读 active/paused）看不到新状态，形成分裂。
func TestUpdateL1TaskBoard_EndedRowWithNewerTimestampIgnored(t *testing.T) {
	repo, db := setupL1TaskBoardRepo(t)
	insertL1BoardTask(t, db, "t-active", "k1", "active", `{}`, "2026-08-16T00:00:01Z")
	insertL1BoardTask(t, db, "t-ended", "k2", "ended", `{"task_board":{"status":"陈旧完结态"}}`, "2026-08-16T00:00:09Z")

	ok, err := repo.UpdateL1TaskBoard(context.Background(), "sess-board", "ag-board", `{"status":"进行中"}`)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if got := readL1TaskMetadata(t, db, "t-active")["task_board"]; !strings.Contains(string(got), "进行中") {
		t.Fatalf("active row must receive board: %s", got)
	}
	if got := readL1TaskMetadata(t, db, "t-ended")["task_board"]; !strings.Contains(string(got), "陈旧完结态") {
		t.Fatalf("ended row must stay untouched: %s", got)
	}
}

// 无 eligible 行（仅 ended）→ (false, nil) 且陈旧 board 不得被复活；
// 空参数静默跳过。
func TestUpdateL1TaskBoard_NoEligibleRow(t *testing.T) {
	repo, db := setupL1TaskBoardRepo(t)
	insertL1BoardTask(t, db, "t-ended", "k1", "ended", `{"task_board":{"status":"陈旧"}}`, "2026-08-16T00:00:01Z")

	ok, err := repo.UpdateL1TaskBoard(context.Background(), "sess-board", "ag-board", `{"status":"新"}`)
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v, want (false,nil)", ok, err)
	}
	for _, args := range [][3]string{
		{"", "ag-board", `{"status":"新"}`},
		{"sess-board", "", `{"status":"新"}`},
		{"sess-board", "ag-board", ""},
	} {
		ok, err = repo.UpdateL1TaskBoard(context.Background(), args[0], args[1], args[2])
		if err != nil || ok {
			t.Fatalf("args=%v ok=%v err=%v, want (false,nil)", args, ok, err)
		}
	}
	if got := readL1TaskMetadata(t, db, "t-ended")["task_board"]; !strings.Contains(string(got), "陈旧") {
		t.Fatalf("ended row must stay untouched: %s", got)
	}
}

// 连续压缩回写：第二次产出覆盖第一次，任何时刻 metadata 中只有一份 board——
// 与快照注入的状态同源同份，杜绝 cue 与快照各渲染一份的漂移。
func TestUpdateL1TaskBoard_RepeatedWritebacksLatestWins(t *testing.T) {
	repo, db := setupL1TaskBoardRepo(t)
	insertL1BoardTask(t, db, "t1", "k1", "active", `{}`, "2026-08-16T00:00:01Z")

	if ok, err := repo.UpdateL1TaskBoard(context.Background(), "sess-board", "ag-board",
		`{"status":"turn8","done":["d1"],"next":"n1"}`); err != nil || !ok {
		t.Fatalf("first writeback ok=%v err=%v", ok, err)
	}
	if ok, err := repo.UpdateL1TaskBoard(context.Background(), "sess-board", "ag-board",
		`{"status":"turn12","done":["d2"],"next":"n2"}`); err != nil || !ok {
		t.Fatalf("second writeback ok=%v err=%v", ok, err)
	}
	raw := string(readL1TaskMetadata(t, db, "t1")["task_board"])
	if strings.Contains(raw, "turn8") || strings.Contains(raw, "d1") {
		t.Fatalf("first board must be fully superseded: %s", raw)
	}
	if !strings.Contains(raw, "turn12") {
		t.Fatalf("latest board missing: %s", raw)
	}
}

// 非法 boardJSON 报错且目标行不变（失败不得留下半更新状态）。
func TestUpdateL1TaskBoard_InvalidBoardJSON(t *testing.T) {
	repo, db := setupL1TaskBoardRepo(t)
	insertL1BoardTask(t, db, "t1", "k1", "active", `{"task_board":{"status":"旧"}}`, "2026-08-16T00:00:01Z")

	if _, err := repo.UpdateL1TaskBoard(context.Background(), "sess-board", "ag-board", `{not-json`); err == nil {
		t.Fatal("expected error for invalid board json")
	}
	if got := readL1TaskMetadata(t, db, "t1")["task_board"]; !strings.Contains(string(got), "旧") {
		t.Fatalf("row must stay untouched on error: %s", got)
	}
}

// 业务流程集成（真实 PG，跨 biz/data 两层）：
// L1 任务启动（StartL1Task 初始化 metadata_json='{}'）→ 压缩产出的
// task_state 段回写 → L1 prompt cue 读路径（ListL1TaskRows 首行）解析出
// 与压缩产出完全一致的状态。（v4 双段拆段由 session 侧集成测试覆盖——
// data 测试 import compress 会经 agent→session/trpc 形成测试 import 环。）
func TestL1TaskBoardFlow_StartWritebackCueRead(t *testing.T) {
	repo, _ := setupL1TaskBoardRepo(t)
	ctx := context.Background()

	// 1) 任务启动：metadata_json 初始为 '{}'。
	if _, err := repo.StartL1Task(ctx, biz.L1TaskInsert{
		SessionID: "sess-board", AgentID: "ag-board",
		TaskKey: "vpn", TaskTitle: "vpn 故障处置",
	}); err != nil {
		t.Fatalf("StartL1Task: %v", err)
	}

	// 2) 压缩产出的 task_state 段（v4 契约键）回写。
	stateJSON := `{"status":"取证完成","done":["确认告警","定位R2"],"next":"执行清除","blockers":["等待审批"]}`
	ok, err := repo.UpdateL1TaskBoard(ctx, "sess-board", "ag-board", stateJSON)
	if err != nil || !ok {
		t.Fatalf("writeback ok=%v err=%v", ok, err)
	}

	// 3) cue 读路径：镜像 l1_prompt.go 取 taskRows[0] 的 metadata_json["task_board"]。
	rows, err := repo.ListL1TaskRows(ctx, "sess-board", "ag-board", "", "")
	if err != nil || len(rows) != 1 {
		t.Fatalf("cue rows n=%d err=%v", len(rows), err)
	}
	var task map[string]any
	if err := json.Unmarshal(rows[0], &task); err != nil {
		t.Fatal(err)
	}
	var meta struct {
		Board biz.TaskState `json:"task_board"`
	}
	if err := json.Unmarshal([]byte(fmt.Sprint(task["metadata_json"])), &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Board.Status != "取证完成" || meta.Board.Next != "执行清除" ||
		len(meta.Board.Done) != 2 || len(meta.Board.Blockers) != 1 {
		t.Fatalf("cue 读到的 board 与压缩产出不一致: %+v", meta.Board)
	}
}

func insertL1BoardTaskSession(t *testing.T, db *sql.DB, id, sessionID, agentID, title, status, metadata, updatedAt string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `INSERT INTO memory_l1_tasks (
		id, session_id, agent_id, task_key, task_title, status, metadata_json, started_at, created_at, updated_at
	) VALUES ($1, $2, $3, $4, $5, $6, $7, '2026-08-16T00:00:00Z', '2026-08-16T00:00:00Z', $8)`,
		id, sessionID, agentID, id, title, status, metadata, updatedAt)
	if err != nil {
		t.Fatalf("insert l1 task %s: %v", id, err)
	}
}

func TestLatestL1TaskBoard_SkipsCurrentSessionAndEmptyBoard(t *testing.T) {
	repo, db := setupL1TaskBoardRepo(t)
	insertL1BoardTaskSession(t, db, "t-cur", "sess-new", "ag-1", "当前", "active",
		`{"task_board":{"status":"本会话"}}`, "2026-08-22T12:00:00Z")
	insertL1BoardTaskSession(t, db, "t-empty", "sess-old-empty", "ag-1", "空板", "active",
		`{}`, "2026-08-22T11:00:00Z")
	insertL1BoardTaskSession(t, db, "t-old", "sess-old", "ag-1", "机房巡检", "paused",
		`{"task_board":{"status":"取证完成","next":"执行清除"}}`, "2026-08-22T10:00:00Z")

	raw, err := repo.LatestL1TaskBoard(context.Background(), "ag-1", "sess-new")
	if err != nil || raw == nil {
		t.Fatalf("LatestL1TaskBoard: raw=%s err=%v", raw, err)
	}
	var task map[string]any
	if err := json.Unmarshal(raw, &task); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(task["id"]) != "t-old" {
		t.Fatalf("want t-old (skip current + empty board), got %v", task["id"])
	}
}
