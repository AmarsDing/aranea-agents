package session

import (
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

// --- 压缩产物双段化：结构化任务状态随快照注入 ---

func TestRewriteSnapshotWithCompression_TaskStateRendered(t *testing.T) {
	tail := []biz.ChatMessage{
		{Role: "user", ContentMarkdown: "继续", CreatedAt: "2026-08-16T00:00:00Z", TurnNumber: 9},
	}
	state := &biz.TaskState{
		Status:   "取证完成",
		Done:     []string{"确认告警", "定位R2"},
		Next:     "执行清除",
		Blockers: []string{"等待审批"},
	}
	raw, err := RewriteSnapshotWithCompression("{}", "## 1. User Intent\nfix vpn", tail, "agent", state)
	if err != nil {
		t.Fatal(err)
	}
	mustContain := []string{
		"[Conversation summary",
		"Task progress (structured state)",
		"Status: 取证完成",
		"- 确认告警",
		"Next: 执行清除",
		"- 等待审批",
		"## 1. User Intent",
	}
	for _, want := range mustContain {
		if !strings.Contains(raw, want) {
			t.Errorf("snapshot missing %q\n--- raw ---\n%s", want, raw)
		}
	}
	// 顺序约束：结构化状态块在叙事摘要之前（先读状态，再读叙事）。
	if strings.Index(raw, "Task progress (structured state)") > strings.Index(raw, "## 1. User Intent") {
		t.Error("structured task state must precede narrative summary")
	}
}

func TestRewriteSnapshotWithCompression_NilTaskState_LegacyFormat(t *testing.T) {
	raw, err := RewriteSnapshotWithCompression("{}", "narrative only", nil, "agent", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "Task progress") {
		t.Errorf("nil task state must not render structured block\n--- raw ---\n%s", raw)
	}
	if !strings.Contains(raw, "narrative only") {
		t.Errorf("narrative must be preserved\n--- raw ---\n%s", raw)
	}
}

func TestLatestTaskState(t *testing.T) {
	mk := func(id, ts string) biz.SessionSummary {
		return biz.SessionSummary{ID: id, SessionID: "s1", SummaryMarkdown: "m", TaskStateJSON: ts}
	}
	valid := `{"status":"s1","next":"n1"}`
	valid2 := `{"status":"s2"}`

	t.Run("empty rows", func(t *testing.T) {
		if got := latestTaskState(nil); got != nil {
			t.Fatalf("expected nil, got %+v", got)
		}
	})
	t.Run("picks newest non-empty", func(t *testing.T) {
		rows := []biz.SessionSummary{mk("a", valid), mk("b", ""), mk("c", valid2)}
		got := latestTaskState(rows)
		if got == nil || got.Status != "s2" {
			t.Fatalf("expected newest state s2, got %+v", got)
		}
	})
	t.Run("skips invalid json", func(t *testing.T) {
		rows := []biz.SessionSummary{mk("a", valid), mk("b", "{oops")}
		got := latestTaskState(rows)
		if got == nil || got.Status != "s1" {
			t.Fatalf("expected fallback to a, got %+v", got)
		}
	})
	t.Run("all empty returns nil", func(t *testing.T) {
		rows := []biz.SessionSummary{mk("a", ""), mk("b", "{}")}
		if got := latestTaskState(rows); got != nil {
			t.Fatalf("expected nil, got %+v", got)
		}
	})
}
