import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import KnowledgeRecallChips from '../KnowledgeRecallChips.vue';
import zhCN from '../../../i18n/locales/zh-CN';
import { KNOWLEDGE_RECALLED_NOTICE_TYPE } from '../../../features/chat/knowledgeRecall';
import type { Step } from '../../../features/chat/v2Types';

const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } });

const quasarStubs = {
  'q-icon': { template: '<i />' },
  'q-tooltip': { template: '<span />' },
};

function seedChunks(store: ReturnType<typeof useChatActivityStore>, turnId: string) {
  store.upsertStep({
    ID: 'st-kb-1',
    TurnID: turnId,
    TaskID: 'tk1',
    SessionID: 's1',
    SpiritSessionID: 's1',
    Kind: 'notice',
    NoticeType: KNOWLEDGE_RECALLED_NOTICE_TYPE,
    AuthorAgentKey: 'a1',
    Seq: 1,
    Version: 1,
    Content: JSON.stringify({
      chunks: [{ chunk_id: 'k1', doc_id: 'd1', score: 0.9, line: 'SLA 承诺 99.9%' }],
    }),
    Reasoning: '',
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
}

describe('KnowledgeRecallChips', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    sessionStorage.clear();
  });

  it('defaults collapsed and localizes the title', () => {
    const store = useChatActivityStore();
    seedChunks(store, 'turn1');
    const wrapper = mount(KnowledgeRecallChips, {
      props: { turnId: 'turn1' },
      global: { plugins: [i18n], stubs: quasarStubs },
    });
    expect(wrapper.find('.knowledge-recall-chips__header').exists()).toBe(true);
    expect(wrapper.find('.knowledge-recall-chips__list').exists()).toBe(false);
    expect(wrapper.text()).toContain('已引用 1 条知识');
  });

  it('expands to show the passage line', async () => {
    const store = useChatActivityStore();
    seedChunks(store, 'turn1');
    const wrapper = mount(KnowledgeRecallChips, {
      props: { turnId: 'turn1' },
      global: { plugins: [i18n], stubs: quasarStubs },
    });
    await wrapper.find('.knowledge-recall-chips__header').trigger('click');
    expect(wrapper.text()).toContain('SLA 承诺 99.9%');
  });
});
