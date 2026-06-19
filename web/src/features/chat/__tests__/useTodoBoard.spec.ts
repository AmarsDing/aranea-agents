import { computed } from 'vue';
import { beforeEach, describe, expect, it } from 'vitest';
import { useTodoBoard } from '../composables/useTodoBoard';
import { clearToolEventCache } from '../envelopeToolCall';
import type { Message } from '../../../domain/types';
import type { ToolUseEvent } from '../types';

function makeToolEvent(overrides: Partial<ToolUseEvent> = {}): ToolUseEvent {
  return {
    id: 'ev-1',
    phase: 'after',
    status: 'success',
    agent_id: 'agent-1',
    agent_key: 'agent-1',
    agent_name: 'Agent',
    tool_name: 'todo_write',
    tool_label: '待办管理',
    occurred_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

function makeMessage(toolEvent: ToolUseEvent | null = null, overrides: Partial<Message> = {}): Message {
  return {
    id: 'msg-1',
    session_id: '',
    parent_message_id: '',
    turn_id: '',
    turn_number: 0,
    seq_in_turn: 0,
    role: 'assistant',
    content_markdown: '',
    model_name: '',
    token_in: 0,
    token_out: 0,
    latency_ms: 0,
    status: '',
    attachments_count: 0,
    options_json: toolEvent ? JSON.stringify({ tool_event: toolEvent }) : '{}',
    error_message: '',
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  } as Message;
}

function makeTodoWriteResult(todos: Array<{ content: string; status: string; id?: string }>) {
  return { todos };
}

describe('useTodoBoard', () => {
  // P2-F1: toolEventFromMessage caches parsed results by message.id. Tests
  // reuse id 'msg-1' with different options_json payloads, so the cache must
  // be reset between cases to avoid stale null entries polluting later tests.
  beforeEach(() => {
    clearToolEventCache();
  });

  it('returns null when no messages', () => {
    const msgs = computed(() => []);
    const { todoBoardState } = useTodoBoard(msgs);
    expect(todoBoardState.value).toBeNull();
  });

  it('returns null when no todo_write tool result exists', () => {
    const msgs = computed(() => [makeMessage(null, { role: 'user', content_markdown: 'hello' })]);
    const { todoBoardState } = useTodoBoard(msgs);
    expect(todoBoardState.value).toBeNull();
  });

  it('returns null when todo_write result has empty todos', () => {
    const ev = makeToolEvent({
      tool_name: 'todo_write',
      status: 'success',
      result: makeTodoWriteResult([]),
    });
    const msgs = computed(() => [makeMessage(ev)]);
    const { todoBoardState } = useTodoBoard(msgs);
    expect(todoBoardState.value).toBeNull();
  });

  it('returns null when todo_write status is not success', () => {
    const ev = makeToolEvent({
      tool_name: 'todo_write',
      status: 'failed',
      result: makeTodoWriteResult([{ content: 'Task 1', status: 'pending' }]),
    });
    const msgs = computed(() => [makeMessage(ev)]);
    const { todoBoardState } = useTodoBoard(msgs);
    expect(todoBoardState.value).toBeNull();
  });

  it('parses todo_write result with valid todos', () => {
    const ev = makeToolEvent({
      tool_name: 'todo_write',
      status: 'success',
      result: makeTodoWriteResult([
        { content: 'Task A', status: 'pending', id: 't1' },
        { content: 'Task B', status: 'in_progress', id: 't2' },
        { content: 'Task C', status: 'completed', id: 't3' },
      ]),
      occurred_at: '2026-01-01T12:00:00Z',
    });
    const msgs = computed(() => [makeMessage(ev)]);
    const { todoBoardState } = useTodoBoard(msgs);

    expect(todoBoardState.value).not.toBeNull();
    expect(todoBoardState.value!.todos).toHaveLength(3);
    expect(todoBoardState.value!.todos[0].content).toBe('Task A');
    expect(todoBoardState.value!.todos[0].status).toBe('pending');
    expect(todoBoardState.value!.todos[1].status).toBe('in_progress');
    expect(todoBoardState.value!.todos[2].status).toBe('completed');
    expect(todoBoardState.value!.source).toBe('tool_result');
  });

  it('generates stable id from content when id is missing', () => {
    const ev = makeToolEvent({
      tool_name: 'todo_write',
      status: 'success',
      result: makeTodoWriteResult([{ content: 'Unique task', status: 'pending' }]),
      occurred_at: '2026-01-01T12:00:00Z',
    });
    const msgs = computed(() => [makeMessage(ev)]);
    const { todoBoardState } = useTodoBoard(msgs);

    expect(todoBoardState.value).not.toBeNull();
    expect(todoBoardState.value!.todos[0].id).toBeTruthy();
  });

  it('uses the last successful todo_write when multiple exist', () => {
    const ev1 = makeToolEvent({
      id: 'ev-1',
      tool_name: 'todo_write',
      status: 'success',
      result: makeTodoWriteResult([{ content: 'Old task', status: 'pending', id: 'old' }]),
      occurred_at: '2026-01-01T10:00:00Z',
    });
    const ev2 = makeToolEvent({
      id: 'ev-2',
      tool_name: 'todo_write',
      status: 'success',
      result: makeTodoWriteResult([{ content: 'New task', status: 'in_progress', id: 'new' }]),
      occurred_at: '2026-01-01T12:00:00Z',
    });
    const msgs = computed(() => [makeMessage(ev1), makeMessage(ev2)]);
    const { todoBoardState } = useTodoBoard(msgs);

    expect(todoBoardState.value).not.toBeNull();
    expect(todoBoardState.value!.todos).toHaveLength(1);
    expect(todoBoardState.value!.todos[0].content).toBe('New task');
  });

  it('filters out items missing content or status', () => {
    const ev = makeToolEvent({
      tool_name: 'todo_write',
      status: 'success',
      result: {
        todos: [
          { content: 'Valid task', status: 'pending', id: 'v1' },
          { content: '', status: 'pending', id: 'v2' },
          { content: 'No status', id: 'v3' },
          { status: 'completed', id: 'v4' },
        ],
      },
      occurred_at: '2026-01-01T12:00:00Z',
    });
    const msgs = computed(() => [makeMessage(ev)]);
    const { todoBoardState } = useTodoBoard(msgs);

    expect(todoBoardState.value).not.toBeNull();
    expect(todoBoardState.value!.todos).toHaveLength(1);
    expect(todoBoardState.value!.todos[0].content).toBe('Valid task');
  });
});
