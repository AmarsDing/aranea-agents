package session

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// --- P0 闭环：压缩产出 TaskState 回写 L1 task_board ---

type stubL1BoardWriter struct {
	calls     int
	sessionID string
	agentID   string
	boardJSON string
	ok        bool
	err       error
}

func (s *stubL1BoardWriter) UpdateL1TaskBoard(_ context.Context, sessionID, agentID, boardJSON string) (bool, error) {
	s.calls++
	s.sessionID = sessionID
	s.agentID = agentID
	s.boardJSON = boardJSON
	return s.ok, s.err
}

func newBoardWritebackCompressor(w biz.L1TaskBoardWriter) *Compressor {
	return &Compressor{l1BoardWriter: w, lg: loggateway.NewNoop()}
}

func TestWritebackL1TaskBoard(t *testing.T) {
	sess := biz.Session{ID: "s1", AgentID: "ag1"}
	ag := biz.Agent{ID: "ag1"}
	state := &biz.TaskState{Status: "取证完成", Next: "执行清除"}

	t.Run("writes marshaled state to session agent task", func(t *testing.T) {
		w := &stubL1BoardWriter{ok: true}
		c := newBoardWritebackCompressor(w)
		c.writebackL1TaskBoard(context.Background(), sess, ag, state)
		if w.calls != 1 {
			t.Fatalf("calls=%d want 1", w.calls)
		}
		if w.sessionID != "s1" || w.agentID != "ag1" {
			t.Fatalf("target mismatch: %s/%s", w.sessionID, w.agentID)
		}
		var board map[string]any
		if err := json.Unmarshal([]byte(w.boardJSON), &board); err != nil {
			t.Fatalf("boardJSON must be valid json: %v", err)
		}
		if board["status"] != "取证完成" || board["next"] != "执行清除" {
			t.Fatalf("board content mismatch: %s", w.boardJSON)
		}
	})

	t.Run("nil writer skips", func(t *testing.T) {
		c := newBoardWritebackCompressor(nil)
		c.writebackL1TaskBoard(context.Background(), sess, ag, state) // 不得 panic
	})

	t.Run("nil/empty state skips", func(t *testing.T) {
		w := &stubL1BoardWriter{ok: true}
		c := newBoardWritebackCompressor(w)
		c.writebackL1TaskBoard(context.Background(), sess, ag, nil)
		c.writebackL1TaskBoard(context.Background(), sess, ag, &biz.TaskState{})
		if w.calls != 0 {
			t.Fatalf("calls=%d want 0", w.calls)
		}
	})

	t.Run("falls back to ag.ID when session agent empty", func(t *testing.T) {
		w := &stubL1BoardWriter{ok: true}
		c := newBoardWritebackCompressor(w)
		c.writebackL1TaskBoard(context.Background(), biz.Session{ID: "s1"}, ag, state)
		if w.agentID != "ag1" {
			t.Fatalf("agentID=%q want ag1", w.agentID)
		}
	})

	t.Run("writer error degrades to warn, no panic", func(t *testing.T) {
		w := &stubL1BoardWriter{err: context.DeadlineExceeded}
		c := newBoardWritebackCompressor(w)
		c.writebackL1TaskBoard(context.Background(), sess, ag, state)
		if w.calls != 1 {
			t.Fatalf("calls=%d want 1", w.calls)
		}
	})

	t.Run("both agent ids empty skips", func(t *testing.T) {
		w := &stubL1BoardWriter{ok: true}
		c := newBoardWritebackCompressor(w)
		c.writebackL1TaskBoard(context.Background(), biz.Session{ID: "s1"}, biz.Agent{}, state)
		if w.calls != 0 {
			t.Fatalf("calls=%d want 0", w.calls)
		}
	})
}

// 回写 payload 与 L1 task_board 渲染契约一致（status/done/next/blockers 键）。
func TestWritebackPayloadMatchesL1BoardContract(t *testing.T) {
	state := &biz.TaskState{Status: "s", Done: []string{"d"}, Next: "n", Blockers: []string{"b"}}
	raw := marshalTaskState(state)
	for _, key := range []string{`"status"`, `"done"`, `"next"`, `"blockers"`} {
		if !strings.Contains(raw, key) {
			t.Fatalf("payload missing %s: %s", key, raw)
		}
	}
}
