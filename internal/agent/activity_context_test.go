package agent

import (
	"testing"

	"github.com/google/uuid"
)

// S-3（2026-08-05）：TeamStage/TeamRun/MemberSession 确定性 ID 引入 run
// 维度（rootTaskID）。同团队每轮 turn 必须派生全新 ID 链，否则第二轮 turn
// 复用第一轮的 team_stages_v2 行——FSM completed→running 转换被拒、
// outcome 哨兵版本带阻塞 created 写入，状态永久冻结。

func TestNewTeamStageActivityID_RunDimension(t *testing.T) {
	teamID := "team-1"
	taskA := "task-a"
	taskB := "task-b"

	a1 := NewTeamStageActivityID(teamID, taskA)
	a2 := NewTeamStageActivityID(teamID, taskA)
	if a1 != a2 {
		t.Errorf("same (teamID, rootTaskID) must derive identical ID: %q vs %q", a1, a2)
	}

	b := NewTeamStageActivityID(teamID, taskB)
	if a1 == b {
		t.Errorf("different rootTaskID must derive different TeamStageID (run isolation): both %q", a1)
	}

	otherTeam := NewTeamStageActivityID("team-2", taskA)
	if a1 == otherTeam {
		t.Errorf("different teamID must derive different TeamStageID: both %q", a1)
	}
}

func TestNewTeamStageActivityID_EmptyRootTaskFallsBackToLegacy(t *testing.T) {
	// Degraded compat: exotic paths without a turn ctx (e.g. recovery) keep
	// the legacy teamID-only formula so their writes stay self-consistent.
	got := NewTeamStageActivityID("team-1", "")
	want := TeamStageActivityID(legacyTeamStageIDForTest("team-1"))
	if got != want {
		t.Errorf("empty rootTaskID must fall back to legacy teamID-only formula: got %q want %q", got, want)
	}
}

func TestRunIsolatedIDChain_InheritsFromTeamStage(t *testing.T) {
	teamID := "team-1"
	stageA := string(NewTeamStageActivityID(teamID, "task-a"))
	stageB := string(NewTeamStageActivityID(teamID, "task-b"))

	runA := NewTeamRunV2ID(stageA)
	runB := NewTeamRunV2ID(stageB)
	if runA == runB {
		t.Error("TeamRunV2ID must differ across runs (inherits isolation from TeamStageID)")
	}

	msA := NewMemberSessionActivityID(runA, "agent-1")
	msB := NewMemberSessionActivityID(runB, "agent-1")
	if msA == msB {
		t.Error("MemberSessionActivityID must differ across runs (inherits isolation from TeamRunV2ID)")
	}

	// Same member within one run converges on ONE row (upsert-by-ID).
	if again := NewMemberSessionActivityID(runA, "agent-1"); again != msA {
		t.Errorf("same (runID, agentKey) must derive identical member session ID: %q vs %q", again, msA)
	}
}

// legacyTeamStageIDForTest reproduces the pre-S-3 formula
// (SHA1(teamStageNamespace, teamID)) so the fallback test pins the exact
// legacy value instead of a re-derivation through production code.
func legacyTeamStageIDForTest(teamID string) string {
	return uuid.NewSHA1(teamStageNamespace, []byte(teamID)).String()
}
