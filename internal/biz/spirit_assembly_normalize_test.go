package biz

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// D8-6（87-orchestration-chain-hardening Wave 2）：归一化移到
// ResolveGraphTemplateRoute 之后殿后——builtin 模板（dispatch/adaptive/coordinator）
// 对 ≤1 成员的 coordinator 覆盖同样被归一为 sequential；builder 侧防御纵深
// 保证任何不经 AssembleTeam 归一化的调用方也不会给单成员授 synthesizer。
//
// 不变量 I1：len(agentIDs) <= 1 ⇒ 终态 def.mode != coordinator；
// 单成员恒 RoleWorker；coordinator 模式 RoleSynthesizer 仅在成员数 ≥2 时存在。

type assembleDefShape struct {
	Mode    string `json:"mode"`
	Members []struct {
		Role string `json:"role"`
	} `json:"members"`
	EnableStateDeliverable bool `json:"enable_state_deliverable"`
}

func newAssembleTestUsecase(spiritSessionID string) *SpiritTeamUsecase {
	teamRepo := newMemSpiritTeamRepo()
	sessionRepo := newMemSpiritSessionRepo()
	sessionRepo.seedSpirit(spiritSessionID)
	transactor := &memSpiritTransactor{}
	teamUC := NewTeamUsecase(TeamUsecaseOpts{Reader: teamRepo, Writer: teamRepo, RunReader: teamRepo, RunWriter: teamRepo, StepRepo: teamRepo, DeadLetter: teamRepo, Lg: loggateway.NewNoop()})
	sessionUC := NewSessionUsecase(sessionRepo, &memSpiritAgentLookup{}, NewSessionTeamLookup(teamRepo), nil, nil, nil, nil, nil, nil, loggateway.NewNoop(), nil)
	return NewSpiritTeamUsecase(teamUC, sessionUC, &memSpiritAgentResolver{}, loggateway.NewNoop(), WithSpiritTransactor(transactor))
}

// ① 单成员 + Mode=coordinator + GraphTemplateID=dispatch：
// route 会把终态 mode 重置回 coordinator；归一化必须殿后保证终态 sequential。
// pre-fix RED：终态 def.mode=coordinator、members[0].role=synthesizer。
func TestAssembleTeam_SingleMember_DispatchTemplate_NormalizesToSequential(t *testing.T) {
	uc := newAssembleTestUsecase("spirit-d8-1")
	result, err := uc.AssembleTeam(context.Background(), SpiritTeamParams{
		SpiritSessionID: "spirit-d8-1",
		TaskDescription: "single-member dispatch task",
		AgentKeys:       []string{"agent-1"},
		Mode:            TeamModeCoordinator,
		GraphTemplateID: "dispatch",
	})
	if err != nil {
		t.Fatalf("AssembleTeam failed: %v", err)
	}
	var def assembleDefShape
	if err := json.Unmarshal([]byte(result.Team.DefinitionJSON), &def); err != nil {
		t.Fatalf("definition JSON not parseable: %v", err)
	}
	if def.Mode != TeamModeSequential {
		t.Errorf("def.mode = %q, want sequential (dispatch overwrite must be re-normalized)", def.Mode)
	}
	if len(def.Members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(def.Members))
	}
	if def.Members[0].Role != RoleWorker {
		t.Errorf("members[0].role = %q, want worker (single-member never synthesizer)", def.Members[0].Role)
	}
	if result.Team.Topology != TeamModeSequential {
		t.Errorf("Team.Topology = %q, want sequential", result.Team.Topology)
	}
}

