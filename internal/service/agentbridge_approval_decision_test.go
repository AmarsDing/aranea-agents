package service

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz/agentbridge"
	"aranea-agents/internal/biz/decision"
)

// captureDecisionCollector is the test double for decision.Collector.
type captureDecisionCollector struct {
	recs []decision.Record
}

func (c *captureDecisionCollector) Emit(_ context.Context, r decision.Record) {
	c.recs = append(c.recs, r)
}

// dispatchPermissionTask drives one task into the OnPermission wait and
// returns the task ID; the permission resolves when the test confirms it.
func dispatchPermissionTask(t *testing.T, e *abSvcEnv, opts []agentbridge.PermissionOption) string {
	t.Helper()
	started := make(chan struct{})
	e.factory.sessions = []*abFakeSession{{
		promptFn: func(ctx context.Context, _, _ string, h agentbridge.EventHandler) (string, error) {
			close(started)
			id, err := h.OnPermission(ctx, "rm -rf build/", opts)
			if err != nil {
				return "", err
			}
			_ = id
			return "ok", nil
		},
	}}
	res, err := e.svc.DispatchTask(context.Background(), "sess-1", "codex", "aranea", "清理构建目录")
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	<-started
	e.bus.waitNoticeType(t, noticeCodingTaskApproval, 1)
	return res.Task.ID
}

var bridgePermOpts = []agentbridge.PermissionOption{
	{OptionID: "allow", Name: "允许", Kind: "allow_once"},
	{OptionID: "always", Name: "总是允许", Kind: "allow_always"},
	{OptionID: "deny", Name: "拒绝", Kind: "reject_once"},
}

// TestAgentBridgeDecision_Approve pins the M80 1.5 AgentBridge-chain mapping
// (design §3.2 row 2): approve → hitl_approval/approved, source_ref.task_id.
func TestAgentBridgeDecision_Approve(t *testing.T) {
	e := newABSvcEnv()
	e.seedAgent("codex")
	e.seedProject("aranea", `F:\aranea`)
	cc := &captureDecisionCollector{}
	e.svc.SetDecisionCollector(cc)

	taskID := dispatchPermissionTask(t, e, bridgePermOpts)
	if err := e.svc.ConfirmBridgePermission(context.Background(), taskID, agentbridge.DecisionApprove); err != nil {
		t.Fatalf("confirm: %v", err)
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
	if r.SourceRef.TaskID != taskID {
		t.Errorf("source_ref.task_id = %q, want %s", r.SourceRef.TaskID, taskID)
	}
	if r.ActorType != decision.ActorHuman {
		t.Errorf("actor_type = %q, want human", r.ActorType)
	}
	if got := r.Metadata["grant_scope"]; got != "once" {
		t.Errorf("metadata.grant_scope = %v, want once", got)
	}
	foundTool := false
	for _, en := range r.RelatedEntities {
		if en.Type == "tool" && en.Key == agentbridge.ToolExternalCoding {
			foundTool = true
		}
	}
	if !foundTool {
		t.Errorf("related_entities must include tool:external_coding, got %v", r.RelatedEntities)
	}
}

// TestAgentBridgeDecision_Deny covers the reject resolution.
func TestAgentBridgeDecision_Deny(t *testing.T) {
	e := newABSvcEnv()
	e.seedAgent("codex")
	e.seedProject("aranea", `F:\aranea`)
	cc := &captureDecisionCollector{}
	e.svc.SetDecisionCollector(cc)

	taskID := dispatchPermissionTask(t, e, bridgePermOpts)
	if err := e.svc.ConfirmBridgePermission(context.Background(), taskID, agentbridge.DecisionDeny); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if len(cc.recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(cc.recs))
	}
	if r := cc.recs[0]; r.Outcome != "rejected" {
		t.Errorf("outcome = %q, want rejected", r.Outcome)
	}
}

// TestAgentBridgeDecision_AlwaysGrantScope: allow_always maps to approved
// with grant_scope=always (task-scoped cache, not a persisted grant).
func TestAgentBridgeDecision_AlwaysGrantScope(t *testing.T) {
	e := newABSvcEnv()
	e.seedAgent("codex")
	e.seedProject("aranea", `F:\aranea`)
	cc := &captureDecisionCollector{}
	e.svc.SetDecisionCollector(cc)

	taskID := dispatchPermissionTask(t, e, bridgePermOpts)
	if err := e.svc.ConfirmBridgePermission(context.Background(), taskID, agentbridge.DecisionAlways); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if len(cc.recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(cc.recs))
	}
	r := cc.recs[0]
	if r.Outcome != "approved" {
		t.Errorf("outcome = %q, want approved", r.Outcome)
	}
	if got := r.Metadata["grant_scope"]; got != "always" {
		t.Errorf("metadata.grant_scope = %v, want always", got)
	}
}

// TestAgentBridgeDecision_Timeout: approval deadline expiry is recorded as
// outcome=timeout so audits can distinguish it from a deliberate deny.
func TestAgentBridgeDecision_Timeout(t *testing.T) {
	e := newABSvcEnv()
	e.seedAgent("codex")
	e.seedProject("aranea", `F:\aranea`)
	e.svc.SetApprovalTimeout(30 * time.Millisecond)
	cc := &captureDecisionCollector{}
	e.svc.SetDecisionCollector(cc)

	done := make(chan error, 1)
	e.factory.sessions = []*abFakeSession{{
		promptFn: func(ctx context.Context, _, _ string, h agentbridge.EventHandler) (string, error) {
			_, err := h.OnPermission(ctx, "rm -rf build/", bridgePermOpts)
			done <- err
			return "", err
		},
	}}
	if _, err := e.svc.DispatchTask(context.Background(), "sess-1", "codex", "aranea", "清理构建目录"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("OnPermission did not time out")
	}
	if len(cc.recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(cc.recs))
	}
	if r := cc.recs[0]; r.Outcome != "timeout" {
		t.Errorf("outcome = %q, want timeout", r.Outcome)
	}
}
