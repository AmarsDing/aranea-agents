import { describe, expect, it } from 'vitest';
import {
  defaultRalphLoopForm,
  ralphLoopFormFromSettings,
  serializeRalphLoopForm,
  validateRalphLoopForm,
} from '../ralphLoopConfig';

describe('ralphLoopConfig', () => {
  it('serializes disabled form as zeros', () => {
    const form = defaultRalphLoopForm();
    form.enabled = true;
    form.completion_promise = 'DONE';
    form.max_iterations = 3;
    form.enabled = false;
    const out = serializeRalphLoopForm(form);
    expect(out.ralph_loop_max_iterations).toBe(0);
    expect(out.ralph_loop_completion_promise).toBe('');
  });

  it('validates stop condition when enabled', () => {
    const form = defaultRalphLoopForm();
    form.enabled = true;
    form.max_iterations = 2;
    expect(validateRalphLoopForm(form)).toContain('完成承诺');
    form.completion_promise = 'DONE';
    expect(validateRalphLoopForm(form)).toBeNull();
  });

  it('hydrates from settings', () => {
    const form = ralphLoopFormFromSettings({
      ralph_loop_max_iterations: 5,
      ralph_loop_verify_command: 'go test ./...',
    } as never);
    expect(form.enabled).toBe(true);
    expect(form.max_iterations).toBe(5);
  });

  it('serializes verify_command only when enabled', () => {
    const form = defaultRalphLoopForm();
    form.enabled = true;
    form.verify_command = '  go test ./...  ';
    const out = serializeRalphLoopForm(form);
    expect(out.ralph_loop_verify_command).toBe('go test ./...');
    expect(out.ralph_loop_max_iterations).toBe(0);
  });
});
