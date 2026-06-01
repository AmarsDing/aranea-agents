package evaluation

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/loggateway"

	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	trpcevalset "trpc.group/trpc-go/trpc-agent-go/evaluation/evalset"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/usersimulation"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/runner"
)

const simSystemInstruction = `You are simulating an end user in a multi-turn conversation with an AI assistant.
Follow the conversation plan. Reply with ONLY the next user message — no explanation, no quotes, no role labels.
Stop when the plan objective is satisfied.`

// NewLLMUserSimulator builds trpc usersimulation.Simulator backed by catalog LLM (Phase 5).
// Precedence: env KRATOS_EVAL_SIM_* → system_settings → env KRATOS_EVAL_JUDGE_* → catalog mini/flash.
func NewLLMUserSimulator(catalog *biz.LlmProviderModelUsecase, rt *provider.RoundTrip, sys EvalLLMSettingsReader, lg loggateway.Logger) (usersimulation.Simulator, error) {
	if catalog == nil || rt == nil {
		return nil, fmt.Errorf("llm user simulator: catalog or round trip nil")
	}
	m, err := resolveSimModel(context.Background(), catalog, rt, sys, lg)
	if err != nil {
		return nil, err
	}
	agent := llmagent.New(
		"eval-user-simulator",
		llmagent.WithModel(m),
		llmagent.WithInstruction(simSystemInstruction),
		llmagent.WithDescription("Aranea evaluation user simulator"),
		llmagent.WithGenerationConfig(trpcmodel.GenerationConfig{
			MaxTokens:   intPtr(512),
			Temperature: floatPtr(0.3),
			Stream:      false,
		}),
	)
	simRunner := runner.NewRunner(AppName+"-user-sim", agent)
	return usersimulation.New(simRunner)
}

func intPtr(v int) *int { return &v }

func floatPtr(v float64) *float64 { return &v }

// resolveUserSimulator picks scripted vs LLM simulator based on case metadata and run config.
func resolveUserSimulator(
	cases []biz.EvalCase,
	cfg RunConfig,
	llmSim usersimulation.Simulator,
) usersimulation.Simulator {
	if !cfg.UseUserSimulation && !casesNeedUserSimulation(cases) {
		return nil
	}
	if casesNeedScriptedSimulation(cases) {
		return newScriptedSimulator(cases)
	}
	if llmSim != nil && (cfg.UseUserSimulation || casesNeedLLMSimulation(cases)) {
		return llmSim
	}
	return nil
}

func casesNeedScriptedSimulation(cases []biz.EvalCase) bool {
	for _, c := range cases {
		if ParseCaseMetadata(c.MetadataJSON).HasScriptedSimulation() {
			return true
		}
	}
	return false
}

func casesNeedLLMSimulation(cases []biz.EvalCase) bool {
	for _, c := range cases {
		meta := ParseCaseMetadata(c.MetadataJSON)
		if meta.HasLLMSimulation() {
			return true
		}
	}
	return false
}

// enrichConversationScenario ensures LLM sim cases have a ConversationScenario plan.
func enrichConversationScenario(c *biz.EvalCase, ec *trpcevalset.EvalCase) {
	meta := ParseCaseMetadata(c.MetadataJSON)
	if ec.ConversationScenario != nil {
		return
	}
	if !meta.HasLLMSimulation() && meta.UserSimulation == nil {
		return
	}
	plan := ""
	if meta.UserSimulation != nil {
		plan = strings.TrimSpace(meta.UserSimulation.ConversationPlan)
	}
	if plan == "" {
		plan = "Achieve the goal implied by the expected assistant response: " + strings.TrimSpace(c.ExpectedOutput)
	}
	start := strings.TrimSpace(c.Input)
	ec.ConversationScenario = buildConversationScenario(CaseMetadata{
		UserSimulation: &UserSimMetadata{
			ConversationPlan: plan,
			MaxInvocations:   meta.UserSimulationMaxInvocations(),
		},
	}, start)
}
