import { describe, expect, it } from 'vitest';
import {
  teamStatusMap,
  teamModeMap,
  modeOptions,
  statusOptions,
  roleOptions,
  teamTemplateOptions,
  failureDefaultOptions,
  parallelFailOptions,
  failureOnErrorOptions,
  BuiltinGroupId,
  PresetGroupId,
  validStatusTransitions,
  isValidStatusTransition,
} from '../teamConstants';

describe('teamConstants', () => {
  it('teamStatusMap covers all known statuses', () => {
    const expected = [
      'pending',
      'running',
      'completed',
      'partial_failure',
      'failed',
      'cancelled',
      'interrupted',
      'archived',
      'active',
    ];
    for (const status of expected) {
      expect(teamStatusMap[status]).toBeDefined();
      expect(teamStatusMap[status].label).toBeTruthy();
      expect(teamStatusMap[status].color).toBeTruthy();
    }
  });

  it('modeOptions shows 5 UI templates; adaptive stands in for swarm', () => {
    expect(modeOptions).toHaveLength(5);
    const values = modeOptions.map((o) => o.value);
    expect(values).toEqual(['sequential', 'parallel', 'coordinator', 'critic_loop', 'adaptive']);
    expect(values).not.toContain('swarm');
  });

  it('teamModeMap covers all 6 legal API modes', () => {
    expect(Object.keys(teamModeMap).sort()).toEqual(
      ['adaptive', 'coordinator', 'critic_loop', 'parallel', 'sequential', 'swarm'].sort(),
    );
  });

  it('statusOptions covers all backend statuses', () => {
    expect(statusOptions.map((o) => o.value)).toEqual([
      'pending',
      'running',
      'completed',
      'partial_failure',
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

  it('failureDefaultOptions has 3 strategies', () => {
    expect(failureDefaultOptions).toHaveLength(3);
  });

  it('parallelFailOptions has continue and abort', () => {
    expect(parallelFailOptions.map((o) => o.value)).toEqual(['continue', 'abort']);
  });

  it('failureOnErrorOptions has await_review and halt', () => {
    expect(failureOnErrorOptions.map((o) => o.value)).toEqual(['await_review', 'halt']);
  });

  it('BuiltinGroupId is __builtin__', () => {
    expect(BuiltinGroupId).toBe('__builtin__');
  });
  it('PresetGroupId is __preset__', () => {
    expect(PresetGroupId).toBe('__preset__');
  });

  it('validStatusTransitions mirrors backend state machine', () => {
    // Mirrors teamTransitionRules in internal/biz/team_state_machine.go,
    // including rework (running→pending), recover (failed/cancelled/partial_failure→pending)
    // and complete_partial (running→partial_failure).
    expect(validStatusTransitions.pending).toEqual(['running', 'cancelled', 'failed']);
    expect(validStatusTransitions.running).toEqual([
      'completed',
      'partial_failure',
      'failed',
      'cancelled',
      'interrupted',
      'pending',
    ]);
    expect(validStatusTransitions.interrupted).toEqual(['running']);
    expect(validStatusTransitions.completed).toEqual(['archived']);
    expect(validStatusTransitions.partial_failure).toEqual(['archived', 'pending']);
    expect(validStatusTransitions.failed).toEqual(['archived', 'pending']);
    expect(validStatusTransitions.cancelled).toEqual(['archived', 'pending']);
    expect(validStatusTransitions.archived).toEqual([]);
  });

  it('isValidStatusTransition works correctly', () => {
    expect(isValidStatusTransition('pending', 'running')).toBe(true);
    expect(isValidStatusTransition('pending', 'completed')).toBe(false);
    expect(isValidStatusTransition('running', 'completed')).toBe(true);
    expect(isValidStatusTransition('running', 'partial_failure')).toBe(true);
    expect(isValidStatusTransition('partial_failure', 'archived')).toBe(true);
    expect(isValidStatusTransition('partial_failure', 'pending')).toBe(true);
    expect(isValidStatusTransition('partial_failure', 'failed')).toBe(false);
    expect(isValidStatusTransition('running', 'archived')).toBe(false);
    expect(isValidStatusTransition('completed', 'archived')).toBe(true);
    expect(isValidStatusTransition('pending', 'pending')).toBe(true);
  });
});
