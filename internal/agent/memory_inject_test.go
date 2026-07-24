package agent

import (
	"testing"

	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

func TestMemoryCueResult_IsEmpty(t *testing.T) {
	if (&MemoryCueResult{}).IsEmpty() != true {
		t.Error("empty result should be empty")
	}
	if (&MemoryCueResult{L1Cue: "test"}).IsEmpty() != false {
		t.Error("result with L1Cue should not be empty")
	}
	if (&MemoryCueResult{RecallCue: "test"}).IsEmpty() != false {
		t.Error("result with RecallCue should not be empty")
	}
}

func TestMemoryCueResult_JoinCues(t *testing.T) {
	r := &MemoryCueResult{L1Cue: "L1", RecallCue: "Recall"}
	if r.JoinCues() != "L1\n\nRecall" {
		t.Errorf("unexpected JoinCues result: %q", r.JoinCues())
	}
	if (&MemoryCueResult{L1Cue: "L1"}).JoinCues() != "L1" {
		t.Errorf("unexpected JoinCues result with only L1: %q", (&MemoryCueResult{L1Cue: "L1"}).JoinCues())
	}
}

func TestIsMemoryInjectMessage(t *testing.T) {
	msg := trpcmodel.NewSystemMessage(memoryInjectCueContent("test cue"))
	if !isMemoryInjectMessage(msg) {
		t.Error("message with marker should be identified")
	}
	plainMsg := trpcmodel.NewSystemMessage("plain content")
	if isMemoryInjectMessage(plainMsg) {
		t.Error("plain message should not be identified as memory inject")
	}
}

// TestMemoryRuntimeContext_TeamIDResolution verifies the C5 team_id chain:
// session state takes priority; when absent, fall back to the invocation's
// RunOptions.RuntimeState (injected by the team graph runtime).
func TestMemoryRuntimeContext_TeamIDResolution(t *testing.T) {
	ag := biz.Agent{ID: "ag-1"}

	// Case 1: session state wins over RuntimeState.
	inv := &trpcagent.Invocation{
		Session: &trpcsession.Session{
			UserID: "u1",
			State:  map[string][]byte{"team_id": []byte("team-from-session")},
		},
		RunOptions: trpcagent.RunOptions{
			RuntimeState: map[string]any{"team_id": "team-from-runtime"},
		},
	}
	if rt := memoryRuntimeContext(inv, ag); rt.TeamID != "team-from-session" {
		t.Fatalf("session state should win, TeamID=%q", rt.TeamID)
	}

	// Case 2: fallback to RuntimeState when session state lacks team_id.
	inv2 := &trpcagent.Invocation{
		Session: &trpcsession.Session{UserID: "u1"},
		RunOptions: trpcagent.RunOptions{
			RuntimeState: map[string]any{"team_id": "team-from-runtime"},
		},
	}
	if rt := memoryRuntimeContext(inv2, ag); rt.TeamID != "team-from-runtime" {
		t.Fatalf("RuntimeState fallback failed, TeamID=%q", rt.TeamID)
	}

	// Case 3: neither source → empty TeamID.
	inv3 := &trpcagent.Invocation{Session: &trpcsession.Session{UserID: "u1"}}
	if rt := memoryRuntimeContext(inv3, ag); rt.TeamID != "" {
		t.Fatalf("expected empty TeamID, got %q", rt.TeamID)
	}

	// Case 4: non-string RuntimeState value is ignored.
	inv4 := &trpcagent.Invocation{
		Session: &trpcsession.Session{UserID: "u1"},
		RunOptions: trpcagent.RunOptions{
			RuntimeState: map[string]any{"team_id": 42},
		},
	}
	if rt := memoryRuntimeContext(inv4, ag); rt.TeamID != "" {
		t.Fatalf("non-string team_id should be ignored, got %q", rt.TeamID)
	}
}
