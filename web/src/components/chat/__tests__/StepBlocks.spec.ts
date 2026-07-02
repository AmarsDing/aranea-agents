// web/src/components/chat/__tests__/StepBlocks.spec.ts
import { describe, it, expect } from 'vitest';
import { mount } from '@vue/test-utils';
import ThinkingBlock from '../ThinkingBlock.vue';
import ReplyBlock from '../ReplyBlock.vue';
import type { Step } from '../../../features/chat/v2Types';

function mkStep(over: Partial<Step> = {}): Step {
  return {
    ID: 's1', TurnID: 't1', TaskID: 'tk1', SessionID: 's1',
    SpiritSessionID: 's1', Kind: 'thinking', AuthorAgentKey: 'a1',
    Seq: 1, Version: 1, Content: '', Reasoning: 'I should help',
    ToolName: '', ToolCallID: '', ToolArgs: null, ToolResult: null,
    ToolDurationMs: 0, ToolErrorCode: '', Status: 'completed',
    IsFinal: false, StartedAt: '', CompletedAt: null, ...over,
  };
}

describe('ThinkingBlock v2', () => {
  it('accepts Step prop', () => {
    const wrapper = mount(ThinkingBlock, {
      props: { step: mkStep({ Kind: 'thinking', Reasoning: 'test reasoning' }) },
    });
    expect(wrapper.text()).toContain('test reasoning');
  });
});

describe('ReplyBlock v2', () => {
  it('accepts Step prop with content', () => {
    const wrapper = mount(ReplyBlock, {
      props: { step: mkStep({ Kind: 'reply', Content: 'Hello world', IsFinal: true }) },
    });
    expect(wrapper.text()).toContain('Hello world');
  });

  it('shows final label when IsFinal', () => {
    const wrapper = mount(ReplyBlock, {
      props: { step: mkStep({ Content: 'hi', IsFinal: true }) },
    });
    expect(wrapper.text()).toContain('最终回复');
  });
});
