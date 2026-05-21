package evaluation

import (
	"fmt"
	"strings"

	"aranea-agents/internal/biz"

	trpcevalset "trpc.group/trpc-go/trpc-agent-go/evaluation/evalset"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

const (
	AppName = "aranea"

	MetricExactMatch       = "exact_match"
	MetricContainsMatch    = "contains_match"
	MetricLLMAsJudge       = "llm_as_judge"
	MetricToolCallAccuracy = "tool_call_accuracy"
)

func FrameworkMetricNames() []string {
	return []string{MetricExactMatch, MetricContainsMatch, MetricLLMAsJudge, MetricToolCallAccuracy}
}

func BizCasesToEvalSet(dataset biz.EvalDataset, cases []biz.EvalCase) *trpcevalset.EvalSet {
	es := &trpcevalset.EvalSet{
		EvalSetID: dataset.ID,
		Name:      dataset.Name,
	}
	for _, c := range cases {
		es.EvalCases = append(es.EvalCases, bizCaseToEvalCase(c))
	}
	return es
}

func bizCaseToEvalCase(c biz.EvalCase) *trpcevalset.EvalCase {
	evalID := c.ID
	if evalID == "" {
		evalID = fmt.Sprintf("case-%s", c.DatasetID)
	}
	meta := ParseCaseMetadata(c.MetadataJSON)
	ec := &trpcevalset.EvalCase{
		EvalID:       evalID,
		Conversation: invocationsFromCase(c, meta),
	}
	if scenario := buildConversationScenario(meta, c.Input); scenario != nil {
		ec.ConversationScenario = scenario
	}
	enrichEvalCase(c, ec)
	return ec
}

func invocationsFromCase(c biz.EvalCase, meta CaseMetadata) []*trpcevalset.Invocation {
	if meta.HasMultiTurn() {
		return invocationsFromTurns(c.ID, meta.Turns, c.ExpectedOutput)
	}
	userMsg := trpcmodel.NewUserMessage(c.Input)
	expectedMsg := trpcmodel.NewAssistantMessage(c.ExpectedOutput)
	return []*trpcevalset.Invocation{{
		InvocationID:  c.ID + "-inv-0",
		UserContent:   &userMsg,
		FinalResponse: &expectedMsg,
	}}
}

func invocationsFromTurns(caseID string, turns []EvalTurn, fallbackExpected string) []*trpcevalset.Invocation {
	var invs []*trpcevalset.Invocation
	var pendingUser string
	invIdx := 0
	flush := func(user, expected string) {
		if strings.TrimSpace(user) == "" {
			return
		}
		if expected == "" {
			expected = fallbackExpected
		}
		u := trpcmodel.NewUserMessage(user)
		a := trpcmodel.NewAssistantMessage(expected)
		invs = append(invs, &trpcevalset.Invocation{
			InvocationID:  fmt.Sprintf("%s-inv-%d", caseID, invIdx),
			UserContent:   &u,
			FinalResponse: &a,
		})
		invIdx++
	}
	for _, t := range turns {
		role := strings.ToLower(strings.TrimSpace(t.Role))
		content := t.Content
		switch role {
		case "user":
			if pendingUser != "" {
				flush(pendingUser, "")
			}
			pendingUser = content
		case "assistant":
			if pendingUser != "" {
				flush(pendingUser, content)
				pendingUser = ""
			}
		}
	}
	if pendingUser != "" {
		flush(pendingUser, fallbackExpected)
	}
	if len(invs) == 0 && strings.TrimSpace(fallbackExpected) != "" {
		u := trpcmodel.NewUserMessage("")
		a := trpcmodel.NewAssistantMessage(fallbackExpected)
		invs = append(invs, &trpcevalset.Invocation{
			InvocationID:  caseID + "-inv-0",
			UserContent:   &u,
			FinalResponse: &a,
		})
	}
	return invs
}
