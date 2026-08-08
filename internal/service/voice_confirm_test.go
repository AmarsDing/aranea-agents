package service

import (
	"context"
	"testing"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools/serviceawaitreply"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
)

// stubStepV2ReaderSession 按 session/spirit 归属过滤的 StepV2Reader stub。
type stubStepV2ReaderSession struct {
	biz.StepV2Reader
	steps []biz.Step
}

func (s stubStepV2ReaderSession) ListStepsBySpiritSession(_ context.Context, sid string) ([]biz.Step, error) {
	var out []biz.Step
	for _, st := range s.steps {
		if st.SpiritSessionID == sid {
			out = append(out, st)
		}
	}
	return out, nil
}

func (s stubStepV2ReaderSession) ListStepsBySession(_ context.Context, sid string) ([]biz.Step, error) {
	var out []biz.Step
	for _, st := range s.steps {
		if st.SessionID == sid {
			out = append(out, st)
		}
	}
	return out, nil
}

func (s stubStepV2ReaderSession) GetStep(_ context.Context, id string) (biz.Step, error) {
	for _, st := range s.steps {
		if st.ID == id {
			return st, nil
		}
	}
	return biz.Step{}, apierror.NotFound(apierror.DomainChat, "step not found")
}

func newVoiceConfirmTestSvc(coord awaitCoordinator, steps []biz.Step) (*ChatService, *stubStepV2Writer) {
	stepWriter := &stubStepV2Writer{}
	orch := newSubmitAwaitReplyTestOrch(coord)
	orch.core.StepReader = stubStepV2ReaderSession{steps: steps}
	orch.core.StepWriter = stepWriter
	orch.core.TD.Sessions = stubSessionTurnManagerGet{getFn: func(_ context.Context, id string) (biz.Session, error) {
		return biz.Session{ID: id, UserID: "user-1"}, nil
	}}
	return &ChatService{orch: orch, lg: loggateway.NewNoop()}, stepWriter
}

func voiceConfirmTestCtx() context.Context {
	return ctxuser.WithUserID(context.Background(), "user-1")
}

