package session

// session_attachment_purge_test.go — T-3 附件生命周期（2026-10-26 评审
// 二轮）：会话删除级联清理附件的接线钉板。

import (
	"context"
	"fmt"
	"testing"
)

// ① Delete 成功后调用注入的附件 purger（携带调用方 ctx 与会话 id）。
func TestSessionUsecase_DeletePurgesAttachments(t *testing.T) {
	repo := &mockSessionRepo{
		getSessionByIDFn: func(_ context.Context, id string) (Session, error) {
			return Session{ID: id, AgentID: "agent-1", Status: string(SessionStatusCompleted)}, nil
		},
		deleteSessionFn: func(context.Context, string) (int, error) { return 1, nil },
	}

	var purged []string
	uc := newTestUsecase(repo, &mockAgentLookup{}, &mockTeamLookup{})
	uc.SetAttachmentPurger(func(_ context.Context, sessionID string) error {
		purged = append(purged, sessionID)
		return nil
	})

	if err := uc.Delete(context.Background(), "s-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(purged) != 1 || purged[0] != "s-1" {
		t.Fatalf("T-3: delete must purge session attachments, got %v", purged)
	}

	// 未注入 purger 时不得 panic（测试/CLI 形态默认）。
	if err := newTestUsecase(repo, &mockAgentLookup{}, &mockTeamLookup{}).Delete(context.Background(), "s-1"); err != nil {
		t.Fatalf("delete without purger: %v", err)
	}
}

// ② Archive 不触发附件清理——归档是可恢复冻结，附件必须随会话存活。
func TestSessionUsecase_ArchiveKeepsAttachments(t *testing.T) {
	repo := &mockSessionRepo{
		getSessionByIDFn: func(_ context.Context, id string) (Session, error) {
			return Session{ID: id, AgentID: "agent-1", Status: string(SessionStatusCompleted)}, nil
		},
		archiveSessionFn: func(context.Context, string) (int, error) { return 1, nil },
	}

	var purged []string
	uc := newTestUsecase(repo, &mockAgentLookup{}, &mockTeamLookup{})
	uc.SetAttachmentPurger(func(_ context.Context, sessionID string) error {
		purged = append(purged, sessionID)
		return nil
	})

	if err := uc.Archive(context.Background(), "s-1"); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if len(purged) != 0 {
		t.Fatalf("T-3: archive must NOT purge attachments (restorable freeze), got %v", purged)
	}
}

// ③ purger 失败只告警不回滚——会话已软删，删除主流程返回成功。
func TestSessionUsecase_PurgeFailureDoesNotFailDelete(t *testing.T) {
	repo := &mockSessionRepo{
		getSessionByIDFn: func(_ context.Context, id string) (Session, error) {
			return Session{ID: id, AgentID: "agent-1", Status: string(SessionStatusCompleted)}, nil
		},
		deleteSessionFn: func(context.Context, string) (int, error) { return 1, nil },
	}

	uc := newTestUsecase(repo, &mockAgentLookup{}, &mockTeamLookup{})
	uc.SetAttachmentPurger(func(context.Context, string) error { return fmt.Errorf("disk gone") })

	if err := uc.Delete(context.Background(), "s-1"); err != nil {
		t.Fatalf("T-3: purge failure must not fail the delete, got %v", err)
	}
}

// ④ 保护态会话删除被拒时不触发附件清理（与 FR-7 对照组同语义）。
func TestSessionUsecase_ProtectedSessionDoesNotPurge(t *testing.T) {
	repo := &mockSessionRepo{
		getSessionByIDFn: func(_ context.Context, id string) (Session, error) {
			return Session{ID: id, AgentID: "agent-1", Status: string(SessionStatusRunning)}, nil
		},
	}

	var purged []string
	uc := newTestUsecase(repo, &mockAgentLookup{}, &mockTeamLookup{})
	uc.SetAttachmentPurger(func(_ context.Context, sessionID string) error {
		purged = append(purged, sessionID)
		return nil
	})

	if err := uc.Delete(context.Background(), "s-1"); err == nil {
		t.Fatal("protected session must not delete")
	}
	if len(purged) != 0 {
		t.Fatalf("T-3: rejected delete must not purge, got %v", purged)
	}
}
