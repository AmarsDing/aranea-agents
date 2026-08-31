package agent

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestUsesConversationalContextBudget(t *testing.T) {
	if !usesConversationalContextBudget(biz.Agent{AgentKey: biz.SpiritAgentKey}) {
		t.Fatal("Spirit must use conversational caps")
	}
	if !usesConversationalContextBudget(biz.Agent{AgentKey: biz.VoiceButlerAgentKey}) {
		t.Fatal("voice butler must use conversational caps")
	}
	chat := biz.Agent{AgentKey: "front_desk", Settings: &biz.AgentRuntimeSettings{ToolsProfile: "chat_only"}}
	if !usesConversationalContextBudget(chat) {
		t.Fatal("chat_only profile must use conversational caps")
	}
	ops := biz.Agent{AgentKey: "ops_fault_diagnosis", Settings: &biz.AgentRuntimeSettings{ToolsProfile: "coding"}}
	if usesConversationalContextBudget(ops) {
		t.Fatal("specialist coding profile must not use conversational caps")
	}
}

func TestResolveAssemblyBudget(t *testing.T) {
	soft, hard, on := resolveAssemblyBudget(biz.Agent{})
	if on || soft != 0 || hard != 0 {
		t.Fatalf("anonymous agent hard=0 must stay off, got soft=%d hard=%d on=%v", soft, hard, on)
	}
	soft, hard, on = resolveAssemblyBudget(biz.Agent{AgentKey: biz.SpiritAgentKey})
	if !on || soft != conversationalAssemblySoftTokens || hard != conversationalAssemblyHardTokens {
		t.Fatalf("Spirit hard=0 must default 40K/60K, got soft=%d hard=%d on=%v", soft, hard, on)
	}
	explicit := biz.Agent{
		AgentKey: "ops_fault_diagnosis",
		Settings: &biz.AgentRuntimeSettings{AssemblyBudgetSoftTokens: 1000, AssemblyBudgetHardTokens: 2000},
	}
	soft, hard, on = resolveAssemblyBudget(explicit)
	if !on || soft != 1000 || hard != 2000 {
		t.Fatalf("explicit hard must win, got soft=%d hard=%d on=%v", soft, hard, on)
	}
	specialistOff := biz.Agent{AgentKey: "ops_fault_diagnosis", Settings: &biz.AgentRuntimeSettings{}}
	soft, hard, on = resolveAssemblyBudget(specialistOff)
	if !on || soft != specialistAssemblySoftTokens || hard != specialistAssemblyHardTokens {
		t.Fatalf("specialist hard=0 must default 64K/96K, got soft=%d hard=%d on=%v", soft, hard, on)
	}
	forcedOff := biz.Agent{
		AgentKey: "ops_fault_diagnosis",
		Settings: &biz.AgentRuntimeSettings{AssemblyBudgetHardTokens: -1},
	}
	if _, _, on = resolveAssemblyBudget(forcedOff); on {
		t.Fatal("hard<0 must force the assembly gate off")
	}
}

func TestHardTriggerRatioForConversationalChat(t *testing.T) {
	spirit := biz.Agent{AgentKey: biz.SpiritAgentKey}
	got := hardTriggerRatioForAgent(spirit)
	want := float64(conversationalCompressSoftTopTokens) / float64(conversationalCompressWindowTokens)
	if got != want {
		t.Fatalf("Spirit ratio = %v, want %v (32K/64K)", got, want)
	}
	ops := biz.Agent{AgentKey: "ops_fault_diagnosis"}
	if got := hardTriggerRatioForAgent(ops); got != biz.DefaultHardTriggerRatio {
		t.Fatalf("specialist ratio = %v, want default %v", got, biz.DefaultHardTriggerRatio)
	}
}
