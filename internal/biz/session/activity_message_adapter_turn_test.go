package session

import (
	"encoding/json"
	"testing"
	"time"
)

// 工具名经 options_json 透传给前端渲染工具卡片：含引号/反斜杠/控制字符的
// ToolName 必须经 json.Marshal 转义——字符串拼接会产生非法 JSON，前端解析失败
// 静默丢失工具卡片。
func TestBuildOptionsJSON_EscapesToolName(t *testing.T) {
	name := "evil\"tool\\name\n"
	got := buildOptionsJSON(ActivityEntry{Kind: "action", ToolName: name})
	var parsed map[string]string
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("options_json 非法: %q: %v", got, err)
	}
	if parsed["tool_name"] != name {
		t.Fatalf("tool_name = %q, want %q（必须原样往返）", parsed["tool_name"], name)
	}
}

// The session compression pipeline (internal/session/compressor.go) filters
// the compression body by ChatMessage.TurnNumber (m.TurnNumber > maxSummarized
// && m.TurnNumber <= cutoffTurn). After the messages table was dropped
// (migration 20260902), ActivityMessageReader became the only message source,
// but Activity carries TurnID only — TurnNumber was never populated, leaving
// every message at TurnNumber=0 and the compression body permanently empty
// (silent no-op). The adapter must synthesize stable per-turn ordinals from
// TurnID (activities are append-only, so chronological ordinal assignment is
// stable across compressions).
func TestActivitiesToChatMessages_SynthesizesTurnNumber(t *testing.T) {
	base := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	acts := []ActivityEntry{
		{ID: "a1", Kind: "task", Status: "completed", SessionID: "s1", TurnID: "t1", Timestamp: base, Content: "u1"},
		{ID: "a2", Kind: "action", Status: "completed", SessionID: "s1", TurnID: "t1", Timestamp: base.Add(time.Second), ToolName: "search", ToolResult: "r1"},
		{ID: "a3", Kind: "reply", Status: "completed", SessionID: "s1", TurnID: "t1", Timestamp: base.Add(2 * time.Second), Content: "a1"},
		{ID: "a4", Kind: "task", Status: "completed", SessionID: "s1", TurnID: "t2", Timestamp: base.Add(3 * time.Second), Content: "u2"},
		{ID: "a5", Kind: "reply", Status: "completed", SessionID: "s1", TurnID: "t2", Timestamp: base.Add(4 * time.Second), Content: "a2"},
	}
	msgs := activitiesToChatMessages(acts)
	if len(msgs) != 5 {
		t.Fatalf("len(msgs) = %d, want 5", len(msgs))
	}
	want := map[string]int{"a1": 1, "a2": 1, "a3": 1, "a4": 2, "a5": 2}
	for _, m := range msgs {
		if got := m.TurnNumber; got != want[m.ID] {
			t.Errorf("msg %s TurnNumber = %d, want %d", m.ID, got, want[m.ID])
		}
	}
}

// Activities without TurnID (legacy/session-level rows) keep TurnNumber=0 and
// are therefore excluded from the compression body — preserving prior
// behavior for un-attributed rows.
func TestActivitiesToChatMessages_EmptyTurnIDKeepsZero(t *testing.T) {
	base := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	acts := []ActivityEntry{
		{ID: "a1", Kind: "notice", NoticeType: "info", Status: "completed", SessionID: "s1", Timestamp: base, Content: "hello"},
		{ID: "a2", Kind: "task", Status: "completed", SessionID: "s1", TurnID: "t1", Timestamp: base.Add(time.Second), Content: "u1"},
	}
	msgs := activitiesToChatMessages(acts)
	if len(msgs) != 2 {
		t.Fatalf("len(msgs) = %d, want 2", len(msgs))
	}
	if msgs[0].TurnNumber != 0 {
		t.Errorf("TurnID-less msg TurnNumber = %d, want 0", msgs[0].TurnNumber)
	}
	if msgs[1].TurnNumber != 1 {
		t.Errorf("turn t1 msg TurnNumber = %d, want 1", msgs[1].TurnNumber)
	}
}

// Out-of-order input must still produce chronological ordinals (defensive:
// ListBySession ordering is not contractual).
func TestActivitiesToChatMessages_TurnOrdinalStableWhenUnsorted(t *testing.T) {
	base := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	acts := []ActivityEntry{
		{ID: "a2", Kind: "reply", Status: "completed", SessionID: "s1", TurnID: "t2", Timestamp: base.Add(2 * time.Second), Content: "a2"},
		{ID: "a1", Kind: "task", Status: "completed", SessionID: "s1", TurnID: "t1", Timestamp: base, Content: "u1"},
	}
	msgs := activitiesToChatMessages(acts)
	if len(msgs) != 2 {
		t.Fatalf("len(msgs) = %d, want 2", len(msgs))
	}
	if msgs[0].ID != "a1" || msgs[0].TurnNumber != 1 {
		t.Errorf("msgs[0] = %+v, want a1 with TurnNumber 1", msgs[0])
	}
	if msgs[1].ID != "a2" || msgs[1].TurnNumber != 2 {
		t.Errorf("msgs[1] = %+v, want a2 with TurnNumber 2", msgs[1])
	}
}
