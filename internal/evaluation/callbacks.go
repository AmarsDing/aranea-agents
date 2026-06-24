package evaluation

import (
	"context"
	"time"

	"aranea-agents/pkg/loggateway"

	"trpc.group/trpc-go/trpc-agent-go/evaluation/service"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/status"
)

// NewEvalCallbacks creates evaluation lifecycle callbacks that log progress
// and enable real-time evaluation stage awareness.
func NewEvalCallbacks(lg loggateway.Logger) *service.Callbacks {
	cb := service.NewCallbacks()

	cb.Register("aranea-progress", &service.Callback{
		AfterInferenceCase: func(ctx context.Context, args *service.AfterInferenceCaseArgs) (*service.AfterInferenceCaseResult, error) {
			if args == nil {
				return &service.AfterInferenceCaseResult{Context: ctx}, nil
			}
			caseID := ""
			if args.Result != nil {
				caseID = args.Result.EvalCaseID
			}
			lg.Info("eval.inference.case_done",
				loggateway.StepID("evaluation.inference.case_done"),
				loggateway.Str("eval_case_id", caseID),
				loggateway.Err(args.Error),
				loggateway.Str("duration", time.Since(args.StartTime).Round(time.Millisecond).String()),
			)
			return &service.AfterInferenceCaseResult{Context: ctx}, nil
		},
		AfterEvaluateCase: func(ctx context.Context, args *service.AfterEvaluateCaseArgs) (*service.AfterEvaluateCaseResult, error) {
			if args == nil {
				return &service.AfterEvaluateCaseResult{Context: ctx}, nil
			}
			evalSetID := ""
			if args.Request != nil {
				evalSetID = args.Request.EvalSetID
			}
			caseID := ""
			passed := false
			if args.Result != nil {
				caseID = args.Result.EvalID
				passed = args.Result.FinalEvalStatus == status.EvalStatusPassed
			}
			lg.Info("eval.evaluate.case_done",
				loggateway.StepID("evaluation.evaluate.case_done"),
				loggateway.Str("eval_set_id", evalSetID),
				loggateway.Str("eval_case_id", caseID),
				loggateway.Bool("passed", passed),
				loggateway.Err(args.Error),
				loggateway.Str("duration", time.Since(args.StartTime).Round(time.Millisecond).String()),
			)
			return &service.AfterEvaluateCaseResult{Context: ctx}, nil
		},
	})

	return cb
}
