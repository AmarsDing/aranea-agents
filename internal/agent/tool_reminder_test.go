package agent

import (
	"strings"
	"testing"
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
