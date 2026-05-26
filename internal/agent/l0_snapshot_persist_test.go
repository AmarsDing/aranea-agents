package agent

import (
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/llmcontext"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
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
