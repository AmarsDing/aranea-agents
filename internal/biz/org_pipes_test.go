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
