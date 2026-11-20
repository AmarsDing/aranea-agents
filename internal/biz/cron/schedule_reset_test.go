package cron

import (
	"context"
	"strings"
	"testing"
)

// 改调度或重新启用后，旧的 next_run_at 必须清掉；只改名字不动它。
func TestUsecase_UpdateTask_ResetsNextRunAt(t *testing.T) {
	const meta = `{"run_count":3,"next_run_at":"2026-11-20T00:00:00Z"}`
	base := Task{
		ID: "x", TaskKey: "k", Name: "n", Status: "active", Enabled: true,
		ConfigJSON:   `{"schedule_type":"interval","interval_seconds":900,"message":"m"}`,
		MetadataJSON: meta,
	}
	cases := []struct {
		name      string
		cur       Task
		patch     TaskPatch
		wantClear bool
	}{
		{"rename keeps next_run_at", base, TaskPatch{Name: StrPtr("renamed")}, false},
		{"interval change clears", base, TaskPatch{ConfigJSON: StrPtr(`{"schedule_type":"interval","interval_seconds":3600,"message":"m"}`)}, true},
		{"switch to cron clears", base, TaskPatch{ConfigJSON: StrPtr(`{"schedule_type":"cron","cron_expression":"0 9 * * *","message":"m"}`)}, true},
		{"message-only change keeps", base, TaskPatch{ConfigJSON: StrPtr(`{"schedule_type":"interval","interval_seconds":900,"message":"other"}`)}, false},
		{"re-enable clears", func() Task { c := base; c.Enabled = false; c.Status = "paused"; return c }(), TaskPatch{Enabled: BoolPtr(true), Status: StrPtr("active")}, true},
		{"pause keeps", base, TaskPatch{Enabled: BoolPtr(false), Status: StrPtr("paused")}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var saved Task
			repo := noOpRepo()
			repo.getFn = func(context.Context, string) (Task, error) { return tc.cur, nil }
			repo.updateFn = func(_ context.Context, tk Task) (Task, error) { saved = tk; return tk, nil }
			if _, err := NewUsecase(repo, noOpTrigger()).UpdateTask(context.Background(), "x", tc.patch); err != nil {
				t.Fatalf("UpdateTask: %v", err)
			}
			cleared := !strings.Contains(saved.MetadataJSON, "next_run_at")
			if cleared != tc.wantClear {
				t.Fatalf("next_run_at cleared = %v, want %v (meta %s)", cleared, tc.wantClear, saved.MetadataJSON)
			}
			if !strings.Contains(saved.MetadataJSON, `"run_count":3`) {
				t.Fatalf("run_count lost: %s", saved.MetadataJSON)
			}
		})
	}
}
