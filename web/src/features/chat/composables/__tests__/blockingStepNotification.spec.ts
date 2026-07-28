import { describe, expect, it } from 'vitest';
import { collectBlockingSteps, buildNotificationRoute, type BlockingNotification } from '../../blockingStepNotification';
import type { Step } from '../../v2Types';

function makeStep(over: Partial<Step>): Step {
  return {
    ID: 'step-1',
    TurnID: 'turn-1',
    TaskID: 'task-1',
    SessionID: 'sess-1',
    SpiritSessionID: 'spirit-1',
    Kind: 'confirm',
    AuthorAgentKey: 'agent-a',
    Seq: 1,
    Version: 1,
    Content: '允许安装 curl 吗？',
    Reasoning: '',
    ToolName: '',
    ToolCallID: '',
    ToolArgs: null,
    ToolResult: null,
    ToolDurationMs: 0,
    ToolErrorCode: '',
    Status: 'running',
    IsFinal: false,
    StartedAt: '2026-07-28T00:00:00Z',
    CompletedAt: null,
    ...over,
  };
}

describe('collectBlockingSteps', () => {
  it('collects running confirm and clarify steps as notifications', () => {
    const steps = [
      makeStep({ ID: 'c1', Kind: 'confirm', Status: 'running' }),
      makeStep({ ID: 'c2', Kind: 'clarify', Status: 'running', Content: '目标是哪台机器？' }),
    ];
    const out = collectBlockingSteps(steps, new Set());
    expect(out.map((n) => n.stepId)).toEqual(['c1', 'c2']);
    expect(out[0].kind).toBe('confirm');
    expect(out[1].kind).toBe('clarify');
    expect(out[0].sessionId).toBe('sess-1');
  });

  it('skips non-blocking kinds and non-running statuses', () => {
    const steps = [
      makeStep({ ID: 't1', Kind: 'thinking', Status: 'running' }),
      makeStep({ ID: 'c1', Kind: 'confirm', Status: 'completed' }),
      makeStep({ ID: 'c2', Kind: 'confirm', Status: 'failed' }),
      makeStep({ ID: 'r1', Kind: 'reply', Status: 'running' }),
    ];
    expect(collectBlockingSteps(steps, new Set())).toEqual([]);
  });

  it('dedups steps already notified', () => {
    const steps = [makeStep({ ID: 'c1', Kind: 'confirm', Status: 'running' })];
    const first = collectBlockingSteps(steps, new Set());
    expect(first).toHaveLength(1);
    const second = collectBlockingSteps(steps, new Set(first.map((n) => n.stepId)));
    expect(second).toEqual([]);
  });

  it('truncates long content for the notification body', () => {
    const long = 'x'.repeat(200);
    const out = collectBlockingSteps([makeStep({ Content: long })], new Set());
    expect(out[0].body.length).toBeLessThanOrEqual(81); // 80 chars + ellipsis
    expect(out[0].body.endsWith('…')).toBe(true);
  });
});

describe('buildNotificationRoute', () => {
  it('builds the mobile chat route with session query', () => {
    const n: BlockingNotification = {
      stepId: 'c1',
      sessionId: 'sess-9',
      kind: 'confirm',
      title: 't',
      body: 'b',
    };
    expect(buildNotificationRoute(n)).toBe('/mobile/chat?session=sess-9');
  });

  it('falls back to the mobile sessions tab when sessionId is empty', () => {
    const n: BlockingNotification = {
      stepId: 'c1',
      sessionId: '',
      kind: 'clarify',
      title: 't',
      body: 'b',
    };
    expect(buildNotificationRoute(n)).toBe('/mobile/sessions');
  });
});
