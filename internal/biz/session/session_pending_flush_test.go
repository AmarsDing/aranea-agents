package session

import (
	"context"
	"testing"
)

// --- P1 FR-7：删除/归档会话的内存 pending 队列级联清理 ---

// TestSessionUsecase_ArchiveFlushesPendingQueue 复现 FR-7 级联缺口：归档
// 成功但内存 pending 队列未清——未消费追问（含重启恢复行）会随下一条
// 消息幽灵派发，批量 ReplaceAll 还会把条目重新插回持久层。
// 修复期望：Archive 成功后调用注入的 flusher；未注入时静默跳过。
func TestSessionUsecase_ArchiveFlushesPendingQueue(t *testing.T) {
	repo := &mockSessionRepo{
		getSessionByIDFn: func(_ context.Context, id string) (Session, error) {
			return Session{ID: id, AgentID: "agent-1", Status: string(SessionStatusCompleted)}, nil
		},
		archiveSessionFn: func(context.Context, string) (int, error) { return 1, nil },
	}

	var flushed []string
	uc := newTestUsecase(repo, &mockAgentLookup{}, &mockTeamLookup{})
	uc.SetPendingQueueFlusher(func(sessionID string) { flushed = append(flushed, sessionID) })

	if err := uc.Archive(context.Background(), "s-1"); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if len(flushed) != 1 || flushed[0] != "s-1" {
		t.Fatalf("P1 FR-7: archive must flush the session pending queue, got %v", flushed)
	}

	// 未注入 flusher 时不得 panic（测试/CLI 形态默认）。
	if err := newTestUsecase(repo, &mockAgentLookup{}, &mockTeamLookup{}).Archive(context.Background(), "s-1"); err != nil {
		t.Fatalf("archive without flusher: %v", err)
	}
}

// TestSessionUsecase_DeleteFlushesPendingQueue：DB 侧 cascadeDeleteBySession
// 已清 pending_queue_entries（FR-7）；内存队列必须同步清空，否则批量
// ReplaceAll 把已删会话条目插回（FK 违例/幽灵行复活）。
func TestSessionUsecase_DeleteFlushesPendingQueue(t *testing.T) {
	repo := &mockSessionRepo{
		getSessionByIDFn: func(_ context.Context, id string) (Session, error) {
			return Session{ID: id, AgentID: "agent-1", Status: string(SessionStatusCompleted)}, nil
		},
		deleteSessionFn: func(context.Context, string) (int, error) { return 1, nil },
	}

	var flushed []string
	uc := newTestUsecase(repo, &mockAgentLookup{}, &mockTeamLookup{})
	uc.SetPendingQueueFlusher(func(sessionID string) { flushed = append(flushed, sessionID) })

	if err := uc.Delete(context.Background(), "s-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(flushed) != 1 || flushed[0] != "s-1" {
		t.Fatalf("P1 FR-7: delete must flush the session pending queue, got %v", flushed)
	}
}

// TestSessionUsecase_ProtectedSessionDoesNotFlush 对照组：保护态会话被
// Archive/Delete 拒绝时不触发任何清理（内存队列与用户可恢复状态保持
// 一致——拒绝即无副作用）。
func TestSessionUsecase_ProtectedSessionDoesNotFlush(t *testing.T) {
	repo := &mockSessionRepo{
		getSessionByIDFn: func(_ context.Context, id string) (Session, error) {
			return Session{ID: id, AgentID: "agent-1", Status: string(SessionStatusRunning)}, nil
		},
	}

	var flushed []string
	uc := newTestUsecase(repo, &mockAgentLookup{}, &mockTeamLookup{})
	uc.SetPendingQueueFlusher(func(sessionID string) { flushed = append(flushed, sessionID) })

	if err := uc.Archive(context.Background(), "s-1"); err == nil {
		t.Fatal("protected session must not archive")
	}
	if err := uc.Delete(context.Background(), "s-1"); err == nil {
		t.Fatal("protected session must not delete")
	}
	if len(flushed) != 0 {
		t.Fatalf("P1 FR-7: rejected archive/delete must not flush, got %v", flushed)
	}
}
