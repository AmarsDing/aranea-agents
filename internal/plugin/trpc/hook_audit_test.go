package plugintrpc

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

type pluginRunsInsertStub struct {
	mu   sync.Mutex
	runs []biz.PluginRun
}

func (s *pluginRunsInsertStub) Insert(_ context.Context, run biz.PluginRun) error {
	s.mu.Lock()
	s.runs = append(s.runs, run)
	s.mu.Unlock()
	return nil
}

func (s *pluginRunsInsertStub) List(context.Context, biz.PluginRunQuery) (biz.PluginRunListResult, error) {
	return biz.PluginRunListResult{}, nil
}

func TestRecordHookAudit_persistsBlockedRun(t *testing.T) {
	repo := &statsRepoStub{key: "unused"}
	runs := &pluginRunsInsertStub{}
	rec := NewRepoStatsRecorder(repo, runs)

	rh := biz.ResolvedHook{
		Hook: biz.Hook{Key: "deny-tools", Name: "Deny Tools"},
		Rule: biz.HookConfig{Action: biz.HookAction{Type: "block"}},
	}
	recordHookAudit(rec, context.Background(), rh, "before_tool", "blocked", "agent-1", 12)

	deadline := time.Now().Add(2 * time.Second)
	for {
		runs.mu.Lock()
		n := len(runs.runs)
		runs.mu.Unlock()
		if n > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	runs.mu.Lock()
	defer runs.mu.Unlock()
	if len(runs.runs) != 1 {
		t.Fatalf("runs=%d", len(runs.runs))
	}
	if runs.runs[0].PluginKey != "hook:deny-tools" || runs.runs[0].CallbackPoint != "before_tool" || runs.runs[0].Status != "blocked" {
		t.Fatalf("run=%+v", runs.runs[0])
	}
}
