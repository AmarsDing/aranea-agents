package evaluation

import (
	"fmt"

	"aranea-agents/internal/biz"

	trpcevalset "trpc.group/trpc-go/trpc-agent-go/evaluation/evalset"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

const (
	// AppName is the trpc EvalSet namespace for Aranea datasets (EP-RT-08).
	AppName = "aranea"

	MetricExactMatch        = "exact_match"
	MetricContainsMatch     = "contains_match"
	MetricLLMAsJudge        = "llm_as_judge"
	MetricToolCallAccuracy  = "tool_call_accuracy"
)

// FrameworkMetricNames lists metrics aligned with trpc-agent-go evaluators.
func FrameworkMetricNames() []string {
	return []string{MetricExactMatch, MetricContainsMatch, MetricLLMAsJudge, MetricToolCallAccuracy}
}

// BizCasesToEvalSet maps persisted biz cases into a trpc EvalSet for framework alignment.
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
	userMsg := trpcmodel.NewUserMessage(c.Input)
	expectedMsg := trpcmodel.NewAssistantMessage(c.ExpectedOutput)
	inv := &trpcevalset.Invocation{
		InvocationID:  evalID + "-inv",
		UserContent:   &userMsg,
		FinalResponse: &expectedMsg,
	}
	return &trpcevalset.EvalCase{
		EvalID:       evalID,
		Conversation: []*trpcevalset.Invocation{inv},
	}
}
