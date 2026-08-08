package team

import (
	"testing"
	"time"

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

// TestGraphNodeStartTracker_FirstWriteWins 锚定 2026-08-08 问题4b：节点重试
// 时保留最早 node_start，使持久化窗口覆盖全部尝试——与 MemberExecutionWindow
// 的成员 step 流最早 StartedAt 口径一致。
func TestGraphNodeStartTracker_FirstWriteWins(t *testing.T) {
	tr := newGraphNodeStartTracker()
	first := time.Date(2026, 8, 8, 1, 0, 0, 0, time.UTC)
	second := first.Add(5 * time.Second)
	tr.mark("node-a", first)
	tr.mark("node-a", second)
	got, ok := tr.get("node-a")
	if !ok {
		t.Fatal("expected tracked start")
	}
	if !got.Equal(first) {
		t.Fatalf("first-write-wins violated: got %s want %s", got, first)
	}
	if _, ok := tr.get("node-missing"); ok {
		t.Fatal("unknown node must report ok=false")
	}
}

// TestGraphNodeStartTracker_NilSafe 锚定 nil tracker 不 panic（standalone 路径
// 的 GraphRunStepContext 可能不挂 tracker）。
func TestGraphNodeStartTracker_NilSafe(t *testing.T) {
	var tr *graphNodeStartTracker
	tr.mark("node-a", time.Now())
	if _, ok := tr.get("node-a"); ok {
		t.Fatal("nil tracker must report ok=false")
	}
	ctx := &GraphRunStepContext{}
	ctx.MarkNodeStarted("node-a", time.Now())
	if _, ok := ctx.NodeStartedAt("node-a"); ok {
		t.Fatal("context without tracker must report ok=false")
	}
}

// TestGraphRunStepContext_NodeStartsSharedAcrossNotices 锚定会话级共享：
// handleGraphWatchNotice 每条 notice 都新建 stepCtx，node_start 标记必须能
// 被后续 node_end 的 stepCtx 读到——只有挂在 session 上才能跨 notice 传递。
func TestGraphRunStepContext_NodeStartsSharedAcrossNotices(t *testing.T) {
	sess := &teamGraphRunSession{}
	start := time.Date(2026, 8, 8, 1, 2, 3, 0, time.UTC)
	sess.stepContext().MarkNodeStarted("node-a", start)
	got, ok := sess.stepContext().NodeStartedAt("node-a")
	if !ok {
		t.Fatal("second stepContext must see first stepContext's mark")
	}
	if !got.Equal(start) {
		t.Fatalf("got %s want %s", got, start)
	}
}
