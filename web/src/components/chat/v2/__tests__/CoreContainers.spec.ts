// web/src/components/chat/v2/__tests__/CoreContainers.spec.ts
import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import { useChatActivityStore } from '../../../../stores/chat/activityV2Store';
import TaskCard from '../TaskCard.vue';
import TurnContainer from '../TurnContainer.vue';
import zhCN from '../../../../i18n/locales/zh-CN';
import type { Task, Turn, Step } from '../../../../features/chat/v2Types';

const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } });

/** Quasar stubs — label props must render as text. */
const quasarStubs = {
  'q-chip': { props: ['label'], template: '<span class="q-chip-stub">{{ label }}</span>' },
  'q-icon': { template: '<i />' },
  'q-btn': { props: ['label'], template: '<button class="q-btn-stub">{{ label }}</button>' },
  'q-tooltip': { template: '<span />' },
  'q-expansion-item': {
    props: ['label'],
    template: '<div class="q-expansion-stub"><div class="q-expansion-stub__label">{{ label }}</div><slot /></div>',
  },
};

function mkTask(over: Partial<Task> = {}): Task {
  return {
    ID: 'tk1',
    SessionID: 's1',
    UserMessage: 'Hello',
    Status: 'completed',
    Seq: 1,
    Version: 1,
    CreatedAt: '',
    UpdatedAt: '',
    CompletedAt: null,
    ...over,
  } as Task;
}

function mkOrphanStep(over: Partial<Step> = {}): Step {
  return {
    ID: 'st-notice-1',
    TurnID: '',
    TaskID: 'tk1',
    SessionID: 's1',
    SpiritSessionID: 's1',
    Kind: 'notice',
    AuthorAgentKey: 'spirit-synthesis',
    Seq: 9,
    Version: 1,
    Content: '所有团队已完成',
    Reasoning: '',
    ToolName: '',
    ToolCallID: '',
    ToolArgs: null,
    ToolResult: null,
    ToolDurationMs: 0,
    ToolErrorCode: '',
    NoticeType: 'success',
    Status: 'completed',
    IsFinal: false,
    StartedAt: '',
    CompletedAt: null,
    ...over,
  } as Step;
}

describe('v2 Core Containers', () => {
  beforeEach(() => setActivePinia(createPinia()));

  it('TaskCard renders orphan fallback notice but hides system-internal notices', () => {
    const store = useChatActivityStore();
    store.upsertStep(mkOrphanStep()); // success notice "所有团队已完成"
    store.upsertStep(
      mkOrphanStep({ ID: 'st-notice-2', NoticeType: 'context_usage', Content: 'ctx 80%' }),
    );
    const wrapper = mount(TaskCard, {
      props: { task: mkTask() },
      global: { plugins: [i18n], stubs: quasarStubs },
    });
    expect(wrapper.text()).toContain('所有团队已完成');
    expect(wrapper.text()).not.toContain('ctx 80%');
  });

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
