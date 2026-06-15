import { describe, expect, it } from 'vitest';
import { groupMessagesByTurn, lastAssistantTurnBlockIndex, lastAssistant } from '../groupMessagesByTurn';
import type { Message } from '../types';
import type { MessageOrigin } from '../../../domain/types';

function originFromId(id: string): MessageOrigin | undefined {
  if (id.startsWith('pending-user-')) return { kind: 'pending_user', localId: id };
  if (id.startsWith('ws-stream-') || id.startsWith('ws-team-stream-'))
    return { kind: 'streaming', sessionId: id.replace(/^ws-(team-)?stream-/, '') };
  if (id.startsWith('member-')) return { kind: 'team_member', agentKey: id.replace(/^member-/, '') };
  if (id.startsWith('act-') || id.startsWith('tool-')) return { kind: 'tool_activity', toolEventId: id };
  return undefined;
}

function msg(partial: Partial<Message> & Pick<Message, 'id' | 'role'>): Message {
  return {
    session_id: 's1',
    parent_message_id: '',
    turn_id: partial.turn_id ?? '',
    turn_number: partial.turn_number ?? 0,
    seq_in_turn: partial.seq_in_turn ?? 0,
    content_markdown: partial.content_markdown ?? 'body',
    model_name: '',
    token_in: 0,
    token_out: 0,
    latency_ms: partial.latency_ms ?? 0,
    status: partial.status ?? 'ok',
    attachments_count: 0,
    options_json: partial.options_json ?? '',
    error_message: '',
    created_at: partial.created_at ?? '2026-05-23T00:00:00Z',
    origin: partial.origin ?? originFromId(partial.id),
    ...partial,
  };
}

