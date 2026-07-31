package biz

import (
	"context"
	"testing"
)

// ── activity sink fake ───────────────────────────────────────────────────────

type siActivityFakeSink struct{ records []SIActivityRecord }

func (s *siActivityFakeSink) EmitSIActivity(_ context.Context, a SIActivityRecord) error {
	s.records = append(s.records, a)
	return nil
}

// stageSeq condenses the emission log into "stage:status:attempt" tuples.
func stageSeq(records []SIActivityRecord) []string {
	out := make([]string, 0, len(records))
	for _, r := range records {
		out = append(out, r.Stage+":"+string(r.Status)+":"+itoa(r.Attempt))
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func assertSeq(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("emission count = %d, want %d\n got: %v\nwant: %v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("emission[%d] = %s, want %s\n got: %v\nwant: %v", i, got[i], want[i], got, want)
		}
	}
}

// ── deterministic IDs ────────────────────────────────────────────────────────

func TestSIActivityIDs_Deterministic(t *testing.T) {
	if got := SIRunActivityID("run-1"); got != "si-run:run-1" {
		t.Errorf("SIRunActivityID = %q", got)
	}
	if got := SIStageActivityID("run-1", SIStageDiagnosing, 0); got != "si-run:run-1:diagnosing" {
		t.Errorf("diagnosing id = %q", got)
	}
	if got := SIStageActivityID("run-1", SIStagePatching, 2); got != "si-run:run-1:patching:a2" {
		t.Errorf("patching a2 id = %q", got)
	}
}

// ── event tree: happy path ───────────────────────────────────────────────────

func TestSIPipeline_ActivityTree(t *testing.T) {
	sink := &siActivityFakeSink{}
	uc, _, _, _ := siPipelineFixture(3, func(d *SelfImprovementPipelineDeps) {
		d.ActivitySink = sink
	})
	if err := uc.Execute(context.Background(), "run-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	assertSeq(t, stageSeq(sink.records), []string{
		"run:running:0",
		"diagnosing:running:0",
		"diagnosing:completed:0",
		"patching:running:1",
		"patching:completed:1",
		"verifying:running:1",
		"verifying:completed:1",
		"governing:running:0",
		"governing:completed:0",
		"run:completed:0",
	})
	// 树断言：根无父，其余全部挂根（resolveParentActivityID 规范）。
	rootID := SIRunActivityID("run-1")
	for i, r := range sink.records {
		wantParent := rootID
		if r.Stage == SIStageRun {
			wantParent = ""
			if r.ID != rootID {
				t.Errorf("record[%d] root id = %q, want %q", i, r.ID, rootID)
			}
		} else if r.ID == rootID || r.ID == "" {
			t.Errorf("record[%d] stage id invalid: %q", i, r.ID)
		}
		if r.ParentActivityID != wantParent {
			t.Errorf("record[%d] %s parent = %q, want %q", i, r.Stage, r.ParentActivityID, wantParent)
		}
		if r.RunID != "run-1" {
			t.Errorf("record[%d] RunID = %q", i, r.RunID)
		}
	}
}

// ── event tree: verify 失败重试产生 attempt 作用域子树 ───────────────────────

func TestSIPipeline_ActivityTreeRetry(t *testing.T) {
	sink := &siActivityFakeSink{}
	uc, store, sandbox, _ := siPipelineFixture(3, func(d *SelfImprovementPipelineDeps) {
		d.ActivitySink = sink
	})
	sandbox.gateFn = func(gate SandboxGateKind, callIdx int) SandboxGateResult {
		if callIdx == 0 { // 第一次 G1 失败，第二次全过
			return SandboxGateResult{Gate: gate, Passed: false, Output: "boom"}
		}
		return SandboxGateResult{Gate: gate, Passed: true}
	}
	if err := uc.Execute(context.Background(), "run-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if store.run.Status != RunStatusAwaitingGovernance {
		t.Fatalf("status = %s, want awaiting_governance", store.run.Status)
	}
	assertSeq(t, stageSeq(sink.records), []string{
		"run:running:0",
		"diagnosing:running:0",
		"diagnosing:completed:0",
		"patching:running:1",
		"patching:completed:1",
		"verifying:running:1",
		"verifying:failed:1",
		"patching:running:2",
		"patching:completed:2",
		"verifying:running:2",
		"verifying:completed:2",
		"governing:running:0",
		"governing:completed:0",
		"run:completed:0",
	})
	// 两次 patching 活动 ID 必须按 attempt 区分且同挂一个根。
	rootID := SIRunActivityID("run-1")
	var patchingIDs []string
	for _, r := range sink.records {
		if r.Stage == SIStagePatching && r.Status == ActivityStatusRunning {
			patchingIDs = append(patchingIDs, r.ID)
			if r.ParentActivityID != rootID {
				t.Errorf("patching parent = %q, want %q", r.ParentActivityID, rootID)
			}
		}
	}
	if len(patchingIDs) != 2 || patchingIDs[0] == patchingIDs[1] {
		t.Errorf("patching attempt ids = %v, want 2 distinct", patchingIDs)
	}
}

// ── event tree: 失败路径不留 dangling running ────────────────────────────────

func TestSIPipeline_ActivityTreeFailFast(t *testing.T) {
	sink := &siActivityFakeSink{}
	uc, store, _, _ := siPipelineFixture(3, func(d *SelfImprovementPipelineDeps) {
		d.ActivitySink = sink
		d.Patcher = siPatcherFn(func(context.Context, SIPatchRequest) (*PatcherOutput, error) {
			return &PatcherOutput{Diff: siRiskDiff("Makefile", 3), Kind: PatchKindConfig}, nil
		})
	})
	if err := uc.Execute(context.Background(), "run-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if store.run.Status != RunStatusRejected {
		t.Fatalf("status = %s, want rejected", store.run.Status)
	}
	assertSeq(t, stageSeq(sink.records), []string{
		"run:running:0",
		"diagnosing:running:0",
		"diagnosing:completed:0",
		"patching:running:1",
		"patching:failed:1",
		"run:failed:0",
	})
}

// ── 无 sink 时流水线行为不变（nil-safe） ────────────────────────────────────

func TestSIPipeline_ActivitySinkNilSafe(t *testing.T) {
	uc, store, _, _ := siPipelineFixture(3, nil)
	if err := uc.Execute(context.Background(), "run-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if store.run.Status != RunStatusAwaitingGovernance {
		t.Fatalf("status = %s", store.run.Status)
	}
}
