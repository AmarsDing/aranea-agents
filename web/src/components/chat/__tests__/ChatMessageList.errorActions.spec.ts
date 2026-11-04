import { beforeEach, describe, expect, it, vi } from 'vitest';
import { shallowMount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import ChatMessageList from '../ChatMessageList.vue';
import ErrorBlock from '../ErrorBlock.vue';
import SessionPanel from '../v2/SessionPanel.vue';
import TaskCard from '../v2/TaskCard.vue';
import TaskList from '../v2/TaskList.vue';
import TurnContainer from '../v2/TurnContainer.vue';
import TurnList from '../v2/TurnList.vue';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import type { Step, Task, Turn } from '../../../features/chat/v2Types';

vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key }),
}));

vi.mock('../../../features/chat/composables/useLazyTaskHydration', () => ({
  CHAT_SCROLL_EL_KEY: 'chat-scroll-el',
  useLazyTaskHydration: () => ({
    expandTask: vi.fn(),
    toggleCollapse: vi.fn(),
    syncCards: vi.fn(),
    isCollapsed: () => false,
  }),
}));

const task: Task = {
  ID: 'task-1',
  SessionID: 'sess-1',
  UserMessage: 'test request',
  Status: 'failed',
  Seq: 1,
  Version: 1,
  CreatedAt: '2026-11-04T10:00:00Z',
  UpdatedAt: '2026-11-04T10:01:00Z',
  CompletedAt: '2026-11-04T10:01:00Z',
};

const turn: Turn = {
  ID: 'turn-1',
  TaskID: task.ID,
  SessionID: task.SessionID,
  SpiritSessionID: task.SessionID,
  ParentTurnID: '',
  AgentKey: 'agent-1',
  TeamID: '',
  TeamStageID: '',
  Seq: 1,
  Version: 1,
  Status: 'failed',
  StartedAt: '2026-11-04T10:00:00Z',
  CompletedAt: '2026-11-04T10:01:00Z',
};

const step: Step = {
  ID: 'step-1',
  TurnID: turn.ID,
  TaskID: task.ID,
  SessionID: task.SessionID,
  SpiritSessionID: task.SessionID,
  Kind: 'error',
  AuthorAgentKey: turn.AgentKey,
  Seq: 1,
  Version: 1,
  Content: 'request failed',
  Reasoning: '',
  ToolName: '',
  ToolCallID: '',
  ToolArgs: null,
  ToolResult: null,
  ToolDurationMs: 0,
  ToolErrorCode: 'TURN_TIMEOUT',
  Status: 'failed',
  IsFinal: false,
  StartedAt: '2026-11-04T10:01:00Z',
  CompletedAt: '2026-11-04T10:01:00Z',
};

function seedActivity() {
  const store = useChatActivityStore();
  store.upsertTask(task);
  store.upsertTurn(turn);
  store.upsertStep(step);
  store.hydratedTaskIds.add(task.ID);
}

const errorEvents = ['error-retry', 'error-switch-model'] as const;

describe('chat error action propagation', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    seedActivity();
  });

  it.each([
    ['retry', 'error-retry'],
    ['switch-model', 'error-switch-model'],
  ])('TurnContainer maps ErrorBlock %s to %s', async (sourceEvent, targetEvent) => {
    const wrapper = shallowMount(TurnContainer, { props: { turn } });

    wrapper.findComponent(ErrorBlock).vm.$emit(sourceEvent, step);
    await wrapper.vm.$nextTick();

    expect(wrapper.emitted(targetEvent)?.[0]).toEqual([step]);
  });

  it.each(errorEvents)('TurnList forwards %s', async (eventName) => {
    const wrapper = shallowMount(TurnList, { props: { turns: [turn] } });

    wrapper.findComponent(TurnContainer).vm.$emit(eventName, step);
    await wrapper.vm.$nextTick();

    expect(wrapper.emitted(eventName)?.[0]).toEqual([step]);
  });

  it.each(errorEvents)('TaskCard forwards %s', async (eventName) => {
    const wrapper = shallowMount(TaskCard, { props: { task, hydrated: true } });

    wrapper.findComponent(TurnList).vm.$emit(eventName, step);
    await wrapper.vm.$nextTick();

    expect(wrapper.emitted(eventName)?.[0]).toEqual([step]);
  });

  it.each(errorEvents)('TaskList forwards %s', async (eventName) => {
    const wrapper = shallowMount(TaskList, { props: { sessionId: task.SessionID } });

    wrapper.findComponent(TaskCard).vm.$emit(eventName, step);
    await wrapper.vm.$nextTick();

    expect(wrapper.emitted(eventName)?.[0]).toEqual([step]);
  });

  it.each(errorEvents)('SessionPanel forwards %s', async (eventName) => {
    const wrapper = shallowMount(SessionPanel, { props: { sessionId: task.SessionID } });

    wrapper.findComponent(TaskList).vm.$emit(eventName, step);
    await wrapper.vm.$nextTick();

    expect(wrapper.emitted(eventName)?.[0]).toEqual([step]);
  });

  it.each(errorEvents)('ChatMessageList forwards %s', async (eventName) => {
    const wrapper = shallowMount(ChatMessageList, {
      props: {
        sessionKey: task.SessionID,
        sessionId: task.SessionID,
        pendingMessages: [],
        isDark: false,
      },
    });

    wrapper.findComponent(SessionPanel).vm.$emit(eventName, step);
    await wrapper.vm.$nextTick();

    expect(wrapper.emitted(eventName)?.[0]).toEqual([step]);
  });
});
