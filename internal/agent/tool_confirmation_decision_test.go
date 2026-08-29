package agent

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	serviceawaitreply "aranea-agents/internal/tools/serviceawaitreply"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// newDecisionTestHook builds a confirmation hook whose gate requires
// confirmation for "bash", backed by the capture collector.
func newDecisionTestHook(cc decision.Collector) *toolConfirmationBeforeHook {
	deps := TRPCBuilderDeps{}
	deps.DecisionCollector = cc
	gate := &toolConfirmGate{
		catalog:       map[string]confirmCatalogEntry{"bash": {requiresConfirm: true}},
		sessionGrants: newToolGrantStore(time.Now),
	}
	return newToolConfirmationBeforeHook(gate, biz.Agent{ID: "agent-1", AgentKey: "spirit"}, deps)
}

func decisionTestCtx(sessionID, userID string, reply serviceawaitreply.ReplyFunc) context.Context {
	var ctx context.Context = trpcagent.NewInvocationContext(context.Background(), &trpcagent.Invocation{
		Session: &trpcsession.Session{ID: sessionID, UserID: userID},
	})
	if reply != nil {
		ctx = serviceawaitreply.WithReplyFunc(ctx, reply)
	}
	return ctx
}

func beforeToolArgs() *trpctool.BeforeToolArgs {
	return &trpctool.BeforeToolArgs{
		ToolName:   "bash",
		Arguments:  []byte(`{"command":"rm -rf /tmp/x"}`),
		ToolCallID: "tc-1",
	}
}

// TestToolConfirmDecision_Approve pins the M80 1.5 mapping for the approve
// branch (design §3.2 row 2): hitl_approval / approved / human actor /
// source_ref.tool_invocation_id / metadata.decision_reason + grant_scope.
func TestToolConfirmDecision_Approve(t *testing.T) {
	cc := &captureDecisionCollector{}
	h := newDecisionTestHook(cc)
	ctx := decisionTestCtx("sess-1", "u-1", func(context.Context) (string, error) {
		return serviceawaitreply.ReplyApprove, nil
	})

	if _, err := h.HandleBeforeTool(ctx, beforeToolArgs()); err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if len(cc.recs) != 1 {
		t.Fatalf("expected 1 decision record, got %d", len(cc.recs))
	}
	r := cc.recs[0]
	if r.Category != decision.CategoryHITLApproval {
		t.Errorf("category = %q, want hitl_approval", r.Category)
	}
	if r.Outcome != "approved" {
		t.Errorf("outcome = %q, want approved", r.Outcome)
	}
	if r.ActorType != decision.ActorHuman {
		t.Errorf("actor_type = %q, want human", r.ActorType)
	}
	if r.ActorKey != "u-1" {
		t.Errorf("actor_key = %q, want u-1 (session user)", r.ActorKey)
	}
	if r.SourceRef.ToolInvocationID != "tc-1" {
		t.Errorf("source_ref.tool_invocation_id = %q, want tc-1", r.SourceRef.ToolInvocationID)
	}
	if r.SourceRef.SessionID != "sess-1" {
		t.Errorf("source_ref.session_id = %q, want sess-1", r.SourceRef.SessionID)
	}
	if got := r.Metadata["session_id"]; got != "sess-1" {
		t.Errorf("metadata.session_id = %v, want sess-1", got)
	}
	if got := r.Metadata["decision_reason"]; got == nil || got == "" {
		t.Errorf("metadata.decision_reason missing: %v", r.Metadata)
	}
	if got := r.Metadata["grant_scope"]; got != "once" {
		t.Errorf("metadata.grant_scope = %v, want once", got)
	}
	if r.Scenario == "" {
		t.Error("scenario must be set (validation requires non-empty)")
	}
	// tool entity must be linked for Phase-2 precedent retrieval.
	foundTool := false
	for _, e := range r.RelatedEntities {
		if e.Type == "tool" && e.Key == "bash" {
			foundTool = true
		}
	}
	if !foundTool {
		t.Errorf("related_entities must include tool:bash, got %v", r.RelatedEntities)
	}
}

// TestToolConfirmDecision_ApproveSessionScope checks the grant-scope mapping
// for session-scoped approvals.
func TestToolConfirmDecision_ApproveSessionScope(t *testing.T) {
	cc := &captureDecisionCollector{}
	h := newDecisionTestHook(cc)
	ctx := decisionTestCtx("sess-1", "u-1", func(context.Context) (string, error) {
		return serviceawaitreply.ReplyApproveSession, nil
	})

	if _, err := h.HandleBeforeTool(ctx, beforeToolArgs()); err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if len(cc.recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(cc.recs))
	}
	if got := cc.recs[0].Metadata["grant_scope"]; got != "session" {
		t.Errorf("grant_scope = %v, want session", got)
	}
}

