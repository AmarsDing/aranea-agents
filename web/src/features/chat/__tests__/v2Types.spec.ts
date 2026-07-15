// web/src/features/chat/__tests__/v2Types.spec.ts
import { describe, it, expectTypeOf } from 'vitest';
import type { V2WsEnvelope, Task, Step, StepKind, EventKind, V2Event } from '../v2Types';

describe('v2Types', () => {
  it('V2WsEnvelope has correct shape', () => {
    const env: V2WsEnvelope = { type: 'v2_event', kind: 'task.created', payload: {} as V2Event };
    expectTypeOf(env.type).toMatchTypeOf<string>();
    expectTypeOf(env.kind).toMatchTypeOf<string>();
  });

  it('Task has PascalCase fields matching backend', () => {
    const t: Task = {
      ID: 't1',
      SessionID: 's1',
      UserMessage: 'hi',
      Status: 'running',
      Seq: 1,
      Version: 1,
      CreatedAt: '',
      UpdatedAt: '',
      CompletedAt: null,
    };
    expectTypeOf(t.ID).toEqualTypeOf<string>();
    expectTypeOf(t.CompletedAt).toEqualTypeOf<string | null>();
  });

  it('Step has PascalCase fields', () => {
    const s: Step = {
      ID: 's1',
      TurnID: 't1',
      TaskID: 'tk1',
      SessionID: 's1',
      SpiritSessionID: 's1',
      Kind: 'thinking',
      AuthorAgentKey: 'a1',
      Seq: 1,
      Version: 1,
      Content: '',
      Reasoning: '',
      ToolName: '',
      ToolCallID: '',
      ToolArgs: null,
      ToolResult: null,
      ToolDurationMs: 0,
      ToolErrorCode: '',
      Status: 'pending',
      IsFinal: false,
      StartedAt: '',
      CompletedAt: null,
    };
    expectTypeOf(s.Kind).toEqualTypeOf<StepKind>();
    expectTypeOf(s.ToolArgs).toEqualTypeOf<unknown | null>();
  });

  it('EventKind constants are string literals', () => {
    const k: EventKind = 'task.created';
    expectTypeOf(k).toMatchTypeOf<EventKind>();
  });
});
