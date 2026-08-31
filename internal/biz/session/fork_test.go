package session

import (
	"context"
	"testing"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// 79-runtime-governance R6 / 2026-08-28 T5 裁定：多代 fork 放开后的范围门禁。
// 规则：根会话（parent 为空）与 fork 会话（parent+fork_from_turn_id 双非空）
// 可 fork；team/member 子会话（仅 parent 非空）历史不闭合，明确拒绝。

type forkFakeReader struct {
	SessionReader
	sess Session
	// created 记录 writer 落库的 fork 子会话；GetSessionByID 命中其 ID 时返回它
	// （生产实现返回持久化后的行，fake 必须同语义，否则断言到的恒是源会话）。
	created *Session
}

func (f *forkFakeReader) GetSessionByID(_ context.Context, id string) (Session, error) {
	if f.created != nil && f.created.ID == id {
		return *f.created, nil
	}
	return f.sess, nil
}

type forkFakeWriter struct {
	SessionWriter
	reader *forkFakeReader
}

func (f *forkFakeWriter) CreateSession(_ context.Context, s Session) (Session, error) {
	if f.reader != nil {
		f.reader.created = &s
	}
	return s, nil
}

type forkFakeStore struct{}

func (forkFakeStore) ForkSessionInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}
func (forkFakeStore) FindTurnEventBoundary(_ context.Context, _, _ string) (int64, bool, error) {
	return 1, true, nil
}
func (forkFakeStore) CopyFrameworkEvents(_ context.Context, _, _ string, _ int64) (int, error) {
	return 0, nil
}
func (forkFakeStore) CreateFrameworkState(_ context.Context, _, _ string) error { return nil }
func (forkFakeStore) CopyV2Records(_ context.Context, _, _, _ string) (int, int, int, error) {
	return 0, 0, 0, nil
}
func (forkFakeStore) InitForkedSessionMetrics(_ context.Context, _, _ string, _ int) error {
	return nil
}

func TestSessionForkUsecase_ScopeGate(t *testing.T) {
	t.Parallel()
	mk := func(sess Session) *SessionForkUsecase {
		reader := &forkFakeReader{sess: sess}
		return NewSessionForkUsecase(reader, &forkFakeWriter{reader: reader}, forkFakeStore{}, loggateway.NewNoop())
	}

	t.Run("root session allowed", func(t *testing.T) {
		t.Parallel()
		uc := mk(Session{ID: "root-1", Title: "源会话"})
		dst, err := uc.Fork(context.Background(), "root-1", "turn-1", "")
		if err != nil {
			t.Fatalf("root fork: %v", err)
		}
		if dst.ParentSessionID != "root-1" || dst.RootSessionID != "root-1" {
			t.Fatalf("lineage = parent:%q root:%q, want root-1/root-1", dst.ParentSessionID, dst.RootSessionID)
		}
	})

	t.Run("forked session allowed (multi-gen, T5)", func(t *testing.T) {
		t.Parallel()
		src := Session{ID: "fork-1", Title: "源会话（分支）", ParentSessionID: "root-1", ForkFromTurnID: "turn-1", RootSessionID: "root-1"}
		uc := mk(src)
		dst, err := uc.Fork(context.Background(), "fork-1", "fk11111111-turn-1", "")
		if err != nil {
			t.Fatalf("multi-gen fork must be allowed after T5: %v", err)
		}
		if dst.ParentSessionID != "fork-1" {
			t.Fatalf("parent = %q, want fork-1", dst.ParentSessionID)
		}
		if dst.RootSessionID != "root-1" {
			t.Fatalf("root lineage = %q, want inherited root-1", dst.RootSessionID)
		}
		// P6-N5：落库 fork_from_turn_id 必须剥离继承前缀（字段语义 =
		// 框架 invocation id），多代 fork 不得存复合前缀形态。
		if dst.ForkFromTurnID != "turn-1" {
			t.Fatalf("fork_from_turn_id = %q, want normalized turn-1", dst.ForkFromTurnID)
		}
	})

	t.Run("team/member child rejected", func(t *testing.T) {
		t.Parallel()
		src := Session{ID: "member-1", Title: "成员子会话", ParentSessionID: "root-1", RootSessionID: "root-1"}
		uc := mk(src)
		_, err := uc.Fork(context.Background(), "member-1", "turn-1", "")
		if !apierror.IsCode(err, apierror.CodeBadRequest) {
			t.Fatalf("team/member child fork: err = %v, want BadRequest", err)
		}
	})
}
