import { describe, it, expect, vi, beforeEach } from 'vitest';
import { computed, ref, nextTick } from 'vue';
import { useConversationTimeline } from '../useConversationTimeline';
import type { Message } from '../../../../domain/types';
import type { Activity, ActivityTreeNode } from '../../activityTypes';

vi.mock('../../../stores/app', () => ({
  useAppStore: vi.fn().mockReturnValue({ agents: [] }),
}));

function makeMessage(overrides: Partial<Message> & Pick<Message, 'id' | 'role'>): Message {
  return {
    session_id: 'sess-1',
    parent_message_id: '',
    turn_id: overrides.turn_id ?? 'turn-1',
    turn_number: 1,
    seq_in_turn: 1,
    content_markdown: '',
    model_name: '',
    token_in: 0,
    token_out: 0,
    latency_ms: 0,
    status: 'ok',
    attachments_count: 0,
    options_json: '',
    error_message: '',
    created_at: '2026-06-21T00:00:00Z',
    ...overrides,
  } as Message;
}

function makeReplyActivity(id: string, turnId: string, content: string, status: string): Activity {
  return {
    id,
    kind: 'reply',
    status: status as Activity['status'],
    sessionId: 'sess-1',
    agentKey: 'agent-1',
    agentName: 'Agent 1',
    content,
    turnId,
    parentActivityId: null,
    timestamp: '2026-06-21T00:00:00Z',
    durationMs: null,
    collapsed: false,
  };
}

describe('useConversationTimeline', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('returns empty array when messages is empty', () => {
    const messages = ref<Message[]>([]);
    const { conversationTurns } = useConversationTimeline({
      messages: computed(() => messages.value),
    });
    expect(conversationTurns.value).toEqual([]);
  });

  it('builds a single turn from user + assistant messages', () => {
    const messages = ref<Message[]>([
      makeMessage({ id: 'u1', role: 'user', turn_id: 't1', content_markdown: 'hello' }),
      makeMessage({ id: 'a1', role: 'assistant', turn_id: 't1', content_markdown: 'hi' }),
    ]);
    const { conversationTurns } = useConversationTimeline({
      messages: computed(() => messages.value),
      activityRawRecords: computed(() => [makeReplyActivity('act-1', 't1', 'hi', 'completed')]),
    });
    expect(conversationTurns.value).toHaveLength(1);
    expect(conversationTurns.value[0].id).toBe('turn-u1');
    expect(conversationTurns.value[0].agentWork.result).toBe('hi');
  });

  it('reuses unchanged turn references across recomputations', async () => {
    const messages = ref<Message[]>([
      makeMessage({ id: 'u1', role: 'user', turn_id: 't1', content_markdown: 'q1' }),
      makeMessage({ id: 'a1', role: 'assistant', turn_id: 't1', content_markdown: 'a1' }),
      makeMessage({ id: 'u2', role: 'user', turn_id: 't2', content_markdown: 'q2' }),
      makeMessage({ id: 'a2', role: 'assistant', turn_id: 't2', content_markdown: 'a2' }),
    ]);
    const rawRecords = ref<Activity[]>([
      makeReplyActivity('act-1', 't1', 'a1', 'completed'),
      makeReplyActivity('act-2', 't2', 'a2', 'completed'),
    ]);
    const { conversationTurns } = useConversationTimeline({
      messages: computed(() => messages.value),
      activityRawRecords: computed(() => rawRecords.value),
    });

    const firstTurns = conversationTurns.value;
    const firstTurn0 = firstTurns[0];
    const firstTurn1 = firstTurns[1];

    // Mutate only the second turn's reply activity content.
    rawRecords.value = [rawRecords.value[0], makeReplyActivity('act-2', 't2', 'a2 updated', 'completed')];
    await nextTick();

    const secondTurns = conversationTurns.value;
    expect(secondTurns[0]).toBe(firstTurn0);
    expect(secondTurns[1]).not.toBe(firstTurn1);
    expect(secondTurns[1].agentWork.result).toBe('a2 updated');
  });

  it('rebuilds a turn when its activity status changes', async () => {
    const messages = ref<Message[]>([
      makeMessage({ id: 'u1', role: 'user', turn_id: 't1', content_markdown: 'q' }),
      makeMessage({ id: 'a1', role: 'assistant', turn_id: 't1', content_markdown: 'a' }),
    ]);
    const rawRecords = ref<Activity[]>([makeReplyActivity('act-1', 't1', 'a', 'running')]);
    const { conversationTurns } = useConversationTimeline({
      messages: computed(() => messages.value),
      activityRawRecords: computed(() => rawRecords.value),
    });

    const firstTurn = conversationTurns.value[0];
    expect(firstTurn.agentWork.status).toBe('running');

    rawRecords.value = [makeReplyActivity('act-1', 't1', 'a', 'completed')];
    await nextTick();

    expect(conversationTurns.value[0]).not.toBe(firstTurn);
    expect(conversationTurns.value[0].agentWork.status).toBe('completed');
  });

  it('rebuilds a turn when activityTree signature changes', async () => {
    const messages = ref<Message[]>([
      makeMessage({ id: 'u1', role: 'user', turn_id: 't1', content_markdown: 'q' }),
      makeMessage({ id: 'a1', role: 'assistant', turn_id: 't1', content_markdown: 'a' }),
    ]);
    const rawRecords = ref<Activity[]>([makeReplyActivity('act-1', 't1', 'a', 'completed')]);
    const activityTree = ref<ActivityTreeNode[]>([
      {
        id: 'act-1',
        status: 'running',
        kind: 'task',
        sessionId: 'sess-1',
        turnId: 'turn-1',
        parentActivityId: null,
        timestamp: '2026-06-21T00:00:00Z',
        durationMs: null,
        collapsed: false,
        children: [],
      },
    ]);
    const { conversationTurns } = useConversationTimeline({
      messages: computed(() => messages.value),
      activityRawRecords: computed(() => rawRecords.value),
      activityTree: computed(() => activityTree.value),
    });

    const firstTurn = conversationTurns.value[0];

    activityTree.value = [
      {
        id: 'act-1',
        status: 'completed',
        kind: 'task',
        sessionId: 'sess-1',
        turnId: 'turn-1',
        parentActivityId: null,
        timestamp: '2026-06-21T00:00:00Z',
        durationMs: null,
        collapsed: false,
        children: [],
      } as ActivityTreeNode,
    ];
    await nextTick();

    expect(conversationTurns.value[0]).not.toBe(firstTurn);
  });
});
