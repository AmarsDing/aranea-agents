package evaluation

import (
	"context"
	"time"

	"aranea-agents/pkg/loggateway"

	"trpc.group/trpc-go/trpc-agent-go/evaluation/service"
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
			if args.Request != nil {
				caseID = args.Request.EvalCaseID
			}
			errStr := ""
			if args.Error != nil {
				errStr = args.Error.Error()
			}
			lg.Info("eval.inference.case_done",
				loggateway.StepID("evaluation.inference.case_done"),
				loggateway.Str("eval_case_id", caseID),
				loggateway.Str("error", errStr),
				loggateway.Str("duration", time.Since(args.StartTime).Round(time.Millisecond).String()),
			)
			return &service.AfterInferenceCaseResult{Context: ctx}, nil
		},
		AfterEvaluateCase: func(ctx context.Context, args *service.AfterEvaluateCaseArgs) (*service.AfterEvaluateCaseResult, error) {
			if args == nil {
				return &service.AfterEvaluateCaseResult{Context: ctx}, nil
			}
			caseID := ""
			if args.Request != nil {
				caseID = args.Request.EvalCaseID
			}
			score := float64(0)
			passed := false
			if args.Result != nil {
				score = args.Result.Score
				passed = args.Result.FinalEvalStatus.String() == "passed"
			}
			errStr := ""
			if args.Error != nil {
				errStr = args.Error.Error()
			}
			lg.Info("eval.evaluate.case_done",
				loggateway.StepID("evaluation.evaluate.case_done"),
				loggateway.Str("eval_case_id", caseID),
				loggateway.Float64("score", score),
				loggateway.Bool("passed", passed),
				loggateway.Str("error", errStr),
				loggateway.Str("duration", time.Since(args.StartTime).Round(time.Millisecond).String()),
			)
			return &service.AfterEvaluateCaseResult{Context: ctx}, nil
		},
	})

	return cb
}
