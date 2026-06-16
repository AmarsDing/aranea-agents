package evaluation

import "context"

// Deprecated: LLMJudge is superseded by the framework's WithJudgeRunner + llm_final_response
// evaluator. The framework provides structured {score, reason} output, multi-sample aggregation,
// and automatic Judge Runner injection. This type is retained only for backward compatibility
// with external references. New code should use the framework path exclusively.
type LLMJudge func(ctx context.Context, input, expected, actual string) (float32, error)