// ② 单成员 + Mode=sequential + GraphTemplateID=dispatch：
// route 曾把 sequential 翻回 coordinator；归一化殿后保持终态 sequential+worker。
// pre-fix RED：route 把 sequential 翻回 coordinator。
func TestAssembleTeam_SingleMember_SequentialMode_PlusDispatchTemplate_Normalizes(t *testing.T) {
	uc := newAssembleTestUsecase("spirit-d8-2")
	result, err := uc.AssembleTeam(context.Background(), SpiritTeamParams{
		SpiritSessionID: "spirit-d8-2",
		TaskDescription: "single-member sequential routed to dispatch",
		AgentKeys:       []string{"agent-1"},
		Mode:            TeamModeSequential,
		GraphTemplateID: "dispatch",
	})
	if err != nil {
		t.Fatalf("AssembleTeam failed: %v", err)
	}
	var def assembleDefShape
	if err := json.Unmarshal([]byte(result.Team.DefinitionJSON), &def); err != nil {
		t.Fatalf("definition JSON not parseable: %v", err)
	}
	if def.Mode != TeamModeSequential {
		t.Errorf("def.mode = %q, want sequential", def.Mode)
	}
	if len(def.Members) != 1 || def.Members[0].Role != RoleWorker {
		t.Errorf("members[0].role = %q, want worker", def.Members[0].Role)
	}
	if result.Team.Topology != TeamModeSequential {
		t.Errorf("Team.Topology = %q, want sequential", result.Team.Topology)
	}
}

// ③ 单成员 + Mode=""（默认 coordinator）+ GraphTemplateID=adaptive：
// adaptive 归 dispatch 族——同样的 route 覆盖 → 归一化路径。
// pre-fix RED：route 把已归一化的结果翻回 coordinator。
func TestAssembleTeam_SingleMember_EmptyMode_PlusAdaptiveTemplate_Normalizes(t *testing.T) {
	uc := newAssembleTestUsecase("spirit-d8-3")
	result, err := uc.AssembleTeam(context.Background(), SpiritTeamParams{
		SpiritSessionID: "spirit-d8-3",
		TaskDescription: "single-member adaptive task",
		AgentKeys:       []string{"agent-1"},
		Mode:            "",
		GraphTemplateID: "adaptive",
	})
	if err != nil {
		t.Fatalf("AssembleTeam failed: %v", err)
	}
	var def assembleDefShape
	if err := json.Unmarshal([]byte(result.Team.DefinitionJSON), &def); err != nil {
		t.Fatalf("definition JSON not parseable: %v", err)
	}
	if def.Mode != TeamModeSequential {
		t.Errorf("def.mode = %q, want sequential (adaptive→dispatch overwrite must be re-normalized)", def.Mode)
	}
	if len(def.Members) != 1 || def.Members[0].Role != RoleWorker {
		t.Errorf("members[0].role = %q, want worker", def.Members[0].Role)
	}
	if result.Team.Topology != TeamModeSequential {
		t.Errorf("Team.Topology = %q, want sequential", result.Team.Topology)
	}
}

// ④ 多成员 + dispatch：mode=coordinator、members[0]=synthesizer——回归锁
// （pre/post GREEN）。多成员路径不受本修影响。
func TestAssembleTeam_MultiMember_DispatchTemplate_KeepsCoordinator(t *testing.T) {
	uc := newAssembleTestUsecase("spirit-d8-4")
	result, err := uc.AssembleTeam(context.Background(), SpiritTeamParams{
		SpiritSessionID: "spirit-d8-4",
		TaskDescription: "multi-member dispatch task",
		AgentKeys:       []string{"agent-1", "agent-2"},
		Mode:            TeamModeCoordinator,
		GraphTemplateID: "dispatch",
	})
	if err != nil {
		t.Fatalf("AssembleTeam failed: %v", err)
	}
	var def assembleDefShape
	if err := json.Unmarshal([]byte(result.Team.DefinitionJSON), &def); err != nil {
		t.Fatalf("definition JSON not parseable: %v", err)
	}
	if def.Mode != TeamModeCoordinator {
		t.Errorf("def.mode = %q, want coordinator (multi-member dispatch must keep coordinator)", def.Mode)
	}
	if len(def.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(def.Members))
	}
	if def.Members[0].Role != RoleSynthesizer {
		t.Errorf("members[0].role = %q, want synthesizer", def.Members[0].Role)
	}
	if def.Members[1].Role != RoleWorker {
		t.Errorf("members[1].role = %q, want worker", def.Members[1].Role)
	}
	if result.Team.Topology != TeamModeCoordinator {
		t.Errorf("Team.Topology = %q, want coordinator", result.Team.Topology)
	}
}

