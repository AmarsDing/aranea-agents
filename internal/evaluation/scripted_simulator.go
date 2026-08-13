package evaluation

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcevalset "trpc.group/trpc-go/trpc-agent-go/evaluation/evalset"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/usersimulation"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

// scriptedSimulator drives multi-turn eval with a fixed user script (Phase 5).
type scriptedSimulator struct {
	scripts map[string][]string
}

// scriptedScripts collects per-case user scripts keyed by case ID.
func scriptedScripts(cases []biz.EvalCase, lg loggateway.Logger) map[string][]string {
	scripts := make(map[string][]string)
	for _, c := range cases {
		meta := ParseCaseMetadata(c.MetadataJSON, lg)
		if meta.HasScriptedSimulation() {
			scripts[c.ID] = meta.UserSimulation.Script
		}
	}
	return scripts
}

func newScriptedSimulator(cases []biz.EvalCase, lg loggateway.Logger) usersimulation.Simulator {
	scripts := scriptedScripts(cases, lg)
	if len(scripts) == 0 {
		return nil
	}
	return &scriptedSimulator{scripts: scripts}
}

func (s *scriptedSimulator) Start(_ context.Context, req *usersimulation.StartRequest) (usersimulation.Conversation, error) {
	if req == nil {
		return nil, fmt.Errorf("start request is nil")
	}
	return &scriptedConversation{script: s.scripts[req.EvalCaseID]}, nil
}

type scriptedConversation struct {
	script []string
	idx    int
}

func (c *scriptedConversation) Next(_ context.Context, _ *usersimulation.TurnRequest) (*usersimulation.Decision, error) {
	if c.idx >= len(c.script) {
		return &usersimulation.Decision{Stop: true}, nil
	}
	msg := model.NewUserMessage(c.script[c.idx])
	c.idx++
	return &usersimulation.Decision{Message: &msg}, nil
}

func (c *scriptedConversation) Close() error { return nil }

// hybridSimulator routes each case to the right simulator (P1 混合模拟):
// cases with a fixed script replay it; every other case delegates to the LLM
// simulator. Previously a mixed dataset (some scripted + some LLM cases) got
// the scripted simulator globally, so LLM-marked cases silently received an
// immediately-stopping conversation.
type hybridSimulator struct {
	scripted *scriptedSimulator
	llm      usersimulation.Simulator
}

func (h *hybridSimulator) Start(ctx context.Context, req *usersimulation.StartRequest) (usersimulation.Conversation, error) {
	if req == nil {
		return nil, fmt.Errorf("start request is nil")
	}
	if script := h.scripted.scripts[req.EvalCaseID]; len(script) > 0 {
		return &scriptedConversation{script: script}, nil
	}
	return h.llm.Start(ctx, req)
}

func buildConversationScenario(meta CaseMetadata, startingPrompt string) *trpcevalset.ConversationScenario {
	if !meta.HasUserSimulation() {
		return nil
	}
	us := meta.UserSimulation
	maxInv := us.MaxInvocations
	if maxInv <= 0 {
		if len(us.Script) > 0 {
			// Scripted: one invocation per scripted line plus the opening turn.
			maxInv = len(us.Script) + 1
		} else {
			// LLM-driven: the old fallback (len(script)+1) evaluated to 1 for
			// script-less cases and killed the simulated conversation after a
			// single turn. Fall back to the metadata default (5).
			maxInv = meta.UserSimulationMaxInvocations()
		}
	}
	plan := us.ConversationPlan
	if plan == "" {
		plan = "Follow the scripted user messages until the script is exhausted."
	}
	return &trpcevalset.ConversationScenario{
		StartingPrompt:        startingPrompt,
		ConversationPlan:      plan,
		MaxAllowedInvocations: &maxInv,
	}
}

func casesNeedUserSimulation(cases []biz.EvalCase, lg loggateway.Logger) bool {
	for _, c := range cases {
		if ParseCaseMetadata(c.MetadataJSON, lg).HasUserSimulation() {
			return true
		}
	}
	return false
}