describe('groupMessagesByTurn (turn_id-based)', () => {
  it('groups messages with same turn_id into one block', () => {
    const messages: Message[] = [
      msg({ id: 'u1', role: 'user', turn_id: 'turn-1', created_at: '2026-05-23T00:00:00Z', content_markdown: 'hi' }),
      msg({
        id: 't1',
        role: 'assistant',
        turn_id: 'turn-1',
        created_at: '2026-05-23T00:00:01Z',
        content_markdown: '',
        options_json: '{"schema":"chat.activity/v1","tool_event":{}}',
        status: 'tool_ok',
        latency_ms: 1200,
      }),
      msg({
        id: 'a1',
        role: 'assistant',
        turn_id: 'turn-1',
        created_at: '2026-05-23T00:00:02Z',
        content_markdown: 'done',
      }),
    ];
    const blocks = groupMessagesByTurn(messages);
    expect(blocks).toHaveLength(1);
    expect(blocks[0]?.user?.id).toBe('u1');
    expect(blocks[0]?.tools).toHaveLength(1);
    expect(lastAssistant(blocks[0]!)?.id).toBe('a1');
  });

  it('splits multiple turns by turn_id boundary', () => {
    const messages: Message[] = [
      msg({ id: 'u1', role: 'user', turn_id: 'turn-1', created_at: '2026-05-23T00:00:00Z' }),
      msg({ id: 'a1', role: 'assistant', turn_id: 'turn-1', created_at: '2026-05-23T00:00:01Z' }),
      msg({ id: 'u2', role: 'user', turn_id: 'turn-2', created_at: '2026-05-23T00:00:02Z' }),
      msg({ id: 'a2', role: 'assistant', turn_id: 'turn-2', created_at: '2026-05-23T00:00:03Z' }),
    ];
    expect(groupMessagesByTurn(messages)).toHaveLength(2);
    expect(groupMessagesByTurn(messages)[0]?.user?.id).toBe('u1');
    expect(groupMessagesByTurn(messages)[1]?.user?.id).toBe('u2');
  });

  it('merges orphan tool-only blocks into previous user turn', () => {
    const messages: Message[] = [
      msg({ id: 'u1', role: 'user', turn_id: 'turn-1', created_at: '2026-05-23T00:00:00Z' }),
      msg({
        id: 't1',
        role: 'assistant',
        turn_id: 'turn-1',
        created_at: '2026-05-23T00:00:01Z',
        content_markdown: '',
        options_json: '{"schema":"chat.activity/v1"}',
        status: 'tool_ok',
      }),
      msg({
        id: 't2',
        role: 'assistant',
        turn_id: 'turn-1',
        created_at: '2026-05-23T00:00:02Z',
        content_markdown: '',
        options_json: '{"schema":"chat.activity/v1"}',
        status: 'tool_ok',
      }),
    ];
    const blocks = groupMessagesByTurn(messages);
    expect(blocks).toHaveLength(1);
    expect(blocks[0]?.tools).toHaveLength(2);
  });

  it('places in-flight messages after persisted ones', () => {
    const messages: Message[] = [
      msg({ id: 'u1', role: 'user', turn_id: 'turn-1', created_at: '2026-05-23T00:00:00Z' }),
      msg({
        id: 'a1',
        role: 'assistant',
        turn_id: 'turn-1',
        created_at: '2026-05-23T00:00:01Z',
        content_markdown: 'done',
      }),
      msg({
        id: 'pending-user-abc',
        role: 'user',
        turn_id: 'turn-2',
        created_at: '2026-05-23T00:00:02Z',
        content_markdown: 'new question',
      }),
      msg({
        id: 'ws-stream-s1',
        role: 'assistant',
        turn_id: 'turn-2',
        created_at: '2026-05-23T00:00:03Z',
        status: 'streaming',
      }),
    ];
    const blocks = groupMessagesByTurn(messages);
    expect(blocks).toHaveLength(2);
    expect(blocks[0]?.user?.id).toBe('u1');
    expect(lastAssistant(blocks[0]!)?.id).toBe('a1');
    expect(blocks[1]?.user?.id).toBe('pending-user-abc');
    expect(lastAssistant(blocks[1]!)?.id).toBe('ws-stream-s1');
  });

  it('handles first message not being user', () => {
    const messages: Message[] = [
      msg({
        id: 'a1',
        role: 'assistant',
        turn_id: 'turn-1',
        created_at: '2026-05-23T00:00:00Z',
        content_markdown: 'welcome',
      }),
    ];
    const blocks = groupMessagesByTurn(messages);
    expect(blocks).toHaveLength(1);
    expect(blocks[0]?.user).toBeNull();
    expect(lastAssistant(blocks[0]!)?.id).toBe('a1');
  });

  it('groups team member messages into members array', () => {
    const messages: Message[] = [
      msg({ id: 'u1', role: 'user', turn_id: 'turn-1', created_at: '2026-05-23T00:00:00Z' }),
      msg({ id: 'member-researcher', role: 'assistant', turn_id: 'turn-1', created_at: '2026-05-23T00:00:01Z' }),
      msg({ id: 'a1', role: 'assistant', turn_id: 'turn-1', created_at: '2026-05-23T00:00:02Z' }),
    ];
    const blocks = groupMessagesByTurn(messages);
    expect(blocks).toHaveLength(1);
    expect(blocks[0]?.members).toHaveLength(1);
    expect(blocks[0]?.members[0]?.id).toBe('member-researcher');
  });

  it('lastAssistantTurnBlockIndex anchors on assistant body', () => {
    const blocks = groupMessagesByTurn([
      msg({ id: 'u1', role: 'user', turn_id: 'turn-1', created_at: '2026-05-23T00:00:00Z' }),
      msg({
        id: 't1',
        role: 'assistant',
        turn_id: 'turn-1',
        created_at: '2026-05-23T00:00:01Z',
        options_json: '{"schema":"chat.activity/v1"}',
        content_markdown: '',
      }),
      msg({
        id: 'a1',
        role: 'assistant',
        turn_id: 'turn-1',
        created_at: '2026-05-23T00:00:02Z',
        content_markdown: 'answer',
      }),
    ]);
    expect(lastAssistantTurnBlockIndex(blocks)).toBe(0);
  });

  it('groups by turn_id when available', () => {
    const messages: Message[] = [
      msg({ id: 'u1', role: 'user', turn_id: 't1', turn_number: 1, created_at: '2026-05-23T00:00:00Z' }),
      msg({ id: 'a1', role: 'assistant', turn_id: 't1', turn_number: 1, created_at: '2026-05-23T00:00:01Z' }),
      msg({ id: 'u2', role: 'user', turn_id: 't2', turn_number: 2, created_at: '2026-05-23T00:00:02Z' }),
      msg({ id: 'a2', role: 'assistant', turn_id: 't2', turn_number: 2, created_at: '2026-05-23T00:00:03Z' }),
    ];
    const blocks = groupMessagesByTurn(messages);
    expect(blocks).toHaveLength(2);
    expect(lastAssistant(blocks[0]!)?.id).toBe('a1');
    expect(lastAssistant(blocks[1]!)?.id).toBe('a2');
  });

  it('builds rounds array for multi-assistant turn (ReAct loop)', () => {
    const messages: Message[] = [
      msg({ id: 'u1', role: 'user', turn_id: 't1', created_at: '2026-05-23T00:00:00Z' }),
      msg({ id: 'a1', role: 'assistant', turn_id: 't1', created_at: '2026-05-23T00:00:01Z', content_markdown: 'think1' }),
      msg({ id: 'tool1', role: 'assistant', turn_id: 't1', created_at: '2026-05-23T00:00:02Z', options_json: '{"schema":"chat.activity/v1"}', content_markdown: '' }),
      msg({ id: 'a2', role: 'assistant', turn_id: 't1', created_at: '2026-05-23T00:00:03Z', content_markdown: 'think2' }),
      msg({ id: 'tool2', role: 'assistant', turn_id: 't1', created_at: '2026-05-23T00:00:04Z', options_json: '{"schema":"chat.activity/v1"}', content_markdown: '' }),
      msg({ id: 'a3', role: 'assistant', turn_id: 't1', created_at: '2026-05-23T00:00:05Z', content_markdown: 'final' }),
    ];
    const blocks = groupMessagesByTurn(messages);
    expect(blocks).toHaveLength(1);
    expect(blocks[0]!.assistants).toHaveLength(3);
    expect(blocks[0]!.rounds).toHaveLength(3);
    // Round 1: a1 + tool1
    expect(blocks[0]!.rounds[0]!.assistant.id).toBe('a1');
    expect(blocks[0]!.rounds[0]!.tools).toHaveLength(1);
    expect(blocks[0]!.rounds[0]!.tools[0]!.id).toBe('tool1');
    // Round 2: a2 + tool2
    expect(blocks[0]!.rounds[1]!.assistant.id).toBe('a2');
    expect(blocks[0]!.rounds[1]!.tools).toHaveLength(1);
    expect(blocks[0]!.rounds[1]!.tools[0]!.id).toBe('tool2');
    // Round 3: a3, no tools
    expect(blocks[0]!.rounds[2]!.assistant.id).toBe('a3');
    expect(blocks[0]!.rounds[2]!.tools).toHaveLength(0);
  });

  it('attaches pre-round tools to first round when tools arrive before first assistant', () => {
    const messages: Message[] = [
      msg({ id: 'u1', role: 'user', turn_id: 't1', created_at: '2026-05-23T00:00:00Z' }),
      msg({ id: 'tool0', role: 'assistant', turn_id: 't1', created_at: '2026-05-23T00:00:01Z', options_json: '{"schema":"chat.activity/v1"}', content_markdown: '' }),
      msg({ id: 'a1', role: 'assistant', turn_id: 't1', created_at: '2026-05-23T00:00:02Z', content_markdown: 'reply' }),
    ];
    const blocks = groupMessagesByTurn(messages);
    expect(blocks).toHaveLength(1);
    expect(blocks[0]!.assistants).toHaveLength(1);
    expect(blocks[0]!.rounds).toHaveLength(1);
    // tool0 should be in the first round's tools (pre-round buffer)
    expect(blocks[0]!.rounds[0]!.assistant.id).toBe('a1');
    expect(blocks[0]!.rounds[0]!.tools).toHaveLength(1);
    expect(blocks[0]!.rounds[0]!.tools[0]!.id).toBe('tool0');
  });

  it('produces empty rounds for user-only turn', () => {
    const messages: Message[] = [
      msg({ id: 'u1', role: 'user', turn_id: 't1', created_at: '2026-05-23T00:00:00Z' }),
    ];
    const blocks = groupMessagesByTurn(messages);
    expect(blocks).toHaveLength(1);
    expect(blocks[0]!.assistants).toHaveLength(0);
    expect(blocks[0]!.rounds).toHaveLength(0);
    expect(lastAssistant(blocks[0]!)).toBeNull();
  });

  it('lastAssistant returns null for block with no assistants', () => {
    const block = groupMessagesByTurn([
      msg({ id: 'u1', role: 'user', turn_id: 't1', created_at: '2026-05-23T00:00:00Z' }),
    ])[0]!;
    expect(lastAssistant(block)).toBeNull();
  });

  it('splits blocks on role=user even when turn_id is identical (red line #14)', () => {
    const messages: Message[] = [
      msg({ id: 'u1', role: 'user', turn_id: 'same-turn', created_at: '2026-05-23T00:00:00Z' }),
      msg({ id: 'a1', role: 'assistant', turn_id: 'same-turn', created_at: '2026-05-23T00:00:01Z', content_markdown: 'reply1' }),
      msg({ id: 'u2', role: 'user', turn_id: 'same-turn', created_at: '2026-05-23T00:00:02Z' }),
      msg({ id: 'a2', role: 'assistant', turn_id: 'same-turn', created_at: '2026-05-23T00:00:03Z', content_markdown: 'reply2' }),
    ];
    const blocks = groupMessagesByTurn(messages);
    expect(blocks).toHaveLength(2);
    expect(blocks[0]?.user?.id).toBe('u1');
    expect(lastAssistant(blocks[0]!)?.id).toBe('a1');
    expect(blocks[1]?.user?.id).toBe('u2');
    expect(lastAssistant(blocks[1]!)?.id).toBe('a2');
  });

  it('splits blocks on role=user when turn_id is empty', () => {
    const messages: Message[] = [
      msg({ id: 'u1', role: 'user', turn_id: '', created_at: '2026-05-23T00:00:00Z' }),
      msg({ id: 'a1', role: 'assistant', turn_id: '', created_at: '2026-05-23T00:00:01Z', content_markdown: 'reply1' }),
      msg({ id: 'u2', role: 'user', turn_id: '', created_at: '2026-05-23T00:00:02Z' }),
      msg({ id: 'a2', role: 'assistant', turn_id: '', created_at: '2026-05-23T00:00:03Z', content_markdown: 'reply2' }),
    ];
    const blocks = groupMessagesByTurn(messages);
    expect(blocks).toHaveLength(2);
    expect(blocks[0]?.user?.id).toBe('u1');
    expect(blocks[1]?.user?.id).toBe('u2');
  });
});
