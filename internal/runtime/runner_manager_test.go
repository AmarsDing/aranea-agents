package runtime

import (
	"testing"

	trpcllmagent "trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
)

func TestTurnDeps_CoalesceRunnerManager(t *testing.T) {
	var d TurnDeps
	m1 := d.CoalesceRunnerManager()
	m2 := d.CoalesceRunnerManager()
	if m1 == nil || m1 != m2 {
		t.Fatal("expected single coalesced RunnerManager instance")
	}
}

func TestRunnerInstanceRegistryReplace(t *testing.T) {
	mgr := NewRunnerManager(RunnerFactoryDeps{})
	root := trpcllmagent.New("a")
	_, err := mgr.NewTurnRunner(root, TurnRunnerSpec{RegistryKey: "sess-1"})
	if err != nil {
		t.Fatalf("first runner: %v", err)
	}
	root2 := trpcllmagent.New("b")
	mr2, err := mgr.NewTurnRunner(root2, TurnRunnerSpec{RegistryKey: "sess-1"})
	if err != nil {
		t.Fatalf("second runner: %v", err)
	}
	got, ok := mgr.Registry().Get("sess-1")
	if !ok || got != mr2 {
		t.Fatalf("registry should hold the latest runner instance")
	}
	if err := mgr.CloseRunner("sess-1"); err != nil {
		t.Fatalf("CloseRunner() error = %v", err)
	}
}

func TestRunnerManagerNewTurnRunnerNilRoot(t *testing.T) {
	mgr := NewRunnerManager(RunnerFactoryDeps{})
	_, err := mgr.NewTurnRunner(nil, TurnRunnerSpec{})
	if err == nil {
		t.Fatal("expected error for nil root agent")
	}
}

func TestRunnerManagerCloseRunnerUnknownKey(t *testing.T) {
	mgr := NewRunnerManager(RunnerFactoryDeps{})
	if err := mgr.CloseRunner("missing"); err != nil {
		t.Fatalf("CloseRunner() error = %v", err)
	}
}

func TestRunnerManagerNewTurnRunnerMinimal(t *testing.T) {
	root := trpcllmagent.New("test-agent")
	mgr := NewRunnerManager(RunnerFactoryDeps{})
	mr, err := mgr.NewTurnRunner(root, TurnRunnerSpec{})
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	if mr == nil {
		t.Fatal("expected managed runner")
	}
	_ = mr.Close()
}
