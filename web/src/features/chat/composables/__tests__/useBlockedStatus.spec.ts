// web/src/features/chat/composables/__tests__/useBlockedStatus.spec.ts
import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { ref } from 'vue';
import { useChatActivityStore } from '../../../../stores/chat/activityV2Store';
import { useBlockedStatus } from '../useBlockedStatus';
import type { Task, Step } from '../../v2Types';

describe('useBlockedStatus v2', () => {
  beforeEach(() => setActivePinia(createPinia()));

  it('returns not-blocked for completed task', () => {
    const store = useChatActivityStore();
    store.upsertTask({
      ID: 'tk1',
      SessionID: 's1',
      UserMessage: '',
      Status: 'completed',
      Seq: 1,
      Version: 1,
      CreatedAt: '',
      UpdatedAt: '',
      CompletedAt: '',
    } as Task);
    const tasks = ref(store.getSessionTasks('s1'));
    const { blockedInfo } = useBlockedStatus(tasks);
    expect(blockedInfo.value.type).toBe('none');
    expect(blockedInfo.value.blocked).toBe(false);
  });

  it('detects tool_blocked step', () => {
    const store = useChatActivityStore();
    store.upsertTask({
      ID: 'tk1',
      SessionID: 's1',
      UserMessage: '',
      Status: 'running',
      Seq: 1,
      Version: 1,
      CreatedAt: '',
      UpdatedAt: '',
      CompletedAt: null,
    } as Task);
    store.upsertStep({
      ID: 's1',
      TurnID: 't1',
      TaskID: 'tk1',
      SessionID: 's1',
      SpiritSessionID: 's1',
      Kind: 'action',
      AuthorAgentKey: 'a1',
      Seq: 1,
      Version: 1,
      Content: '',
      Reasoning: '',
      ToolName: 'shell',
      ToolCallID: '',
      ToolArgs: null,
      ToolResult: null,
      ToolDurationMs: 0,
      ToolErrorCode: '',
      Status: 'tool_blocked',
      IsFinal: false,
      StartedAt: '',
      CompletedAt: null,
    } as Step);
    const tasks = ref(store.getSessionTasks('s1'));
    const { blockedInfo } = useBlockedStatus(tasks);
    expect(blockedInfo.value.type).toBe('tool');
    expect(blockedInfo.value.blocked).toBe(true);
  });
});
