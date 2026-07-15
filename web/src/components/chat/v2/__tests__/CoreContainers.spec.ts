// web/src/components/chat/v2/__tests__/CoreContainers.spec.ts
import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mount } from '@vue/test-utils';
import { useChatActivityStore } from '../../../../stores/chat/activityV2Store';
import TaskCard from '../TaskCard.vue';
import TurnContainer from '../TurnContainer.vue';
import type { Task, Turn, Step } from '../../../../features/chat/v2Types';

describe('v2 Core Containers', () => {
  beforeEach(() => setActivePinia(createPinia()));

  it('TaskCard renders user message', () => {
    const wrapper = mount(TaskCard, {
      props: {
        task: {
          ID: 'tk1',
          SessionID: 's1',
          UserMessage: 'Hello',
          Status: 'completed',
          Seq: 1,
          Version: 1,
          CreatedAt: '',
          UpdatedAt: '',
          CompletedAt: null,
        } as Task,
      },
    });
    expect(wrapper.text()).toContain('Hello');
  });

  it('TurnContainer renders thinking + reply steps', async () => {
    const store = useChatActivityStore();
    store.upsertStep({
      ID: 's1',
      TurnID: 'turn1',
      TaskID: 'tk1',
      SessionID: 's1',
      SpiritSessionID: 's1',
      Kind: 'thinking',
      AuthorAgentKey: 'a1',
      Seq: 1,
      Version: 1,
      Content: '',
      Reasoning: 'think',
      ToolName: '',
      ToolCallID: '',
      ToolArgs: null,
      ToolResult: null,
      ToolDurationMs: 0,
      ToolErrorCode: '',
      Status: 'completed',
      IsFinal: false,
      StartedAt: '',
      CompletedAt: null,
    } as Step);
    store.upsertStep({
      ID: 's2',
      TurnID: 'turn1',
      TaskID: 'tk1',
      SessionID: 's1',
      SpiritSessionID: 's1',
      Kind: 'reply',
      AuthorAgentKey: 'a1',
      Seq: 2,
      Version: 1,
      Content: 'reply text',
      Reasoning: '',
      ToolName: '',
      ToolCallID: '',
      ToolArgs: null,
      ToolResult: null,
      ToolDurationMs: 0,
      ToolErrorCode: '',
      Status: 'completed',
      IsFinal: true,
      StartedAt: '',
      CompletedAt: null,
    } as Step);
    const wrapper = mount(TurnContainer, {
      props: {
        turn: {
          ID: 'turn1',
          TaskID: 'tk1',
          SessionID: 's1',
          SpiritSessionID: 's1',
          ParentTurnID: '',
          AgentKey: 'a1',
          TeamID: '',
          TeamStageID: '',
          Seq: 1,
          Version: 1,
          Status: 'completed',
          StartedAt: '',
          CompletedAt: null,
        } as Turn,
      },
    });
    expect(wrapper.text()).toContain('think');
    expect(wrapper.text()).toContain('reply text');
  });
});
