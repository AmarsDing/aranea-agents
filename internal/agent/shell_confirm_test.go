package agent

import (
	"context"
	"testing"
)

func TestClassifyShellCommand(t *testing.T) {
	t.Parallel()
	cases := []struct {
		cmd  string
		want shellConfirmClass
	}{
		{"go test ./...", shellClassSafe},
		{"go vet ./internal/agent", shellClassSafe},
		{`go test -count=1 "./pkg/..."`, shellClassSafe},
		{"go test -exec rm", shellClassDanger},
		{"git status", shellClassSafe},
		{"git --no-pager diff --stat", shellClassSafe},
		{"git log -1 --oneline", shellClassSafe},
		{"git push origin main", shellClassDanger},
		{"git reset --hard", shellClassDanger},
		{"rg -n classifyShell", shellClassSafe},
		{"ls -la", shellClassSafe},
		{"dir", shellClassSafe},
		{"golangci-lint run ./...", shellClassSafe},
		{"prettier --check web/src", shellClassSafe},
		{"prettier --write web/src", shellClassUnknown},
		{"rm -rf /", shellClassDanger},
		{"sudo apt update", shellClassDanger},
		{"echo hi && rm -rf tmp", shellClassDanger},
		{"go test ./... && git status", shellClassUnknown},
		{"npm test", shellClassUnknown},
		{"go build ./...", shellClassUnknown},
		{"go run main.go", shellClassUnknown},
		{"", shellClassUnknown},
	}
	for _, tc := range cases {
		if got := classifyShellCommand(tc.cmd); got != tc.want {
			t.Errorf("classifyShellCommand(%q) = %v, want %v", tc.cmd, got, tc.want)
		}
	}
}

func TestIsShellExecRuntimeName(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"shell_exec", "exec_command", "hostexec_exec_command", "shell"} {
		if !isShellExecRuntimeName(name) {
			t.Errorf("%q must be a shell runtime name", name)
		}
	}
	if isShellExecRuntimeName("save_file") {
		t.Fatal("save_file must not be treated as shell")
	}
}

func TestToolConfirmGate_ShellSafeSkipsCard(t *testing.T) {
	t.Parallel()
	g := newTestGate(map[string]confirmCatalogEntry{
		"shell_exec": {requiresConfirm: true},
	}, nil)
	d := g.decide(context.Background(), "sess-1", "agent-1", "shell_exec", []byte(`{"command":"go test ./..."}`))
	if d.needsConfirm || d.reason != confirmReasonShellSafe {
		t.Fatalf("safe shell decide = (%v,%q), want (false,%q)", d.needsConfirm, d.reason, confirmReasonShellSafe)
	}
	d = g.decide(context.Background(), "sess-1", "agent-1", "exec_command", []byte(`{"command":"git status"}`))
	if d.needsConfirm || d.reason != confirmReasonShellSafe {
		t.Fatalf("alias safe shell decide = (%v,%q), want (false,%q)", d.needsConfirm, d.reason, confirmReasonShellSafe)
	}
}

func TestToolConfirmGate_ShellDangerStillConfirms(t *testing.T) {
	t.Parallel()
	g := newTestGate(map[string]confirmCatalogEntry{
		"shell_exec": {requiresConfirm: true},
	}, nil)
	d := g.decide(context.Background(), "sess-1", "agent-1", "shell_exec", []byte(`{"command":"rm -rf tmp"}`))
	if !d.needsConfirm || d.reason != confirmReasonShellDanger {
		t.Fatalf("danger shell decide = (%v,%q), want (true,%q)", d.needsConfirm, d.reason, confirmReasonShellDanger)
	}
}

func TestToolConfirmGate_ShellUnknownConfirms(t *testing.T) {
	t.Parallel()
	g := newTestGate(map[string]confirmCatalogEntry{
		"shell_exec": {requiresConfirm: true},
	}, nil)
	d := g.decide(context.Background(), "sess-1", "agent-1", "shell_exec", []byte(`{"command":"npm test"}`))
	if !d.needsConfirm || d.reason != confirmReasonPolicyCatalog {
		t.Fatalf("unknown shell decide = (%v,%q), want (true,%q)", d.needsConfirm, d.reason, confirmReasonPolicyCatalog)
	}
}

func TestToolConfirmGate_ShellDangerGrantStillBypasses(t *testing.T) {
	t.Parallel()
	g := newTestGate(map[string]confirmCatalogEntry{
		"shell_exec": {requiresConfirm: true},
	}, nil)
	g.sessionGrants.GrantSession("sess-1", "agent-1", "shell_exec")
	d := g.decide(context.Background(), "sess-1", "agent-1", "shell_exec", []byte(`{"command":"git push"}`))
	if d.needsConfirm || d.reason != confirmReasonGrantSession {
		t.Fatalf("granted danger decide = (%v,%q), want (false,%q)", d.needsConfirm, d.reason, confirmReasonGrantSession)
	}
}
