import { describe, expect, it } from 'vitest';
import {
  contextRatioFromUsage,
  contextStatusFromRatio,
  isSessionCompressNoticeFromActivityEvent,
  sessionContextPatchFromActivityEvent,
  sessionContextPatchFromCompressMeta,
  sessionContextPatchFromStepUsage,
  sessionContextPatchFromUsage,
} from '../sessionContextPatch';
import type { ActivityEvent, Activity } from '../../../realtime/activityEvent';

function makeActivity(overrides: Partial<Activity> = {}): Activity {
  return {
    id: 'act-1',
    kind: 'task',
    status: 'running',
    session_id: 's1',
    turn_id: 'turn-1',
    parent_activity_id: '',
    timestamp: '2026-06-10T10:00:00.000Z',
    duration_ms: 0,
    seq: 1,
    prompt_tokens: 0,
    completion_tokens: 0,
    content: '',
    reasoning: '',
    tool_name: '',
    tool_category: 'other',
    tool_call_id: '',
    tool_arguments: '',
    tool_result: '',
    tool_duration_ms: 0,
    tool_error_code: '',
    stage: '',
    child_board_id: '',
    spirit_session_id: '',
    team_id: '',
    dag_node_id: '',
    depends_on: [],
    agent_key: '',
    agent_name: '',
    collapsed: false,
    label: '',
    meta: {},
    ...overrides,
  };
}

describe('sessionContextPatch', () => {
  it('derives ratio from runner_completion usage', () => {
    expect(
      contextRatioFromUsage({
        prompt_tokens: 50_000,
        completion_tokens: 100,
        total_tokens: 50_100,
        max_tokens: 128_000,
      }),
    ).toBeCloseTo(50_000 / 128_000);
  });

  it('returns real ratio above 1 when prompt exceeds the window', () => {
    expect(
      contextRatioFromUsage({
        prompt_tokens: 200_000,
        completion_tokens: 0,
        total_tokens: 200_000,
        max_tokens: 128_000,
      }),
    ).toBeCloseTo(200_000 / 128_000);
  });

  it('builds patch with turn token increment', () => {
    const patch = sessionContextPatchFromUsage(
      {
        prompt_tokens: 64_000,
        completion_tokens: 512,
        total_tokens: 64_512,
        max_tokens: 128_000,
        turn_total_tokens: 64_512,
      },
      { input_tokens: 500, output_tokens: 500, total_tokens: 1000, max_context_used_ratio: 0.3 },
    );
    expect(patch?.context_used_ratio).toBeCloseTo(0.5);
    expect(patch?.context_status).toBe('normal');
    expect(patch?.total_tokens).toBe(1000 + 64_512);
    expect(patch?.max_context_used_ratio).toBeCloseTo(0.5);
  });

  it('maps compress metadata to patch', () => {
    const patch = sessionContextPatchFromCompressMeta({
      kind: 'system.session.compress',
      context_used_ratio: 0.22,
      context_used_tokens: 28_000,
      context_status: 'normal',
    });
    expect(patch).toEqual({
      context_used_ratio: 0.22,
      context_used_tokens: 28_000,
      context_status: 'normal',
    });
  });

  it('step usage patch skips total_tokens', () => {
    const patch = sessionContextPatchFromStepUsage({
      prompt_tokens: 70_000,
      context_prompt_tokens: 70_000,
      completion_tokens: 200,
      total_tokens: 70_200,
      max_tokens: 128_000,
      turn_total_tokens: 70_200,
    });
    expect(patch?.context_used_ratio).toBeCloseTo(70_000 / 128_000);
    expect(patch?.total_tokens).toBeUndefined();
    expect(patch?.input_tokens).toBeUndefined();
  });

  it('prefers context_prompt_tokens over prompt_tokens for ratio', () => {
    expect(
      contextRatioFromUsage({
        prompt_tokens: 90_000,
        context_prompt_tokens: 50_000,
        completion_tokens: 100,
        total_tokens: 90_100,
        max_tokens: 128_000,
      }),
    ).toBeCloseTo(50_000 / 128_000);
  });

  it('maps status thresholds', () => {
    expect(contextStatusFromRatio(0.5)).toBe('normal');
    expect(contextStatusFromRatio(0.65)).toBe('warning');
    expect(contextStatusFromRatio(0.85)).toBe('critical');
    expect(contextStatusFromRatio(0.96)).toBe('exceeded');
  });
});

