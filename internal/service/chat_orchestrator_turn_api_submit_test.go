package service

import (
	"context"
	"testing"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/loggateway"
)

// stubSteerPendingQueueManager 模拟「steer 成功」：accepted=true 但
// queued=false（内容注入在跑 run，未进 pending 队列）。
type stubSteerPendingQueueManager struct {
	noopPendingQueueManager
}

func (stubSteerPendingQueueManager) EnqueueUserMessage(string, string) (bool, bool, string, string, error) {
	return true, false, "", "", nil
}

// stubPendingEnqueueManager 模拟「进 pending 队列」：queued=true + pendingID。
type stubPendingEnqueueManager struct {
	noopPendingQueueManager
}

func (stubPendingEnqueueManager) EnqueueUserMessage(string, string) (bool, bool, string, string, error) {
	return true, true, "pid-1", "", nil
}

func newSubmitAsyncFixture(pendingQ pendingQueueManager) *ChatOrchestrator {
	reg := rt.NewRunRegistry()
	reg.StorePlaceholder("sess-1") // 活跃 run：触发同步入队分支
	return &ChatOrchestrator{
		core:      chatTurnCoreDeps{TD: rt.TurnDeps{Pipeline: rt.EventPipeline{}}},
		infraDeps: ChatInfraDeps{LG: loggateway.NewNoop()},
		runs:      reg,
		turnIdem:  newTurnIdemRegistry(),
		runMgr: &chatRunManagerImpl{
			runStatusTracker:    noopRunStatusTracker{},
			pendingQueueManager: pendingQ,
			awaitCoordinator:    noopAwaitCoordinator{},
			sessionRunLifecycle: noopSessionRunLifecycle{},
		},
	}
}

// TestSubmitChatMessageAsync_SteerAcceptedReturnsQueued（2026-10-29 split-brain
// 修复 pin）：steer 成功（accepted=true, queued=false）必须 ACK "queued" 且
// 立即返回——旧实现穿透到异步 Execute → 准入冲突二次入队（实测同一内容两条
// 「消息已加入队列」notice + ACK 误报 accepted）。
func TestSubmitChatMessageAsync_SteerAcceptedReturnsQueued(t *testing.T) {
	orch := newSubmitAsyncFixture(stubSteerPendingQueueManager{})

	resp, err := orch.submitChatMessageAsync(context.Background(), &chatv1.SendChatMessageRequest{
		SessionId: "sess-1",
		Content:   "follow up",
		RequestId: "req-steer-1",
	})
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}
	if !resp.GetAccepted() {
		t.Fatal("expected Accepted=true")
	}
	if got := resp.GetStatus(); got != "queued" {
		t.Fatalf("steer success must ACK queued, got %q", got)
	}
	// steer 无 pendingID 时回落预生成消息 ID。
	if resp.GetMessageId() == "" {
		t.Fatal("expected non-empty MessageId")
	}
}

// TestSubmitChatMessageAsync_PendingEnqueueReturnsQueued 既有行为 pin：
// 进 pending 队列 ACK "queued" 且携带 pendingID。
func TestSubmitChatMessageAsync_PendingEnqueueReturnsQueued(t *testing.T) {
	orch := newSubmitAsyncFixture(stubPendingEnqueueManager{})

	resp, err := orch.submitChatMessageAsync(context.Background(), &chatv1.SendChatMessageRequest{
		SessionId: "sess-1",
		Content:   "queued msg",
		RequestId: "req-pending-1",
	})
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}
	if got := resp.GetStatus(); got != "queued" {
		t.Fatalf("pending enqueue must ACK queued, got %q", got)
	}
	if resp.GetMessageId() != "pid-1" || resp.GetTurnId() != "pid-1" {
		t.Fatalf("expected pendingID as message/turn id, got %q/%q", resp.GetMessageId(), resp.GetTurnId())
	}
}
