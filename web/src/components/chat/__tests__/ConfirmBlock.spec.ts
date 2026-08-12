// web/src/components/chat/__tests__/ConfirmBlock.spec.ts
// 75 M1.4 A5：Step.Danger=true 的确认卡渲染高危徽标。
import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import ConfirmBlock from '../ConfirmBlock.vue';
import zhCN from '../../../i18n/locales/zh-CN';
import type { Step } from '../../../features/chat/v2Types';

const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } });

function mkConfirmStep(overrides: Partial<Step>): Step {
  return {
    ID: 'step-confirm-1',
    TurnID: '',
    TaskID: 'tk1',
    SessionID: 'sess-1',
    SpiritSessionID: 'sess-1',
    Kind: 'confirm',
    AuthorAgentKey: 'a1',
    Seq: 1,
    Version: 1,
    Content: '工具 computer_use_act 需要确认后执行',
    Reasoning: '',
    ToolName: 'computer_use_act',
    ToolCallID: '',
    ToolArgs: null,
    ToolResult: null,
    ToolDurationMs: 0,
    ToolErrorCode: '',
    Status: 'tool_blocked',
    IsFinal: false,
    StartedAt: new Date().toISOString(),
    CompletedAt: null,
    ...overrides,
  } as Step;
}

describe('ConfirmBlock danger badge', () => {
  beforeEach(() => setActivePinia(createPinia()));

  it('renders 高危 badge when step.Danger is true', () => {
    const wrapper = mount(ConfirmBlock, {
      global: { plugins: [i18n] },
      props: { step: mkConfirmStep({ Danger: true }) },
    });
    expect(wrapper.find('.confirm-block__danger').exists()).toBe(true);
    expect(wrapper.text()).toContain('高危');
  });

  it('omits danger badge when step.Danger is falsy', () => {
    const wrapper = mount(ConfirmBlock, {
      global: { plugins: [i18n] },
      props: { step: mkConfirmStep({}) },
    });
    expect(wrapper.find('.confirm-block__danger').exists()).toBe(false);
  });
});

// 75 M1.4 A5 + 需求 §5.3：高危确认卡只保留「允许本次/拒绝」两选项——
// 后端 danger 命中强制逐次确认（授权不生效），会话/持久授权按钮必须隐藏。
describe('ConfirmBlock danger two-button mode', () => {
  beforeEach(() => setActivePinia(createPinia()));

  it('hides session/always grant buttons when step.Danger is true', () => {
    const wrapper = mount(ConfirmBlock, {
      global: { plugins: [i18n] },
      props: { step: mkConfirmStep({ Danger: true }) },
    });
    expect(wrapper.find('.confirm-block__btn--approve').exists()).toBe(true);
    expect(wrapper.find('.confirm-block__btn--reject').exists()).toBe(true);
    expect(wrapper.find('.confirm-block__btn--approve-session').exists()).toBe(false);
    expect(wrapper.find('.confirm-block__btn--approve-always').exists()).toBe(false);
  });

  it('shows all four buttons when step.Danger is falsy', () => {
    const wrapper = mount(ConfirmBlock, {
      global: { plugins: [i18n] },
      props: { step: mkConfirmStep({}) },
    });
    expect(wrapper.find('.confirm-block__btn--approve').exists()).toBe(true);
    expect(wrapper.find('.confirm-block__btn--reject').exists()).toBe(true);
    expect(wrapper.find('.confirm-block__btn--approve-session').exists()).toBe(true);
    expect(wrapper.find('.confirm-block__btn--approve-always').exists()).toBe(true);
  });
});
