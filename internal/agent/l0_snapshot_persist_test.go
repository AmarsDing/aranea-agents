package agent

import (
	"encoding/json"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/llmcontext"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestL0NeedsWindowRecheck(t *testing.T) {
	settings := &biz.AgentRuntimeSettings{L0SnapshotMode: "on_warning"}
	if l0NeedsWindowRecheck(settings, 0.2, false, 128000) {
		t.Fatal("low ratio should not recheck")
	}
	if !l0NeedsWindowRecheck(settings, 0.5, false, 128000) {
		t.Fatal("borderline ratio should recheck")
	}
	if l0NeedsWindowRecheck(settings, 0.5, true, 128000) {
		t.Fatal("force debug skips recheck")
	}
	settings.L0SnapshotMode = "always"
	if l0NeedsWindowRecheck(settings, 0.5, false, 128000) {
		t.Fatal("always mode skips recheck")
	}
}

func TestL0SnapshotPendingForCall(t *testing.T) {
	inv := &trpcagent.Invocation{}
	setL0SnapshotPendingForCall(inv, 1, "snap-a", 32000)
	setL0SnapshotPendingForCall(inv, 2, "snap-b", 64000)

	p1, ok := l0SnapshotPendingForCall(inv, 1)
	if !ok || p1.ID != "snap-a" || p1.Window != 32000 {
		t.Fatalf("call 1: %#v ok=%v", p1, ok)
	}
	p2, ok := l0SnapshotPendingForCall(inv, 2)
	if !ok || p2.ID != "snap-b" {
		t.Fatalf("call 2: %#v ok=%v", p2, ok)
	}
	if _, ok := l0SnapshotPendingForCall(inv, 3); ok {
		t.Fatal("unexpected call 3")
	}
}

func TestL0GateContextWindow(t *testing.T) {
	if got := l0GateContextWindow(biz.Agent{ContextWindow: 48000}); got != 48000 {
		t.Fatalf("agent window: %d", got)
	}
	if got := l0GateContextWindow(biz.Agent{}); got != llmcontext.DefaultWindowTokens {
		t.Fatalf("default window: %d", got)
	}
}

func TestCountBulletLines_respectsEndMarker(t *testing.T) {
	text := "## L2+L3 memory\n- a\n- b\n## L4 knowledge graph\n- c\n"
	if n := countBulletLines(text, "## L2+L3 memory"); n != 2 {
		t.Fatalf("bullets: %d", n)
	}
}

func TestL0CrossedThreshold(t *testing.T) {
	if l0CrossedThreshold(0.70, 0.85, 0.80) {
		// crossed from below to above
	} else {
		t.Fatal("expected threshold crossing")
	}
	if l0CrossedThreshold(0.85, 0.90, 0.80) {
		t.Fatal("already above threshold, not a crossing")
	}
	if l0CrossedThreshold(0.70, 0.75, 0.80) {
		t.Fatal("did not reach threshold")
	}
	if l0CrossedThreshold(0, 0.85, 0.80) {
		// first write crosses threshold
	} else {
		t.Fatal("first write crossing should be detected")
	}
}

func TestL0SnapshotThrottleAllows(t *testing.T) {
	inv := &trpcagent.Invocation{}

	// First write: no throttle state, should allow
	if !l0SnapshotThrottleAllows(inv, 0.70, false, "on_warning") {
		t.Fatal("first write should be allowed")
	}

	// Record a write
	l0SnapshotThrottleRecord(inv, 0.70)

	// Same ratio, within interval: should be blocked
	if l0SnapshotThrottleAllows(inv, 0.72, false, "on_warning") {
		t.Fatal("same ratio within interval should be blocked")
	}

	// Force mode bypasses throttle
	if !l0SnapshotThrottleAllows(inv, 0.72, true, "on_warning") {
		t.Fatal("force mode should bypass throttle")
	}

	// Low ratio: should be blocked regardless
	if l0SnapshotThrottleAllows(inv, 0.50, false, "on_warning") {
		t.Fatal("low ratio should be blocked")
	}

	// Threshold crossing: should be allowed even within interval
	if !l0SnapshotThrottleAllows(inv, 0.85, false, "on_warning") {
		t.Fatal("threshold crossing should be allowed")
	}

	// "always" mode bypasses throttle
	if !l0SnapshotThrottleAllows(inv, 0.50, false, "always") {
		t.Fatal("always mode should bypass throttle")
	}
}

func TestL0SnapshotThrottleAllows_intervalElapsed(t *testing.T) {
	inv := &trpcagent.Invocation{}
	// Simulate a write 400 seconds ago
	inv.SetState(l0SnapshotThrottleStateKey, l0SnapshotThrottleState{
		LastWriteAt: time.Now().Add(-400 * time.Second),
		LastRatio:   0.70,
	})
	// Interval elapsed, ratio delta sufficient
	if !l0SnapshotThrottleAllows(inv, 0.85, false, "on_warning") {
		t.Fatal("interval elapsed + ratio delta should be allowed")
	}
}

func TestL0SnapshotThrottleAllows_ratioDeltaInsufficient(t *testing.T) {
	inv := &trpcagent.Invocation{}
	// Simulate a write 400 seconds ago
	inv.SetState(l0SnapshotThrottleStateKey, l0SnapshotThrottleState{
		LastWriteAt: time.Now().Add(-400 * time.Second),
		LastRatio:   0.70,
	})
	// Interval elapsed but ratio delta too small
	if l0SnapshotThrottleAllows(inv, 0.74, false, "on_warning") {
		t.Fatal("ratio delta too small should be blocked")
	}
}

func TestBuildL0SegmentsSummaryJSON(t *testing.T) {
	messages := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: "You are a helpful assistant."},
		{Role: trpcmodel.RoleSystem, Content: "## L1 working memory\n- key: value\n- task: test"},
		{Role: trpcmodel.RoleUser, Content: "Hello"},
		{Role: trpcmodel.RoleAssistant, Content: "Hi there!"},
		{Role: trpcmodel.RoleTool, Content: `{"result": "ok"}`},
	}

	result := buildL0SegmentsSummaryJSON(messages)
	var parsed map[string]map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("invalid JSON: %s err=%v", result, err)
	}

	// Check system.prompt section exists
	if _, ok := parsed["system.prompt"]; !ok {
		t.Fatal("expected system.prompt section")
	}
	// Check memory.l1 section exists with field_count
	l1, ok := parsed["memory.l1"]
	if !ok {
		t.Fatal("expected memory.l1 section")
	}
	if fc, _ := l1["field_count"].(float64); fc != 2 {
		t.Fatalf("expected field_count=2, got %v", l1["field_count"])
	}
	// Check user.input section exists
	if _, ok := parsed["user.input"]; !ok {
		t.Fatal("expected user.input section")
	}
	// Check history section exists
	if _, ok := parsed["history"]; !ok {
		t.Fatal("expected history section")
	}
	// Check tool.result section exists
	if _, ok := parsed["tool.result"]; !ok {
		t.Fatal("expected tool.result section")
	}
}
