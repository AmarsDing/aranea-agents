package evaluation

import (
	beval "aranea-agents/internal/biz/evaluation"
	"aranea-agents/internal/biz"
)

func hashEvalCases(cases []biz.EvalCase) string {
	return beval.HashCases(cases)
}
