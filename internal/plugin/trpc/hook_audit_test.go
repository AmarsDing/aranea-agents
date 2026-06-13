package plugintrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
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
	rec := NewRepoStatsRecorder(repo, runs, false, loggateway.NewNoop())
	defer rec.Close()

	rh := biz.ResolvedHook{
		Hook: biz.Hook{Key: "deny-tools", Name: "Deny Tools"},
		Rule: biz.HookConfig{Action: biz.HookAction{Type: "block"}},
	}
	recordHookAudit(rec, context.Background(), rh, "before_tool", "blocked", "agent-1", 12, nil)

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

func TestRecordHookAudit_summaryFallback(t *testing.T) {
	repo := &statsRepoStub{key: "unused"}
	runs := &pluginRunsInsertStub{}
	rec := NewRepoStatsRecorder(repo, runs, true, loggateway.NewNoop())
	defer rec.Close()

	// Case 1: Name is empty → fallback to "hook:<key>"
	rhNoName := biz.ResolvedHook{
		Hook: biz.Hook{Key: "my-hook", Name: ""},
		Rule: biz.HookConfig{Action: biz.HookAction{Type: "log"}},
	}
	recordHookAudit(rec, context.Background(), rhNoName, "after_tool", "ok", "agent-1", 5, nil)

	// Case 2: Name is set with error → summary includes error message
	rhWithErr := biz.ResolvedHook{
		Hook: biz.Hook{Key: "err-hook", Name: "Error Hook"},
		Rule: biz.HookConfig{Action: biz.HookAction{Type: "notify"}},
	}
	recordHookAudit(rec, context.Background(), rhWithErr, "after_tool", "error", "agent-2", 10, fmt.Errorf("webhook failed"))

	deadline := time.Now().Add(2 * time.Second)
	for {
		runs.mu.Lock()
		n := len(runs.runs)
		runs.mu.Unlock()
		if n >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	runs.mu.Lock()
	defer runs.mu.Unlock()
	if len(runs.runs) < 2 {
		t.Fatalf("runs=%d", len(runs.runs))
	}

	// Find runs by plugin key
	runMap := map[string]biz.PluginRun{}
	for _, r := range runs.runs {
		runMap[r.PluginKey] = r
	}

	// Case 1: summary should fallback to "hook:my-hook"
	if r, ok := runMap["hook:my-hook"]; ok {
		var detail map[string]string
		if err := json.Unmarshal([]byte(r.DetailJSON), &detail); err != nil {
			t.Fatalf("unmarshal detail: %v", err)
		}
		if detail["summary"] != "hook:my-hook" {
			t.Errorf("expected summary='hook:my-hook', got %q", detail["summary"])
		}
	} else {
		t.Error("hook:my-hook not found in runs")
	}

	// Case 2: summary should include error message
	if r, ok := runMap["hook:err-hook"]; ok {
		var detail map[string]string
		if err := json.Unmarshal([]byte(r.DetailJSON), &detail); err != nil {
			t.Fatalf("unmarshal detail: %v", err)
		}
		if detail["summary"] != "Error Hook — webhook failed" {
			t.Errorf("expected summary='Error Hook — webhook failed', got %q", detail["summary"])
		}
	} else {
		t.Error("hook:err-hook not found in runs")
	}
}
