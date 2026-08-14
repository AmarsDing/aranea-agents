package service

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/evaluation"
	"aranea-agents/pkg/loggateway"
)

// NewEvaluationAfterTurnTrigger wires the AfterTurn auto-eval hook (US-5).
func NewEvaluationAfterTurnTrigger(uc *biz.EvalUsecase, runner *evaluation.Runner, lg loggateway.Logger) *evaluation.AfterTurnTrigger {
	return evaluation.NewAfterTurnTrigger(uc, runner, lg)
}