// ── ActivityEvent-based functions ──────────────────────────────────────
describe('isSessionCompressNoticeFromActivityEvent', () => {
  it('detects compress notice activity (stage=text_done + meta.kind)', () => {
    const ev: ActivityEvent = {
      event: 'completed',
      activity: makeActivity({ stage: 'text_done', meta: { kind: 'system.session.compress' } }),
    };
    expect(isSessionCompressNoticeFromActivityEvent(ev)).toBe(true);
  });

  it('returns false for non-text_done stage', () => {
    const ev: ActivityEvent = {
      event: 'completed',
      activity: makeActivity({ stage: 'run_status', meta: { kind: 'system.session.compress' } }),
    };
    expect(isSessionCompressNoticeFromActivityEvent(ev)).toBe(false);
  });

  it('returns false when meta.kind is not system.session.compress', () => {
    const ev: ActivityEvent = {
      event: 'completed',
      activity: makeActivity({ stage: 'text_done', meta: { kind: 'other' } }),
    };
    expect(isSessionCompressNoticeFromActivityEvent(ev)).toBe(false);
  });
});

describe('sessionContextPatchFromActivityEvent', () => {
  it('derives step usage patch from context_usage stage (no total_tokens increment)', () => {
    const ev: ActivityEvent = {
      event: 'updated',
      activity: makeActivity({
        stage: 'context_usage',
        prompt_tokens: 70_000,
        meta: {
          max_tokens: 128_000,
          context_prompt_tokens: 70_000,
          total_tokens: 70_200,
        },
      }),
    };
    const patch = sessionContextPatchFromActivityEvent(ev);
    expect(patch?.context_used_ratio).toBeCloseTo(70_000 / 128_000);
    expect(patch?.total_tokens).toBeUndefined();
    expect(patch?.last_context_window_tokens).toBe(128_000);
  });

  it('derives turn usage patch from runner_completion stage with total_tokens increment', () => {
    const ev: ActivityEvent = {
      event: 'completed',
      activity: makeActivity({
        stage: 'runner_completion',
        prompt_tokens: 64_000,
        completion_tokens: 512,
        meta: {
          max_tokens: 128_000,
          total_tokens: 64_512,
          turn_total_tokens: 64_512,
        },
      }),
    };
    const patch = sessionContextPatchFromActivityEvent(ev, {
      input_tokens: 500,
      output_tokens: 500,
      total_tokens: 1000,
      max_context_used_ratio: 0.3,
    });
    expect(patch?.context_used_ratio).toBeCloseTo(0.5);
    expect(patch?.total_tokens).toBe(1000 + 64_512);
    expect(patch?.max_context_used_ratio).toBeCloseTo(0.5);
  });

  it('derives patch from compress notice meta', () => {
    const ev: ActivityEvent = {
      event: 'completed',
      activity: makeActivity({
        stage: 'text_done',
        meta: {
          kind: 'system.session.compress',
          context_used_ratio: 0.22,
          context_used_tokens: 28_000,
          context_status: 'normal',
        },
      }),
    };
    const patch = sessionContextPatchFromActivityEvent(ev);
    expect(patch).toEqual({
      context_used_ratio: 0.22,
      context_used_tokens: 28_000,
      context_status: 'normal',
    });
  });

  it('returns null for unrelated stage', () => {
    const ev: ActivityEvent = {
      event: 'updated',
      activity: makeActivity({ stage: 'run_status' }),
    };
    expect(sessionContextPatchFromActivityEvent(ev)).toBeNull();
  });

  it('returns null when usage fields are all zero', () => {
    const ev: ActivityEvent = {
      event: 'updated',
      activity: makeActivity({ stage: 'context_usage', meta: {} }),
    };
    expect(sessionContextPatchFromActivityEvent(ev)).toBeNull();
  });

  it('reads prompt_tokens from meta when activity.prompt_tokens is zero', () => {
    const ev: ActivityEvent = {
      event: 'updated',
      activity: makeActivity({
        stage: 'context_usage',
        prompt_tokens: 0,
        meta: {
          prompt_tokens: 50_000,
          max_tokens: 128_000,
        },
      }),
    };
    const patch = sessionContextPatchFromActivityEvent(ev);
    expect(patch?.context_used_ratio).toBeCloseTo(50_000 / 128_000);
  });
});
