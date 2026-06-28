package team

import (
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestGraphRunStepContext_dedup(t *testing.T) {
	ctx := buildGraphRunStepContext(`{"mode":"sequential","members":[{"agent_id":"a1","sort_order":1}]}`, "hello", "run-1", "team-1", "sess-1", "sess-1", loggateway.NewNoop())
	if ctx == nil {
		t.Fatal("nil context")
	}
	if ctx.AlreadyPersisted("member-1") {
		t.Fatal("expected fresh")
	}
	ctx.MarkPersisted("member-1")
	if !ctx.AlreadyPersisted("member-1") {
		t.Fatal("expected marked")
	}
	m, ok := ctx.MemberDefForNode("member-1")
	if !ok || m.AgentID != "a1" {
		t.Fatalf("member=%+v ok=%v", m, ok)
	}
}

func TestGraphRunStepPolicy_nativeUsesBulkGraphUsesEvents(t *testing.T) {
	// Documents TG-RT-PARITY step policy: Native bulk-persists; Graph uses event watch + anchor fallback.
	if graphWatchStepsOnly == graphWatchStepsAndFinalize {
		t.Fatal("watch modes must differ")
	}
}
