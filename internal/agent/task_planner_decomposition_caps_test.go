package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// P3a（2026-08-21）：分解 prompt 能力标签与校验门 R2 同源——静态预定义清单
// 与业务 roster 发散会让 capability_unsatisfiable 违例必发，每次组队白付
// 一次修复重分解（17:03 会话 25.4s 第二次 LLM 调用的根因）。

func TestBuildDecompositionPrompt_RosterCapTags(t *testing.T) {
	prompt := buildDecompositionPrompt("do something", nil, 0, []string{"research", "写作"})
	if !strings.Contains(prompt, "current agent roster: research, 写作") {
		t.Error("prompt should carry the real roster tags")
	}
	if !strings.Contains(prompt, "NEVER invent tags outside this list") {
		t.Error("prompt should forbid inventing tags outside the roster")
	}
	if strings.Contains(prompt, "go-backend") {
		t.Error("static predefined list must be replaced when roster tags are provided")
	}
}

func TestBuildDecompositionPrompt_StaticCapFallback(t *testing.T) {
	prompt := buildDecompositionPrompt("do something", nil, 0, nil)
	if !strings.Contains(prompt, "predefined tags: go-backend") {
		t.Error("nil capTags must keep the static predefined list (byte-stable fallback)")
	}
}

func TestBuildDecompositionPrompt_HardCap(t *testing.T) {
	prompt := buildDecompositionPrompt("do something", nil, 0, nil)
	if !strings.Contains(prompt, "Never produce more than 12 subtasks") {
		t.Error("prompt should state the hard subtask cap (verify gate R3 threshold)")
	}
}

func TestBuildDecompositionPrompt_TeamCountSelfCheck(t *testing.T) {
	prompt := buildDecompositionPrompt("do something", nil, 3, nil)
	if !strings.Contains(prompt, "COUNT your subtasks") {
		t.Error("teamCount prompt should require an explicit count self-check")
	}
}

func TestCapabilityRoleUnion(t *testing.T) {
	caps := []biz.AgentCapability{
		{AgentKey: "a", Roles: []string{"Research", " research ", "", "写作"}},
		{AgentKey: "b", Roles: []string{"写作", "Data"}},
	}
	got := capabilityRoleUnion(caps)
	want := []string{"data", "research", "写作"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestDecomposeCapabilityTags_NilCapBuilder(t *testing.T) {
	impl := &taskPlannerImpl{}
	if got := impl.decomposeCapabilityTags(context.Background()); got != nil {
		t.Errorf("nil capBuilder should fail-open to nil, got %v", got)
	}
}

func TestDecomposeCapabilityTags_FromRoster(t *testing.T) {
	reader := &stubAgentReader{agents: []biz.Agent{
		{AgentKey: "finance", Roles: []string{"财务分析"}, Status: "active"},
		{AgentKey: "writer", Roles: []string{"写作"}, Status: "active"},
	}}
	impl := &taskPlannerImpl{
		capBuilder: NewAgentCapabilityBuilder(reader, loggateway.NewNoop()),
		lg:         loggateway.NewNoop(),
	}
	got := impl.decomposeCapabilityTags(context.Background())
	want := []string{"写作", "财务分析"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestDecomposeCapabilityTags_BuildAllErrorFailsOpen(t *testing.T) {
	reader := &stubAgentReader{err: errors.New("db down")}
	impl := &taskPlannerImpl{
		capBuilder: NewAgentCapabilityBuilder(reader, loggateway.NewNoop()),
		lg:         loggateway.NewNoop(),
	}
	if got := impl.decomposeCapabilityTags(context.Background()); got != nil {
		t.Errorf("BuildAll error should fail-open to nil, got %v", got)
	}
}
