import { describe, expect, it } from 'vitest';
import {
  teamStatusMap,
  modeOptions,
  statusOptions,
  roleOptions,
  teamTemplateOptions,
  runtimeEngineOptions,
  failureDefaultOptions,
  parallelFailOptions,
  failureOnErrorOptions,
  BuiltinIndustryId,
  validStatusTransitions,
  isValidStatusTransition,
} from '../teamConstants';

describe('teamConstants', () => {
  it('teamStatusMap covers all known statuses', () => {
    const expected = ['pending', 'running', 'completed', 'failed', 'cancelled', 'interrupted', 'archived', 'active'];
    for (const status of expected) {
      expect(teamStatusMap[status]).toBeDefined();
      expect(teamStatusMap[status].label).toBeTruthy();
      expect(teamStatusMap[status].color).toBeTruthy();
    }
  });

  it('modeOptions includes 5 modes', () => {
    expect(modeOptions).toHaveLength(5);
    const values = modeOptions.map((o) => o.value);
    expect(values).toContain('sequential');
    expect(values).toContain('parallel');
    expect(values).toContain('coordinator');
    expect(values).toContain('critic_loop');
    expect(values).toContain('adaptive');
  });

  it('statusOptions covers all backend statuses', () => {
    expect(statusOptions.map((o) => o.value)).toEqual([
      'pending',
      'running',
      'completed',
      'failed',
      'cancelled',
      'interrupted',
      'archived',
    ]);
  });

  it('roleOptions includes all team roles', () => {
    expect(roleOptions.map((o) => o.value)).toEqual(['worker', 'coordinator', 'synthesizer', 'generator', 'critic']);
  });

  it('teamTemplateOptions has 4 templates', () => {
    expect(teamTemplateOptions).toHaveLength(4);
    for (const opt of teamTemplateOptions) {
      expect(opt.label).toBeTruthy();
      expect(opt.value).toBeTruthy();
      expect(opt.description).toBeTruthy();
    }
  });

  it('runtimeEngineOptions has graph and native', () => {
    expect(runtimeEngineOptions.map((o) => o.value)).toEqual(['graph', 'native']);
  });

  it('failureDefaultOptions has 3 strategies', () => {
    expect(failureDefaultOptions).toHaveLength(3);
  });

  it('parallelFailOptions has continue and abort', () => {
    expect(parallelFailOptions.map((o) => o.value)).toEqual(['continue', 'abort']);
  });

  it('failureOnErrorOptions has await_review and halt', () => {
    expect(failureOnErrorOptions.map((o) => o.value)).toEqual(['await_review', 'halt']);
  });

  it('BuiltinIndustryId is __builtin__', () => {
    expect(BuiltinIndustryId).toBe('__builtin__');
  });

  it('validStatusTransitions mirrors backend state machine', () => {
    // Mirrors teamTransitionRules in internal/biz/team_state_machine.go,
    // including rework (running→pending) and recover (failed/cancelled→pending).
    expect(validStatusTransitions.pending).toEqual(['running', 'cancelled', 'failed']);
    expect(validStatusTransitions.running).toEqual(['completed', 'failed', 'cancelled', 'interrupted', 'pending']);
    expect(validStatusTransitions.interrupted).toEqual(['running']);
    expect(validStatusTransitions.completed).toEqual(['archived']);
    expect(validStatusTransitions.failed).toEqual(['archived', 'pending']);
    expect(validStatusTransitions.cancelled).toEqual(['archived', 'pending']);
    expect(validStatusTransitions.archived).toEqual([]);
  });

  it('isValidStatusTransition works correctly', () => {
    expect(isValidStatusTransition('pending', 'running')).toBe(true);
    expect(isValidStatusTransition('pending', 'completed')).toBe(false);
    expect(isValidStatusTransition('running', 'completed')).toBe(true);
    expect(isValidStatusTransition('running', 'archived')).toBe(false);
    expect(isValidStatusTransition('completed', 'archived')).toBe(true);
    expect(isValidStatusTransition('pending', 'pending')).toBe(true);
  });
});
