package service

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/evaluation"
)

// NewEvaluationAfterTurnTrigger wires the AfterTurn auto-eval hook (US-5).
func NewEvaluationAfterTurnTrigger(uc *biz.EvalUsecase, runner *evaluation.Runner) *evaluation.AfterTurnTrigger {
	return evaluation.NewAfterTurnTrigger(uc, runner)
}
