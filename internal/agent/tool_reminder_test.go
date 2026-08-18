package agent

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/agent/callbacks"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestToolReminder_CollectsReminders(t *testing.T) {
	r := NewToolReminder()
	// File modified but no test executed afterwards.
	r.OnToolExecuted("edit_file", map[string]string{"path": "/foo.go"})
	reminders := r.Collect()
	if len(reminders) == 0 {
		t.Fatal("expected reminders for unverified file edit")
	}
	if !strings.Contains(reminders[0], "/foo.go") {
		t.Fatalf("expected reminder to mention edited path, got %q", reminders[0])
	}
}

func TestToolReminder_ClearedByTestRun(t *testing.T) {
	r := NewToolReminder()
	r.OnToolExecuted("write_file", map[string]string{"path": "/bar.go"})
	r.OnToolExecuted("exec_command", map[string]string{"command": "go test ./internal/foo/..."})
	if got := r.Collect(); len(got) != 0 {
		t.Fatalf("expected no reminders after test run, got %v", got)
	}
}

func TestToolReminder_MultipleEdits(t *testing.T) {
	r := NewToolReminder()
	r.OnToolExecuted("write_file", map[string]string{"path": "/a.go"})
	r.OnToolExecuted("edit_file", map[string]string{"path": "/b.go"})
	reminders := r.Collect()
	if len(reminders) == 0 {
		t.Fatal("expected reminder for unverified edits")
	}
	if !strings.Contains(reminders[0], "/a.go") || !strings.Contains(reminders[0], "/b.go") {
		t.Fatalf("expected reminder to mention both paths, got %q", reminders[0])
	}
}

func TestToolReminder_SaveFileArmsReminder(t *testing.T) {
	r := NewToolReminder()
	r.OnToolExecuted("save_file", map[string]string{"file_name": "internal/foo.go"})
	reminders := r.Collect()
	if len(reminders) == 0 {
		t.Fatal("expected reminder after save_file")
	}
	if !strings.Contains(reminders[0], "internal/foo.go") {
		t.Fatalf("expected file_name in reminder, got %q", reminders[0])
	}
	if !strings.Contains(reminders[0], "read_lints") {
		t.Fatalf("expected read_lints hint, got %q", reminders[0])
	}
	r.OnToolExecuted("file_save_file", map[string]string{"file_name": "bar.go"})
	if got := r.Collect(); len(got) == 0 || !strings.Contains(got[0], "bar.go") {
		t.Fatalf("mounted name file_save_file must arm reminder, got %v", got)
	}
}

func TestToolReminder_ReadLintsClears(t *testing.T) {
	r := NewToolReminder()
	r.OnToolExecuted("save_file", map[string]string{"file_name": "a.go"})
	r.OnToolExecuted("read_lints", map[string]string{"path": "a.go"})
	if got := r.Collect(); len(got) != 0 {
		t.Fatalf("read_lints should clear reminder, got %v", got)
	}
}

func TestToolReminder_NonFileToolsIgnored(t *testing.T) {
	r := NewToolReminder()
	r.OnToolExecuted("read_file", map[string]string{"path": "/foo.go"})
	r.OnToolExecuted("exec_command", map[string]string{"command": "ls -la"})
	if got := r.Collect(); len(got) != 0 {
		t.Fatalf("expected no reminders for read-only tools, got %v", got)
	}
}

func TestToolReminder_NewEditAfterTestRun(t *testing.T) {
	r := NewToolReminder()
	r.OnToolExecuted("write_file", map[string]string{"path": "/a.go"})
	r.OnToolExecuted("exec_command", map[string]string{"command": "go test ./..."})
	// A subsequent edit re-arms the reminder.
	r.OnToolExecuted("edit_file", map[string]string{"path": "/c.go"})
	if got := r.Collect(); len(got) == 0 {
		t.Fatal("expected reminder for edit after last test run")
	}
}

