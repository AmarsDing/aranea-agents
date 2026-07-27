// web/src/components/chat/__tests__/StepBlocks.spec.ts
import { describe, it, expect } from 'vitest';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import ThinkingBlock from '../ThinkingBlock.vue';
import ReplyBlock from '../ReplyBlock.vue';
import zhCN from '../../../i18n/locales/zh-CN';
import type { Step } from '../../../features/chat/v2Types';

// Install i18n with real zh-CN messages so components render actual labels
// instead of falling back to i18n keys (useSafeI18n fallback path).
const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN',
  messages: { 'zh-CN': zhCN },
});

function mkStep(over: Partial<Step> = {}): Step {
  return {
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
    Reasoning: 'I should help',
    ToolName: '',
    ToolCallID: '',
    ToolArgs: null,
    ToolResult: null,
    ToolDurationMs: 0,
    ToolErrorCode: '',
    NoticeType: '',
    Status: 'completed',
    IsFinal: false,
    StartedAt: '',
    CompletedAt: null,
    ...over,
  };
}

describe('ThinkingBlock v2', () => {
  it('accepts Step prop', () => {
    const wrapper = mount(ThinkingBlock, {
      props: { step: mkStep({ Kind: 'thinking', Reasoning: 'test reasoning' }) },
      global: { plugins: [i18n] },
    });
    expect(wrapper.text()).toContain('test reasoning');
  });
});

describe('ReplyBlock v2', () => {
  it('accepts Step prop with content', () => {
    const wrapper = mount(ReplyBlock, {
      props: { step: mkStep({ Kind: 'reply', Content: 'Hello world', IsFinal: true }) },
      global: { plugins: [i18n] },
    });
    expect(wrapper.text()).toContain('Hello world');
  });

  // 精灵总结显著化（2026-07-27）：synthesis turn 的 reply step 渲染总结徽章
  it('shows synthesis badge for spirit-synthesis author', () => {
    const wrapper = mount(ReplyBlock, {
      props: {
        step: mkStep({
          Kind: 'reply',
          Content: '总结：全部完成',
          IsFinal: true,
          AuthorAgentKey: 'spirit-synthesis',
        }),
      },
      global: { plugins: [i18n] },
    });
    const badge = wrapper.find('.reply-block__synthesis-badge');
    expect(badge.exists()).toBe(true);
    expect(badge.text()).toContain('任务总结');
  });

  it('hides synthesis badge for normal reply', () => {
    const wrapper = mount(ReplyBlock, {
      props: { step: mkStep({ Kind: 'reply', Content: 'Hello', IsFinal: true }) },
      global: { plugins: [i18n] },
    });
    expect(wrapper.find('.reply-block__synthesis-badge').exists()).toBe(false);
  });
});
