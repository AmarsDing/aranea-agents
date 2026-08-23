package biz

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestClipUpwardPayloadAndHeartbeatNeverBlocks(t *testing.T) {
	t.Parallel()
	if ClipUpwardPayload("") != "" {
		t.Fatal("empty")
	}
	long := strings.Repeat("上", UpwardPipeMaxRunes+50)
	if n := utf8.RuneCountInString(ClipUpwardPayload(long)); n != UpwardPipeMaxRunes {
		t.Fatalf("clipped=%d", n)
	}
	if UpwardIsDispatchBarrier(PipeUpwardHeartbeat) || UpwardIsDispatchBarrier(PipeUpwardException) {
		t.Fatal("upward must not be a dispatch barrier")
	}
}

func TestNeedsUserConfirmFiveTiersOnly(t *testing.T) {
	t.Parallel()
	if NeedsUserConfirm(ConfirmInput{}) != ConfirmNone {
		t.Fatal("default stage handoff must not pop a card")
	}
	if NeedsUserConfirm(ConfirmInput{CreatingAgent: true}) != ConfirmCreateAgent {
		t.Fatal("create agent")
	}
	if NeedsUserConfirm(ConfirmInput{AuthorizingPlaybook: true}) != ConfirmNewPlaybook {
		t.Fatal("new playbook")
	}
	if IsLateralBriefChannel(PipeDeptMail) || IsLateralBriefChannel(PipeUserInject) {
		t.Fatal("deptmail and inject are not the lateral Brief pipe")
	}
	if !IsLateralBriefChannel(PipeLateralBrief) {
		t.Fatal("brief is the lateral pipe")
	}
}

func TestNewUpwardProgressEventIsNotABarrier(t *testing.T) {
	t.Parallel()
	ev := NewUpwardProgressEvent("sess", PipeUpwardHeartbeat, strings.Repeat("上", UpwardPipeMaxRunes+20), map[string]any{"step_id": "st1"})
	if ev.NoticeType != "orchestration_progress" {
		t.Fatal(ev.NoticeType)
	}
	if ev.Meta["dispatch_barrier"] != false {
		t.Fatal("heartbeat must not be a dispatch barrier")
	}
	if n := utf8.RuneCountInString(ev.Meta["summary"].(string)); n != UpwardPipeMaxRunes {
		t.Fatalf("summary runes=%d", n)
	}
}

func TestClampSpecialistL3ScopesDropsTeamBus(t *testing.T) {
	t.Parallel()
	p := MemoryRuntimePolicy{L3RecallScopes: []string{"agent", "team", "user"}}
	ClampSpecialistL3Scopes(&p, Agent{AgentKey: "be"})
	if len(p.L3RecallScopes) != 1 || p.L3RecallScopes[0] != "agent" {
		t.Fatalf("specialist scopes=%v", p.L3RecallScopes)
	}
	spirit := MemoryRuntimePolicy{L3RecallScopes: []string{"agent", "team"}}
	ClampSpecialistL3Scopes(&spirit, Agent{AgentKey: SpiritAgentKey})
	if len(spirit.L3RecallScopes) != 2 {
		t.Fatalf("spirit scopes should stay %v", spirit.L3RecallScopes)
	}
}

func TestOrgMemberL3ScopesArePersonal(t *testing.T) {
	t.Parallel()
	for _, s := range OrgMemberL3Scopes() {
		if s == "team" || s == "user" {
			t.Fatalf("specialist L3 must not use %q as a sibling bus", s)
		}
	}
}

func TestParseOldCheckpointWithoutOrgFields(t *testing.T) {
	t.Parallel()
	p, err := ParseDurableCheckpointPayload(`{"session_id":"s1","turn_id":"t1","agent_id":"a1","runtime_run_id":"r1"}`)
	if err != nil {
		t.Fatal(err)
	}
	if p.SessionID != "s1" || p.PlaybookID != "" || p.Gear != "" {
		t.Fatalf("old payload must recover: %+v", p)
	}
}

func TestOrgCheckpointFromPlanCopiesPlaybookStages(t *testing.T) {
	t.Parallel()
	if got := OrgCheckpointFromPlan(nil); got.PlaybookID != "" {
		t.Fatalf("nil plan: %+v", got)
	}
	if got := OrgCheckpointFromPlan(&TaskPlan{
		Status:    TaskPlanStatusCompleted,
		SubTasks:  []SubTask{{ID: "design"}},
		MemoryHit: &MemoryHit{PlaybookID: "software_delivery"},
	}); got.PlaybookID != "" {
		t.Fatalf("completed plan must not snapshot: %+v", got)
	}
	got := OrgCheckpointFromPlan(&TaskPlan{
		Status:   TaskPlanStatusExecuting,
		SubTasks: []SubTask{{ID: "design"}, {ID: "be"}},
		MemoryHit: &MemoryHit{
			PlaybookID:            "software_delivery",
			ConstraintFingerprint: "fp-1",
		},
	})
	if got.Gear != "heavy" || got.PlaybookID != "software_delivery" || got.ConstraintFingerprint != "fp-1" {
		t.Fatalf("%+v", got)
	}
	if len(got.AuthorizedStageIDs) != 2 || got.AuthorizedStageIDs[1] != "be" {
		t.Fatalf("stages=%v", got.AuthorizedStageIDs)
	}
	if len(got.IssuedBriefIDs) != 2 || got.IssuedBriefIDs[1] != "be" {
		t.Fatalf("briefs=%v", got.IssuedBriefIDs)
	}
}

func TestHydratePlanStepsFromSubTasksCopiesOrgFields(t *testing.T) {
	t.Parallel()
	planID := "plan-1"
	raw := "be"
	steps := []PlanStep{{ID: DeterministicPlanStepID(planID, raw), Label: "后端"}}
	HydratePlanStepsFromSubTasks(planID, steps, []SubTask{{
		ID:              raw,
		GraphTemplateID: "tmpl-1",
		ConfirmBefore:   true,
		CollectionIDs:   []string{"kb-1"},
	}})
	if steps[0].GraphTemplateID != "tmpl-1" || !steps[0].ConfirmBefore || len(steps[0].CollectionIDs) != 1 {
		t.Fatalf("%+v", steps[0])
	}
}

func TestDurableResumePromptForIncludesPlaybook(t *testing.T) {
	t.Parallel()
	if DurableResumePromptFor(DurableRunCheckpointPayload{}) != DurableResumePrompt() {
		t.Fatal("empty payload must keep generic prompt")
	}
	got := DurableResumePromptFor(DurableRunCheckpointPayload{PlaybookID: "software_delivery", AuthorizedStageIDs: []string{"be"}, IssuedBriefIDs: []string{"be"}})
	if !strings.Contains(got, "software_delivery") || !strings.Contains(got, "be") || !strings.Contains(got, "issued_briefs=") {
		t.Fatalf("prompt=%q", got)
	}
}

func TestPlaybookConfirmActivityIDRoundTrip(t *testing.T) {
	t.Parallel()
	id := PlaybookConfirmActivityID("sp-1", "st-confirm")
	step, ok := ParsePlaybookConfirmActivityID("sp-1", id)
	if !ok || step != "st-confirm" {
		t.Fatalf("id=%q step=%q ok=%v", id, step, ok)
	}
	if _, ok := ParsePlaybookConfirmActivityID("other", id); ok {
		t.Fatal("foreign session must not parse")
	}
	if !IsPlaybookConfirmActivityID(id) || IsPlaybookConfirmActivityID("step-confirm-1") {
		t.Fatal("prefix detect")
	}
}
