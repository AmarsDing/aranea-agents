package evaluation

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"

	trpcevalset "trpc.group/trpc-go/trpc-agent-go/evaluation/evalset"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/usersimulation"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

// scriptedSimulator drives multi-turn eval with a fixed user script (Phase 5).
type scriptedSimulator struct {
	scripts map[string][]string
}

func newScriptedSimulator(cases []biz.EvalCase) usersimulation.Simulator {
	scripts := make(map[string][]string)
	for _, c := range cases {
		meta := ParseCaseMetadata(c.MetadataJSON)
		if meta.HasScriptedSimulation() {
			scripts[c.ID] = meta.UserSimulation.Script
		}
	}
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

func buildConversationScenario(meta CaseMetadata, startingPrompt string) *trpcevalset.ConversationScenario {
	if !meta.HasUserSimulation() {
		return nil
	}
	us := meta.UserSimulation
	maxInv := us.MaxInvocations
	if maxInv <= 0 {
		maxInv = len(us.Script) + 1
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

func casesNeedUserSimulation(cases []biz.EvalCase) bool {
	for _, c := range cases {
		if ParseCaseMetadata(c.MetadataJSON).HasUserSimulation() {
			return true
		}
	}
	return false
}