// TestToolReminder_InvocationIsolation guards against cross-session state
// pollution: two invocations sharing the same cached Agent (and thus the
// same hook instances) must each get an independent ToolReminder.
func TestToolReminder_InvocationIsolation(t *testing.T) {
	before, ok := newToolReminderBeforeAgentHook().(callbacks.BeforeAgentHook)
	if !ok {
		t.Fatal("before hook does not implement BeforeAgentHook")
	}
	after, ok := newToolReminderAfterHook().(callbacks.AfterToolHook)
	if !ok {
		t.Fatal("after hook does not implement AfterToolHook")
	}

	// Session 1: BeforeAgent pre-creates the reminder instance.
	inv1 := &trpcagent.Invocation{}
	ctx1 := trpcagent.NewInvocationContext(context.Background(), inv1)
	if _, err := before.HandleBeforeAgent(ctx1, &trpcagent.BeforeAgentArgs{Invocation: inv1}); err != nil {
		t.Fatalf("before hook session1: %v", err)
	}

	// Session 2: same hooks, independent invocation.
	inv2 := &trpcagent.Invocation{}
	ctx2 := trpcagent.NewInvocationContext(context.Background(), inv2)
	if _, err := before.HandleBeforeAgent(ctx2, &trpcagent.BeforeAgentArgs{Invocation: inv2}); err != nil {
		t.Fatalf("before hook session2: %v", err)
	}

	// Session 1 edits a file; its own result carries the reminder.
	res1, err := after.HandleAfterTool(ctx1, &trpctool.AfterToolArgs{
		ToolName:  "edit_file",
		Arguments: []byte(`{"path": "/session1.go"}`),
		Result:    "ok",
	})
	if err != nil {
		t.Fatalf("after hook session1: %v", err)
	}
	got1, _ := res1.CustomResult.(string)
	if !strings.Contains(got1, "[reminder]") || !strings.Contains(got1, "/session1.go") {
		t.Fatalf("expected session1 result to carry reminder, got %q", got1)
	}

	// Session 2 runs a read-only tool; it must not see session 1's edits.
	res2, err := after.HandleAfterTool(ctx2, &trpctool.AfterToolArgs{
		ToolName:  "read_file",
		Arguments: []byte(`{"path": "/other.go"}`),
		Result:    "data",
	})
	if err != nil {
		t.Fatalf("after hook session2: %v", err)
	}
	if got2, _ := res2.CustomResult.(string); strings.Contains(got2, "[reminder]") {
		t.Fatalf("session2 must not see session1 reminders, got %q", got2)
	}
	v2, found := inv2.GetState(toolReminderStateKey)
	if !found {
		t.Fatal("session2 reminder state missing")
	}
	if collected := v2.(*ToolReminder).Collect(); len(collected) != 0 {
		t.Fatalf("session2 reminder should be empty, got %v", collected)
	}
}

// TestToolReminder_AfterHookLazyInit covers the defensive path where the
// BeforeAgent hook did not run: the AfterTool hook lazily creates the
// per-invocation instance instead of failing silently.
func TestToolReminder_AfterHookLazyInit(t *testing.T) {
	after, ok := newToolReminderAfterHook().(callbacks.AfterToolHook)
	if !ok {
		t.Fatal("after hook does not implement AfterToolHook")
	}
	inv := &trpcagent.Invocation{}
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)
	res, err := after.HandleAfterTool(ctx, &trpctool.AfterToolArgs{
		ToolName:  "edit_file",
		Arguments: []byte(`{"path": "/lazy.go"}`),
		Result:    "ok",
	})
	if err != nil {
		t.Fatalf("after hook: %v", err)
	}
	got, _ := res.CustomResult.(string)
	if !strings.Contains(got, "[reminder]") {
		t.Fatalf("expected reminder via lazy init, got %q", got)
	}
}

// TestToolReminder_AfterHookNoInvocation ensures the hook is a safe no-op
// when the context carries no invocation.
func TestToolReminder_AfterHookNoInvocation(t *testing.T) {
	after, ok := newToolReminderAfterHook().(callbacks.AfterToolHook)
	if !ok {
		t.Fatal("after hook does not implement AfterToolHook")
	}
	res, err := after.HandleAfterTool(context.Background(), &trpctool.AfterToolArgs{
		ToolName:  "edit_file",
		Arguments: []byte(`{"path": "/x.go"}`),
		Result:    "ok",
	})
	if err != nil {
		t.Fatalf("after hook: %v", err)
	}
	if res == nil || res.CustomResult != nil {
		t.Fatalf("expected empty result without invocation, got %+v", res)
	}
}