// TestToolConfirmDecision_Deny covers the user-rejection branch.
func TestToolConfirmDecision_Deny(t *testing.T) {
	cc := &captureDecisionCollector{}
	h := newDecisionTestHook(cc)
	ctx := decisionTestCtx("sess-1", "u-2", func(context.Context) (string, error) {
		return serviceawaitreply.ReplyDeny, nil
	})

	if _, err := h.HandleBeforeTool(ctx, beforeToolArgs()); err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if len(cc.recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(cc.recs))
	}
	r := cc.recs[0]
	if r.Outcome != "rejected" {
		t.Errorf("outcome = %q, want rejected", r.Outcome)
	}
	if r.ActorKey != "u-2" {
		t.Errorf("actor_key = %q, want u-2", r.ActorKey)
	}
}

// TestToolConfirmDecision_Timeout covers the deadline-expiry branch: the
// outcome must be timeout (not rejected) so audits can tell "no response"
// apart from a deliberate deny.
func TestToolConfirmDecision_Timeout(t *testing.T) {
	cc := &captureDecisionCollector{}
	h := newDecisionTestHook(cc)
	h.confirmTimeout = 30 * time.Millisecond
	ctx := decisionTestCtx("sess-1", "u-1", func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})

	if _, err := h.HandleBeforeTool(ctx, beforeToolArgs()); err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if len(cc.recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(cc.recs))
	}
	if r := cc.recs[0]; r.Outcome != "timeout" {
		t.Errorf("outcome = %q, want timeout", r.Outcome)
	}
}

// TestToolConfirmDecision_NoReplyChannel covers the environment-blocked
// branch: recorded as rejected with an explicit block_cause marker.
func TestToolConfirmDecision_NoReplyChannel(t *testing.T) {
	cc := &captureDecisionCollector{}
	h := newDecisionTestHook(cc)
	ctx := decisionTestCtx("sess-1", "u-1", nil) // no reply func

	if _, err := h.HandleBeforeTool(ctx, beforeToolArgs()); err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if len(cc.recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(cc.recs))
	}
	r := cc.recs[0]
	if r.Outcome != "rejected" {
		t.Errorf("outcome = %q, want rejected", r.Outcome)
	}
	if got := r.Metadata["block_cause"]; got != "no_reply_channel" {
		t.Errorf("metadata.block_cause = %v, want no_reply_channel", got)
	}
}

// TestToolConfirmDecision_NilCollector keeps backwards compatibility: hooks
// built without the M80 collector must behave exactly as before.
func TestToolConfirmDecision_NilCollector(t *testing.T) {
	h := newDecisionTestHook(nil)
	ctx := decisionTestCtx("sess-1", "u-1", func(context.Context) (string, error) {
		return serviceawaitreply.ReplyApprove, nil
	})
	if _, err := h.HandleBeforeTool(ctx, beforeToolArgs()); err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
}

// TestToolConfirmDecision_UnknownUserFallback: sessions without a user id
// must still validate (actor_key is required) — fall back to "unknown".
func TestToolConfirmDecision_UnknownUserFallback(t *testing.T) {
	cc := &captureDecisionCollector{}
	h := newDecisionTestHook(cc)
	ctx := decisionTestCtx("sess-1", "", func(context.Context) (string, error) {
		return serviceawaitreply.ReplyApprove, nil
	})
	if _, err := h.HandleBeforeTool(ctx, beforeToolArgs()); err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if len(cc.recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(cc.recs))
	}
	if r := cc.recs[0]; r.ActorKey != "unknown" {
		t.Errorf("actor_key = %q, want unknown fallback", r.ActorKey)
	}
}

// TestToolConfirmDecision_GrantSkip pins F12：grant（session/persisted）放行
// 必须落 hitl_approval 决策记录（outcome=grant_skip）。S07 Evidence：同参
// spawn 前 2 次超时 blocked、首次 approve 后第 3 次 5ms 放行——放行符合批量
// 授权设计，但决策层无迹可寻会被误判为确认闸策略不一致。
func TestToolConfirmDecision_GrantSkip(t *testing.T) {
	cc := &captureDecisionCollector{}
	h := newDecisionTestHook(cc)
	h.gate.sessionGrants.GrantSession("sess-1", "agent-1", "bash")
	ctx := decisionTestCtx("sess-1", "u-1", nil) // grant 命中，不进回复通道

	if _, err := h.HandleBeforeTool(ctx, beforeToolArgs()); err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if len(cc.recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(cc.recs))
	}
	r := cc.recs[0]
	if r.Outcome != "grant_skip" {
		t.Errorf("outcome = %q, want grant_skip", r.Outcome)
	}
	if got := r.Metadata["decision_reason"]; got != confirmReasonGrantSession {
		t.Errorf("decision_reason = %v, want grant_session", got)
	}
}

// TestToolConfirmDecision_DefaultAllowNoRecord pins the counterpart:
// default_allow / shell_safe 等免确认放行不是人工授权事件，不得污染
// hitl_approval 决策层。
func TestToolConfirmDecision_DefaultAllowNoRecord(t *testing.T) {
	cc := &captureDecisionCollector{}
	h := newDecisionTestHook(cc)
	ctx := decisionTestCtx("sess-1", "u-1", nil)
	args := beforeToolArgs()
	args.ToolName = "datetime" // 不在 catalog，default_allow

	if _, err := h.HandleBeforeTool(ctx, args); err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if len(cc.recs) != 0 {
		t.Fatalf("default_allow must not emit hitl_approval, got %d records", len(cc.recs))
	}
}