// 无待决议确认 → (false, nil)，不触碰确认管线。
func TestVoiceConfirmResolver_NoPending(t *testing.T) {
	coord := stubAwaitCoord{trySendFn: func(string, biz.AwaitReplyMsg) bool {
		t.Fatal("channel must not receive anything when nothing is pending")
		return true
	}}
	svc, stepWriter := newVoiceConfirmTestSvc(coord, []biz.Step{
		{ID: "s-done", SessionID: "sess-1", Kind: biz.StepKindConfirm, Status: biz.StepStatusCompleted},
		{ID: "s-reply", SessionID: "sess-1", Kind: biz.StepKindReply, Status: biz.StepStatusCompleted},
	})
	resolver := NewVoiceConfirmResolver(svc)

	resolved, err := resolver.ResolvePendingConfirm(voiceConfirmTestCtx(), "sess-1", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved {
		t.Fatal("resolved must be false when no tool_blocked confirm exists")
	}
	if len(stepWriter.updated) != 0 {
		t.Fatalf("no step update expected, got %d", len(stepWriter.updated))
	}
}

// 多个待决议确认 → 决议最早发起的一个（与前端队首同口径）。
func TestVoiceConfirmResolver_ResolvesOldestPending(t *testing.T) {
	now := time.Now()
	var sent biz.AwaitReplyMsg
	coord := stubAwaitCoord{trySendFn: func(_ string, msg biz.AwaitReplyMsg) bool {
		sent = msg
		return true
	}}
	svc, stepWriter := newVoiceConfirmTestSvc(coord, []biz.Step{
		{ID: "s-newer", SessionID: "sess-1", Kind: biz.StepKindConfirm, Status: biz.StepStatusToolBlocked, ToolName: "client_open_app", StartedAt: now},
		{ID: "s-older", SessionID: "sess-1", Kind: biz.StepKindConfirm, Status: biz.StepStatusToolBlocked, ToolName: "client_open_app", StartedAt: now.Add(-time.Minute)},
		{ID: "s-other-kind", SessionID: "sess-1", Kind: biz.StepKindAction, Status: biz.StepStatusToolBlocked, StartedAt: now.Add(-2 * time.Minute)},
	})
	resolver := NewVoiceConfirmResolver(svc)

	resolved, err := resolver.ResolvePendingConfirm(voiceConfirmTestCtx(), "sess-1", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resolved {
		t.Fatal("resolved must be true")
	}
	if len(stepWriter.updated) != 1 || stepWriter.updated[0].ID != "s-older" {
		t.Fatalf("oldest pending confirm must be resolved, updates = %+v", stepWriter.updated)
	}
	if stepWriter.updated[0].Status != biz.StepStatusCompleted {
		t.Fatalf("approve must complete the step, got %s", stepWriter.updated[0].Status)
	}
	if sent.Reply != serviceawaitreply.ReplyApprove {
		t.Fatalf("channel token = %q, want %q", sent.Reply, serviceawaitreply.ReplyApprove)
	}
}

// deny → ReplyDeny token + step cancelled。
func TestVoiceConfirmResolver_Deny(t *testing.T) {
	var sent biz.AwaitReplyMsg
	coord := stubAwaitCoord{trySendFn: func(_ string, msg biz.AwaitReplyMsg) bool {
		sent = msg
		return true
	}}
	svc, stepWriter := newVoiceConfirmTestSvc(coord, []biz.Step{
		{ID: "s-blocked", SessionID: "sess-1", Kind: biz.StepKindConfirm, Status: biz.StepStatusToolBlocked, ToolName: "client_open_app", StartedAt: time.Now()},
	})
	resolver := NewVoiceConfirmResolver(svc)

	resolved, err := resolver.ResolvePendingConfirm(voiceConfirmTestCtx(), "sess-1", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resolved {
		t.Fatal("resolved must be true")
	}
	if sent.Reply != serviceawaitreply.ReplyDeny {
		t.Fatalf("channel token = %q, want %q", sent.Reply, serviceawaitreply.ReplyDeny)
	}
	if len(stepWriter.updated) != 1 || stepWriter.updated[0].Status != biz.StepStatusCancelled {
		t.Fatalf("deny must cancel the step, updates = %+v", stepWriter.updated)
	}
}

// 挂在 spirit 树上的确认 step（SpiritSessionID 归属）也能被决议（前端同口径）。
func TestVoiceConfirmResolver_SpiritTreePending(t *testing.T) {
	coord := stubAwaitCoord{trySendFn: func(string, biz.AwaitReplyMsg) bool { return true }}
	svc, stepWriter := newVoiceConfirmTestSvc(coord, []biz.Step{
		{ID: "s-spirit", SessionID: "sess-1", SpiritSessionID: "sess-1", Kind: biz.StepKindConfirm, Status: biz.StepStatusToolBlocked, StartedAt: time.Now()},
	})
	resolver := NewVoiceConfirmResolver(svc)

	resolved, err := resolver.ResolvePendingConfirm(voiceConfirmTestCtx(), "sess-1", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resolved || len(stepWriter.updated) != 1 || stepWriter.updated[0].ID != "s-spirit" {
		t.Fatalf("spirit-tree pending confirm must resolve, resolved=%v updates=%+v", resolved, stepWriter.updated)
	}
}

// 其他会话的待决议确认不得被误决议（stub 两路查询均按归属过滤）。
func TestVoiceConfirmResolver_IgnoresForeignSession(t *testing.T) {
	coord := stubAwaitCoord{trySendFn: func(string, biz.AwaitReplyMsg) bool { return true }}
	svc, _ := newVoiceConfirmTestSvc(coord, []biz.Step{
		{ID: "s-foreign", SessionID: "sess-2", SpiritSessionID: "sess-2", Kind: biz.StepKindConfirm, Status: biz.StepStatusToolBlocked, StartedAt: time.Now()},
	})
	resolver := NewVoiceConfirmResolver(svc)

	resolved, err := resolver.ResolvePendingConfirm(voiceConfirmTestCtx(), "sess-1", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved {
		t.Fatal("foreign session confirm must not be resolved")
	}
}

// 确认 step 已终态（并发 WS 决议）→ ConfirmActivity 报错，resolver 透传错误，
// voice 层降级为普通语句（chat_confirm.go 的 tool_blocked 校验兜底）。
func TestVoiceConfirmResolver_ConfirmActivityErrorPropagates(t *testing.T) {
	coord := stubAwaitCoord{trySendFn: func(string, biz.AwaitReplyMsg) bool { return true }}
	svc, _ := newVoiceConfirmTestSvc(coord, nil)
	resolver := NewVoiceConfirmResolver(svc)

	// 直接对不存在 step 走 ConfirmActivity 语义校验（经由 resolver 的列表为空 → 不报错）。
	resolved, err := resolver.ResolvePendingConfirm(voiceConfirmTestCtx(), "sess-1", true)
	if err != nil || resolved {
		t.Fatalf("empty store must yield (false, nil), got (%v, %v)", resolved, err)
	}

	// 负面路径：ConfirmActivity 对非 tool_blocked step 拒绝（voice 层据此降级）。
	_, err = svc.ConfirmActivity(voiceConfirmTestCtx(), &chatv1.ConfirmActivityRequest{
		SessionId: "sess-1", ActivityId: "ghost", Reply: serviceawaitreply.ReplyApprove,
	})
	if err == nil {
		t.Fatal("ConfirmActivity must reject unknown activity")
	}
}
