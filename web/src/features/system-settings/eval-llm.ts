import type { EvalLLMSettings } from '../../services/kratos/system_setting/v1/index';

export type EvalLLMForm = {
  simProvider: string;
  simModel: string;
  judgeProvider: string;
  judgeModel: string;
};

export const DEFAULT_EVAL_LLM_FORM: EvalLLMForm = {
  simProvider: 'openai',
  simModel: 'gpt-4o-mini',
  judgeProvider: '',
  judgeModel: '',
};

export function evalLLMFromSettings(raw?: EvalLLMSettings | null): EvalLLMForm {
  return {
    simProvider: raw?.simProvider?.trim() || DEFAULT_EVAL_LLM_FORM.simProvider,
    simModel: raw?.simModel?.trim() || DEFAULT_EVAL_LLM_FORM.simModel,
    judgeProvider: raw?.judgeProvider ?? DEFAULT_EVAL_LLM_FORM.judgeProvider,
    judgeModel: raw?.judgeModel ?? DEFAULT_EVAL_LLM_FORM.judgeModel,
  };
}
