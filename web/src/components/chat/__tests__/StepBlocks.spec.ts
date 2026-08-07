// web/src/components/chat/__tests__/StepBlocks.spec.ts
import { describe, it, expect, beforeEach } from 'vitest';
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

describe('ThinkingBlock 流式自动展开', () => {
  beforeEach(() => sessionStorage.clear());

  it('挂载时已处于流式：自动展开显示实时推理内容（而非静止的折叠指示器）', async () => {
    // 场景：step.created 事件以 Status=running 落库，组件挂载时 streaming 已为 true，
    // watch(streaming) 不会触发 false→true 跳变 —— 必须在挂载时主动展开。
    const wrapper = mount(ThinkingBlock, {
      props: {
        step: mkStep({ ID: 'st-stream-1', Status: 'running', Reasoning: '正在实时推理的内容，逐字流入' }),
      },
      global: { plugins: [i18n] },
    });
    await wrapper.vm.$nextTick(); // onMounted 中的 setCollapsed 需一个 tick 重渲染
    expect(wrapper.find('.thinking-block__body').exists()).toBe(true);
    expect(wrapper.text()).toContain('正在实时推理的内容');
  });

  it('挂载时已处于流式但用户本会话显式收起过：尊重用户选择保持收起', () => {
    sessionStorage.setItem('chat:collapse:thinking:st-stream-2', 'true');
    const wrapper = mount(ThinkingBlock, {
      props: {
        step: mkStep({ ID: 'st-stream-2', Status: 'running', Reasoning: '推理内容超过三十个字符的阈值限制' }),
      },
      global: { plugins: [i18n] },
    });
    expect(wrapper.find('.thinking-block__body').exists()).toBe(false);
    expect(wrapper.find('.thinking-block__streaming-indicator').exists()).toBe(true);
  });

  it('挂载时为完成态：保持默认收起，不受流式自动展开影响', () => {
    const wrapper = mount(ThinkingBlock, {
      props: {
        step: mkStep({ ID: 'st-done-1', Status: 'completed', Reasoning: '已完成的较长推理内容，需要超过三十个字符才会进入折叠态而非内联短文本' }),
      },
      global: { plugins: [i18n] },
    });
    expect(wrapper.find('.thinking-block__body').exists()).toBe(false);
    expect(wrapper.find('.thinking-block__collapsed').exists()).toBe(true);
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
