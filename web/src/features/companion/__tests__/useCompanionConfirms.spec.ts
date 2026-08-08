// features/companion/__tests__/useCompanionConfirms.spec.ts
import { describe, it, expect } from 'vitest';

import type { Step } from '../../chat/v2Types';
import { pendingConfirmSteps, toConfirmCardModel } from '../useCompanionConfirms';

function makeStep(overrides: Partial<Step> = {}): Step {
  return {
    ID: 'step-1',
    TurnID: 'turn-1',
    TaskID: 'task-1',
    SessionID: 'sess-1',
    SpiritSessionID: 'sess-1',
    Kind: 'confirm',
    AuthorAgentKey: 'default',
    Seq: 1,
    Version: 1,
    Content: '确认打开应用？',
    Reasoning: '',
    ToolName: 'client_open_app',
    ToolCallID: 'call-1',
    ToolArgs: { target: 'weixin' },
    ToolResult: null,
    ToolDurationMs: 0,
    ToolErrorCode: '',
    Status: 'tool_blocked',
    IsFinal: false,
    StartedAt: '2026-08-08T10:00:00Z',
    CompletedAt: null,
    ...overrides,
  };
}

describe('toConfirmCardModel', () => {
  it('extracts target from ToolArgs.target', () => {
    const m = toConfirmCardModel(makeStep());
    expect(m.id).toBe('step-1');
    expect(m.sessionId).toBe('sess-1');
    expect(m.toolName).toBe('client_open_app');
    expect(m.target).toBe('weixin');
    expect(m.description).toBe('确认打开应用？');
    expect(m.startedAt).toBe('2026-08-08T10:00:00Z');
  });

  it('extracts target from ToolArgs.url when target missing', () => {
    const m = toConfirmCardModel(makeStep({ ToolArgs: { url: 'https://example.com' } }));
    expect(m.target).toBe('https://example.com');
  });

  it('falls back to empty target for missing/non-string args', () => {
    expect(toConfirmCardModel(makeStep({ ToolArgs: null })).target).toBe('');
    expect(toConfirmCardModel(makeStep({ ToolArgs: { target: 42 } })).target).toBe('');
    expect(toConfirmCardModel(makeStep({ ToolArgs: 'raw-string' })).target).toBe('');
  });

  it('pretty-prints argsJson and tolerates null', () => {
    const withArgs = toConfirmCardModel(makeStep());
    expect(withArgs.argsJson).toContain('"target": "weixin"');
    expect(toConfirmCardModel(makeStep({ ToolArgs: null })).argsJson).toBe('');
  });
});

describe('pendingConfirmSteps', () => {
  it('keeps only tool_blocked confirm steps of the given session, FIFO by StartedAt', () => {
    const steps = new Map<string, Step>([
      ['a', makeStep({ ID: 'a', StartedAt: '2026-08-08T10:02:00Z' })],
      ['b', makeStep({ ID: 'b', StartedAt: '2026-08-08T10:00:00Z' })],
      ['c', makeStep({ ID: 'c', Status: 'completed', StartedAt: '2026-08-08T09:59:00Z' })],
      ['d', makeStep({ ID: 'd', Kind: 'action', StartedAt: '2026-08-08T09:58:00Z' })],
      ['e', makeStep({ ID: 'e', SessionID: 'other', SpiritSessionID: 'other', StartedAt: '2026-08-08T09:57:00Z' })],
      ['f', makeStep({ ID: 'f', Status: 'cancelled', StartedAt: '2026-08-08T10:01:00Z' })],
    ]);
    const out = pendingConfirmSteps(steps.values(), 'sess-1');
    expect(out.map((s) => s.ID)).toEqual(['b', 'a']);
  });

  it('returns empty when nothing pending', () => {
    expect(pendingConfirmSteps([], 'sess-1')).toEqual([]);
    expect(pendingConfirmSteps([makeStep({ Status: 'completed' })], 'sess-1')).toEqual([]);
  });

  it('includes member-session confirms under the same spirit root (team scenario)', () => {
    const member = makeStep({ ID: 'm', SessionID: 'member-sess', SpiritSessionID: 'sess-1' });
    const out = pendingConfirmSteps([member], 'sess-1');
    expect(out.map((s) => s.ID)).toEqual(['m']);
    // 决议回传用 step 自身的 SessionID（后端校验 step.SessionID == req.session_id）
    expect(out[0].SessionID).toBe('member-sess');
  });
});
