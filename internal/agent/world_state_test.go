package agent

import (
	"context"
	"strings"
	"testing"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

func TestRenderWorldStateDiff(t *testing.T) {
	t.Parallel()
	prev := "- Effective tool keys this turn: read_file, save_file\n- Deny list: shell_exec"
	next := "- Effective tool keys this turn: read_file, save_file, mcp_broker\n- Deny list: shell_exec"
	got := renderWorldStateDiff(prev, next)
	if !strings.Contains(got, worldStateDiffOpen) {
		t.Fatalf("want diff envelope, got %q", got)
	}
	if !strings.Contains(got, "+ - Effective tool keys this turn: read_file, save_file, mcp_broker") {
		t.Fatalf("missing added line: %q", got)
	}
	if !strings.Contains(got, "- - Effective tool keys this turn: read_file, save_file") {
		t.Fatalf("missing removed line: %q", got)
	}
	if renderWorldStateDiff(next, next) != "" {
		t.Fatal("identical cues must not emit a diff")
	}
	if renderWorldStateDiff("", next) != "" {
		t.Fatal("empty prev must not emit a diff (caller sends full)")
	}
}

func TestResolveWorldStateCue_InvocationDiff(t *testing.T) {
	t.Parallel()
	inv := &trpcagent.Invocation{Session: &trpcsession.Session{ID: "s1"}}
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)

	full, kind := resolveWorldStateCue(ctx, "- Effective tool keys this turn: a")
	if kind != worldStateKindFull || full == "" {
		t.Fatalf("first call = (%q,%s), want full", full, kind)
	}
	skip, kind := resolveWorldStateCue(ctx, "- Effective tool keys this turn: a")
	if kind != worldStateKindUnchanged || skip != "" {
		t.Fatalf("unchanged = (%q,%s), want empty/unchanged", skip, kind)
	}
	diff, kind := resolveWorldStateCue(ctx, "- Effective tool keys this turn: a, b")
	if kind != worldStateKindDiff || !strings.Contains(diff, "+ - Effective tool keys this turn: a, b") {
		t.Fatalf("changed = (%q,%s)", diff, kind)
	}
}

func TestResolveWorldStateCue_NoInvocationSendsFull(t *testing.T) {
	t.Parallel()
	got, kind := resolveWorldStateCue(context.Background(), "- Effective tool keys this turn: a")
	if kind != worldStateKindFull || got == "" {
		t.Fatalf("got (%q,%s)", got, kind)
	}
}

func TestInsertBeforeLastUserMessage(t *testing.T) {
	t.Parallel()
	cue := asDynamicCue("WORLD")
	msgs := []trpcmodel.Message{
		trpcmodel.NewSystemMessage("sys"),
		trpcmodel.NewUserMessage("first"),
		trpcmodel.NewAssistantMessage("ok"),
		trpcmodel.NewUserMessage("second"),
	}
	out := insertBeforeLastUserMessage(msgs, cue)
	if len(out) != 5 || out[3].Content != "WORLD" || out[4].Content != "second" {
		t.Fatalf("insert before last user failed: %+v", out)
	}
	onlySys := []trpcmodel.Message{trpcmodel.NewSystemMessage("sys")}
	appended := insertBeforeLastUserMessage(onlySys, cue)
	if len(appended) != 2 || appended[1].Content != "WORLD" {
		t.Fatalf("no user message should append, got %+v", appended)
	}
}

func TestReinjectWorldStateAfterCompact(t *testing.T) {
	t.Parallel()
	inv := &trpcagent.Invocation{Session: &trpcsession.Session{ID: "s1"}}
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)
	inv.SetState(worldStateCueStateKey, "- Effective tool keys this turn: a")

	msgs := []trpcmodel.Message{
		trpcmodel.NewSystemMessage("sys"),
		trpcmodel.NewUserMessage("hello"),
	}
	out := reinjectWorldStateAfterCompact(ctx, msgs)
	if len(out) != 3 || !strings.Contains(out[1].Content, worldStateOpen) {
		t.Fatalf("expected snapshot before last user, got %+v", out)
	}
	// Second pass must not stack another copy.
	again := reinjectWorldStateAfterCompact(ctx, out)
	if len(again) != 3 {
		t.Fatalf("must be idempotent, got %d", len(again))
	}
}
