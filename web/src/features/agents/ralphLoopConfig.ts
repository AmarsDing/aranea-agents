/** Agent Ralph Loop settings — form state and API field mapping. */

import type { AgentRuntimeSettings } from './types';

export type RalphLoopFormState = {
  enabled: boolean;
  max_iterations: number;
  completion_promise: string;
  verify_command: string;
  verify_timeout_seconds: number;
  promise_tag_open: string;
  promise_tag_close: string;
  verify_work_dir: string;
};

export function defaultRalphLoopForm(): RalphLoopFormState {
  return {
    enabled: false,
    max_iterations: 0,
    completion_promise: '',
    verify_command: '',
    verify_timeout_seconds: 0,
    promise_tag_open: '',
    promise_tag_close: '',
    verify_work_dir: '',
  };
}

export function ralphLoopFormFromSettings(s?: AgentRuntimeSettings | null): RalphLoopFormState {
  const d = defaultRalphLoopForm();
  if (!s) return d;
  const promise = (s.ralph_loop_completion_promise ?? '').trim();
  const verify = (s.ralph_loop_verify_command ?? '').trim();
  const maxIter = s.ralph_loop_max_iterations ?? 0;
  d.enabled = maxIter > 0 || promise !== '' || verify !== '';
  d.max_iterations = maxIter;
  d.completion_promise = s.ralph_loop_completion_promise ?? '';
  d.verify_command = s.ralph_loop_verify_command ?? '';
  d.verify_timeout_seconds = s.ralph_loop_verify_timeout_seconds ?? 0;
  d.promise_tag_open = s.ralph_loop_promise_tag_open ?? '';
  d.promise_tag_close = s.ralph_loop_promise_tag_close ?? '';
  d.verify_work_dir = s.ralph_loop_verify_work_dir ?? '';
  return d;
}

export function serializeRalphLoopForm(
  form: RalphLoopFormState,
): Pick<
  AgentRuntimeSettings,
  | 'ralph_loop_max_iterations'
  | 'ralph_loop_completion_promise'
  | 'ralph_loop_verify_command'
  | 'ralph_loop_verify_timeout_seconds'
  | 'ralph_loop_promise_tag_open'
  | 'ralph_loop_promise_tag_close'
  | 'ralph_loop_verify_work_dir'
> {
  if (!form.enabled) {
    return {
      ralph_loop_max_iterations: 0,
      ralph_loop_completion_promise: '',
      ralph_loop_verify_command: '',
      ralph_loop_verify_timeout_seconds: 0,
      ralph_loop_promise_tag_open: '',
      ralph_loop_promise_tag_close: '',
      ralph_loop_verify_work_dir: '',
    };
  }
  return {
    ralph_loop_max_iterations: Math.max(0, form.max_iterations),
    ralph_loop_completion_promise: form.completion_promise.trim(),
    ralph_loop_verify_command: form.verify_command.trim(),
    ralph_loop_verify_timeout_seconds: Math.max(0, form.verify_timeout_seconds),
    ralph_loop_promise_tag_open: form.promise_tag_open.trim(),
    ralph_loop_promise_tag_close: form.promise_tag_close.trim(),
    ralph_loop_verify_work_dir: form.verify_work_dir.trim(),
  };
}

/** Returns a user-visible error message, or null when valid. */
export function validateRalphLoopForm(form: RalphLoopFormState): string | null {
  if (!form.enabled) return null;
  const promise = form.completion_promise.trim();
  const verify = form.verify_command.trim();
  if (!promise && !verify) {
    return 'Ralph Loop 启用后需填写「完成承诺」或「验证命令」至少一项';
  }
  if (form.max_iterations < 0) {
    return '最大迭代次数不能为负数';
  }
  if (form.verify_timeout_seconds < 0) {
    return '验证命令超时不能为负数';
  }
  return null;
}
