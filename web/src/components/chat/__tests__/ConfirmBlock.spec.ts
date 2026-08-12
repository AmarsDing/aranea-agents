// web/src/components/chat/__tests__/ConfirmBlock.spec.ts
// 75 M1.4 A5：Step.Danger=true 的确认卡渲染高危徽标。
import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mount } from '@vue/test-utils';
import ConfirmBlock from '../ConfirmBlock.vue';
import type { Step } from '../../../features/chat/v2Types';

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
      props: { step: mkConfirmStep({ Danger: true }) },
    });
    expect(wrapper.find('.confirm-block__danger').exists()).toBe(true);
    expect(wrapper.text()).toContain('高危');
  });

  it('omits danger badge when step.Danger is falsy', () => {
    const wrapper = mount(ConfirmBlock, {
      props: { step: mkConfirmStep({}) },
    });
    expect(wrapper.find('.confirm-block__danger').exists()).toBe(false);
  });
});
