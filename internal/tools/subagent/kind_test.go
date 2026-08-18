package subagent

import (
	"context"
	"strings"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"

	trpcsubagent "trpc.group/trpc-go/trpc-agent-go/openclaw/subagent"
)

func TestNormalizeKind(t *testing.T) {
	if got := normalizeKind("search"); got != kindExplore {
		t.Fatalf("got %q", got)
	}
	if got := normalizeKind("qa"); got != kindVerify {
		t.Fatalf("got %q", got)
	}
	if got := normalizeKind(""); got != kindGeneral {
		t.Fatalf("got %q", got)
	}
}

func TestKindSystemPrompt(t *testing.T) {
	if p := kindSystemPrompt(kindExplore); !strings.Contains(p, "Kind=explore") || !strings.Contains(p, "search_content") {
		t.Fatalf("%s", p)
	}
	if p := kindSystemPrompt(kindVerify); !strings.Contains(p, "Kind=verify") || !strings.Contains(p, "pass/fail") {
		t.Fatalf("%s", p)
	}
}

func TestWaitForUser_AlreadyTerminal(t *testing.T) {
	svc, err := NewService(t.TempDir(), &stubRunner{}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	svc.Start(context.Background())
	svc.mu.Lock()
	svc.runs["r1"] = &runRecord{
		Run: trpcsubagent.Run{
			ID:     "r1",
			Status: trpcsubagent.StatusCompleted,
			Kind:   kindExplore,
			Task:   "find X",
		},
		OwnerUserID: "user",
	}
	svc.mu.Unlock()
	run, err := svc.WaitForUser(context.Background(), "user", "r1", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if run == nil || run.Kind != kindExplore {
		t.Fatalf("%+v", run)
	}
}

func TestEnrichRunView_RunningFor(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	m := enrichRunView(trpcsubagent.Run{
		ID:        "x",
		Status:    trpcsubagent.StatusRunning,
		StartedAt: &start,
		Kind:      kindVerify,
	}, start.Add(1500*time.Millisecond))
	if m["kind"] != kindVerify {
		t.Fatalf("%v", m)
	}
	if m["running_for_ms"] != int64(1500) {
		t.Fatalf("running_for_ms=%v", m["running_for_ms"])
	}
}

func TestSpawnTool_DeclarationHasKind(t *testing.T) {
	d := newSpawnTool(&Service{maxConcurrent: 2}).Declaration()
	if d.InputSchema == nil || d.InputSchema.Properties[argKind] == nil {
		t.Fatalf("%+v", d.InputSchema)
	}
	gd := newGetTool(&Service{}).Declaration()
	if gd.InputSchema == nil || gd.InputSchema.Properties[argBlockUntilMS] == nil {
		t.Fatalf("%+v", gd.InputSchema)
	}
}