// ⑤ 单成员 + review_loop：维持 validateTeamDefinition 响亮失败——
// critic_loop 要求 generator+critic，静默豁免会掩盖剧本配置错误。
// （pre/post GREEN，锁定策略差异。）
func TestAssembleTeam_SingleMember_ReviewLoop_FailsLoudly(t *testing.T) {
	uc := newAssembleTestUsecase("spirit-d8-5")
	_, err := uc.AssembleTeam(context.Background(), SpiritTeamParams{
		SpiritSessionID: "spirit-d8-5",
		TaskDescription: "single-member review_loop task",
		AgentKeys:       []string{"agent-1"},
		Mode:            TeamModeCoordinator,
		GraphTemplateID: "review_loop",
	})
	if err == nil {
		t.Fatal("single-member review_loop must fail loudly (critic_loop requires generator+critic); got nil error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "critic_loop") && !strings.Contains(msg, "generator") {
		t.Errorf("expected critic_loop/generator validation error, got: %v", err)
	}
}

// ⑥ 防御纵深：不经 AssembleTeam 归一化、直调 builder（coordinator + 1 member）
// 也不得授 synthesizer——与 parallel 末成员规则同源（合成者前提=存在其他成员产出）。
// pre-fix RED：role=synthesizer。
func TestBuildSpiritTeamDefinitionJSON_CoordinatorSingleMember_NeverSynthesizer(t *testing.T) {
	raw := buildSpiritTeamDefinitionJSON(TeamModeCoordinator, []string{"agent-1"}, loggateway.NewNoop(), false, nil, nil)
	var def assembleDefShape
	if err := json.Unmarshal([]byte(raw), &def); err != nil {
		t.Fatalf("definition JSON not parseable: %v", err)
	}
	if len(def.Members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(def.Members))
	}
	if def.Members[0].Role != RoleWorker {
		t.Errorf("members[0].role = %q, want worker (builder defense: coordinator branch must require len>1)", def.Members[0].Role)
	}
}

// ⑦ DAG 单成员 + dispatch：enable_state_deliverable==true 与归一化并存——
// DAG 交付通道（requireDeliverable = DagNodeID != ""）不受归一化影响。
// pre-fix RED：终态 def.mode=coordinator（Fix-1 零交付门 + 角色错位并存）。
func TestAssembleTeam_DAGSingleMember_DispatchTemplate_KeepsDeliverableChannel(t *testing.T) {
	uc := newAssembleTestUsecase("spirit-d8-7")
	result, err := uc.AssembleTeam(context.Background(), SpiritTeamParams{
		SpiritSessionID: "spirit-d8-7",
		TaskDescription: "dag single-member dispatch task",
		AgentKeys:       []string{"agent-1"},
		Mode:            TeamModeCoordinator,
		DagNodeID:       "st_1",
		GraphTemplateID: "dispatch",
	})
	if err != nil {
		t.Fatalf("AssembleTeam failed: %v", err)
	}
	var def assembleDefShape
	if err := json.Unmarshal([]byte(result.Team.DefinitionJSON), &def); err != nil {
		t.Fatalf("definition JSON not parseable: %v", err)
	}
	if !def.EnableStateDeliverable {
		t.Error("enable_state_deliverable must stay true for DAG single-member team (requireDeliverable)")
	}
	if def.Mode != TeamModeSequential {
		t.Errorf("def.mode = %q, want sequential", def.Mode)
	}
	if len(def.Members) != 1 || def.Members[0].Role != RoleWorker {
		t.Errorf("members[0].role = %q, want worker", def.Members[0].Role)
	}
}
