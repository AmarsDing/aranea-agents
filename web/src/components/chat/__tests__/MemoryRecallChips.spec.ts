// web/src/components/chat/__tests__/MemoryRecallChips.spec.ts
// 记忆召回 chips 折叠行为：默认收起（仅标题行），点击标题展开/收起。
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import { createMemoryHistory, createRouter } from 'vue-router';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import MemoryRecallChips from '../MemoryRecallChips.vue';
import zhCN from '../../../i18n/locales/zh-CN';
import { MEMORY_RECALLED_NOTICE_TYPE } from '../../../features/chat/memoryRecall';
import type { Step } from '../../../features/chat/v2Types';

const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } });

const quasarStubs = {
  'q-icon': { template: '<i />' },
  'q-tooltip': { template: '<span />' },
};

function seedRecallHits(store: ReturnType<typeof useChatActivityStore>, turnId: string) {
  store.upsertStep({
    ID: 'st-recall-1',
    TurnID: turnId,
    TaskID: 'tk1',
    SessionID: 's1',
    SpiritSessionID: 's1',
    Kind: 'notice',
    NoticeType: MEMORY_RECALLED_NOTICE_TYPE,
    AuthorAgentKey: 'a1',
    Seq: 1,
    Version: 1,
    Content: JSON.stringify({
      hits: [
        { layer: 'L3', line: '用户偏好 XX 餐厅', score: 0.91, fact_id: 'f-1' },
        { layer: 'L2', line: '上次聚餐点了日料', score: 0.72 },
      ],
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

describe('MemoryRecallChips 折叠', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    sessionStorage.clear();
  });

  it('默认收起：只显示标题行，不渲染 chips 列表', () => {
    const store = useChatActivityStore();
    seedRecallHits(store, 'turn1');
    const wrapper = mount(MemoryRecallChips, {
      props: { turnId: 'turn1' },
      global: { plugins: [i18n], stubs: quasarStubs },
    });
    expect(wrapper.find('.memory-recall-chips__header').exists()).toBe(true);
    expect(wrapper.find('.memory-recall-chips__list').exists()).toBe(false);
    expect(wrapper.text()).not.toContain('用户偏好 XX 餐厅');
    // i18n 键回归：标题必须渲染为本地化文案，而非原始 key
    expect(wrapper.text()).toContain('已召回 2 条记忆');
    expect(wrapper.text()).not.toContain('chat.memoryRecall.title');
  });

  it('点击标题展开 chips 列表，再点击收起', async () => {
    const store = useChatActivityStore();
    seedRecallHits(store, 'turn1');
    const wrapper = mount(MemoryRecallChips, {
      props: { turnId: 'turn1' },
      global: { plugins: [i18n], stubs: quasarStubs },
    });
    await wrapper.find('.memory-recall-chips__header').trigger('click');
    expect(wrapper.find('.memory-recall-chips__list').exists()).toBe(true);
    expect(wrapper.text()).toContain('用户偏好 XX 餐厅');
    expect(wrapper.text()).toContain('上次聚餐点了日料');

    await wrapper.find('.memory-recall-chips__header').trigger('click');
    expect(wrapper.find('.memory-recall-chips__list').exists()).toBe(false);
  });

  it('展开状态按 turn 记忆（sessionStorage）', async () => {
    const store = useChatActivityStore();
    seedRecallHits(store, 'turn1');
    const first = mount(MemoryRecallChips, {
      props: { turnId: 'turn1' },
      global: { plugins: [i18n], stubs: quasarStubs },
    });
    await first.find('.memory-recall-chips__header').trigger('click');
    first.unmount();

    // 重新挂载（模拟虚拟滚动回收/刷新）：保持用户展开选择
    const second = mount(MemoryRecallChips, {
      props: { turnId: 'turn1' },
      global: { plugins: [i18n], stubs: quasarStubs },
    });
    expect(second.find('.memory-recall-chips__list').exists()).toBe(true);
  });

  it('点击 L3 chip 跳转记忆中心并带 factId', async () => {
    const store = useChatActivityStore();
    seedRecallHits(store, 'turn1');
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', component: { template: '<div />' } },
        { path: '/memory', component: { template: '<div />' } },
      ],
    });
    const wrapper = mount(MemoryRecallChips, {
      props: { turnId: 'turn1', sessionId: 's1', agentKey: 'spirit' },
      global: { plugins: [i18n, router], stubs: quasarStubs },
    });
    await router.isReady();
    const push = vi.spyOn(router, 'push');
    await wrapper.find('.memory-recall-chips__header').trigger('click');
    await wrapper.find('.memory-recall-chips__chip').trigger('click');
    expect(push).toHaveBeenCalledWith({
      path: '/memory',
      query: {
        tab: 'browse',
        layer: 'L3',
        factId: 'f-1',
        agentKey: 'spirit',
        sessionId: 's1',
      },
    });
  });
});
