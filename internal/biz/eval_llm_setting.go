package biz

import "strings"

// EvalLLMSetting holds platform defaults for evaluation UserSim and LLM-as-Judge.
// Env KRATOS_EVAL_SIM_* / KRATOS_EVAL_JUDGE_* take precedence at runtime.
type EvalLLMSetting struct {
	SimProvider   string
	SimModel      string
	JudgeProvider string
	JudgeModel    string
}

func (s EvalLLMSetting) SimConfigured() bool {
	return strings.TrimSpace(s.SimProvider) != "" && strings.TrimSpace(s.SimModel) != ""
}

func (s EvalLLMSetting) JudgeConfigured() bool {
	return strings.TrimSpace(s.JudgeProvider) != "" && strings.TrimSpace(s.JudgeModel) != ""
}

// ApplyEvalLLMPatch merges an update onto current eval LLM settings.
func ApplyEvalLLMPatch(cur EvalLLMSetting, simProvider, simModel, judgeProvider, judgeModel string) EvalLLMSetting {
	out := cur
	if simProvider != "" || simModel != "" {
		out.SimProvider = strings.TrimSpace(simProvider)
		out.SimModel = strings.TrimSpace(simModel)
	}
	if judgeProvider != "" || judgeModel != "" {
		out.JudgeProvider = strings.TrimSpace(judgeProvider)
		out.JudgeModel = strings.TrimSpace(judgeModel)
	}
	return out
}
