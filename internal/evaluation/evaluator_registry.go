package evaluation

import (
	trpceval "trpc.group/trpc-go/trpc-agent-go/evaluation/evaluator"
	finalresponse "trpc.group/trpc-go/trpc-agent-go/evaluation/evaluator/finalresponse"
	llmfinalresponse "trpc.group/trpc-go/trpc-agent-go/evaluation/evaluator/llm/finalresponse"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/evaluator/llm/hallucination"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/evaluator/llm/rubriccritic"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/evaluator/llm/rubricknowledgerecall"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/evaluator/llm/rubricreferencecritic"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/evaluator/llm/rubricresponse"
	llmtemplate "trpc.group/trpc-go/trpc-agent-go/evaluation/evaluator/llm/template"
	trpcevalregistry "trpc.group/trpc-go/trpc-agent-go/evaluation/evaluator/registry"
	tooltrajectory "trpc.group/trpc-go/trpc-agent-go/evaluation/evaluator/tooltrajectory"
)

func RegisterBuiltinEvaluators(reg trpcevalregistry.Registry) error {
	evaluators := []struct {
		name string
		e    trpceval.Evaluator
	}{
		{name: "tool_trajectory_avg_score", e: tooltrajectory.New()},
		{name: "final_response_avg_score", e: finalresponse.New()},
		{name: "llm_final_response", e: llmfinalresponse.New()},
		{name: "llm_rubric_critic", e: rubriccritic.New()},
		{name: "llm_rubric_response", e: rubricresponse.New()},
		{name: "llm_rubric_reference_critic", e: rubricreferencecritic.New()},
		{name: "llm_rubric_knowledge_recall", e: rubricknowledgerecall.New()},
		{name: "llm_hallucinations", e: hallucination.New()},
		{name: "llm_judge_template", e: llmtemplate.New()},
	}
	for _, ev := range evaluators {
		if err := reg.Register(ev.name, ev.e); err != nil {
			return err
		}
	}
	return nil
}
