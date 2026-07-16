import { describe, expect, it } from 'vitest';
import { stepToActivitySnapshot } from '../stepToActivitySnapshot';
import type { Step } from '../v2Types';

function baseStep(over: Partial<Step> = {}): Step {
  return {
    ID: 'step-1',
    TurnID: 'turn-1',
    TaskID: 'task-1',
    SessionID: 'sess-1',
    SpiritSessionID: 'spirit-1',
    Kind: 'action',
    AuthorAgentKey: 'researcher',
    Seq: 3,
    Version: 1,
    Content: 'done',
    Reasoning: '',
    ToolName: 'web.search',
    ToolCallID: 'tc-1',
    ToolArgs: { q: 'x' },
    ToolResult: { hits: 1 },
    ToolDurationMs: 12,
    ToolErrorCode: '',
    Status: 'completed',
    IsFinal: true,
    StartedAt: '2026-07-16T10:00:00.000Z',
    CompletedAt: '2026-07-16T10:00:01.000Z',
    ...over,
  };
}

describe('stepToActivitySnapshot', () => {
  it('maps core Step fields onto Activity', () => {
    const act = stepToActivitySnapshot(baseStep());
    expect(act.id).toBe('step-1');
    expect(act.kind).toBe('action');
    expect(act.status).toBe('completed');
    expect(act.sessionId).toBe('sess-1');
    expect(act.turnId).toBe('turn-1');
    expect(act.toolName).toBe('web.search');
    expect(act.toolArguments).toContain('"q"');
    expect(act.toolResult).toContain('hits');
    expect(act.durationMs).toBe(1000);
    expect(act.meta?.is_final).toBe(true);
    expect(act.meta?.agent_key).toBe('researcher');
  });

  it('falls back sessionId to SpiritSessionID', () => {
    const act = stepToActivitySnapshot(baseStep({ SessionID: '', SpiritSessionID: 'spirit-9' }));
    expect(act.sessionId).toBe('spirit-9');
  });
});
