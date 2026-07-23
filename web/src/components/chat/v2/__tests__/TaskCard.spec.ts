// web/src/components/chat/v2/__tests__/TaskCard.spec.ts
// 设计：docs/superpowers/specs/2026-07-23-chat-history-lazy-load-design.md §4.4
import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mount } from '@vue/test-utils';
import TaskCard from '../TaskCard.vue';
import { useChatActivityStore } from '../../../../stores/chat/activityV2Store';
import type { Task } from '../../../../features/chat/v2Types';

function mkTask(over: Partial<Task> = {}): Task {
  return {
    ID: 't-1',
    SessionID: 'sess-1',
    UserMessage: '帮我写一份季度总结',
    Status: 'completed',
    Seq: 1,
    Version: 1,
    CreatedAt: '2026-07-23T10:00:00Z',
    UpdatedAt: '2026-07-23T10:01:30Z',
    CompletedAt: '2026-07-23T10:01:30Z',
    ...over,
  };
}

describe('TaskCard lazy hydration states', () => {
  beforeEach(() => setActivePinia(createPinia()));

  it('collapsed (!hydrated): renders user panel + meta-bar, zero execution DOM', () => {
    const wrapper = mount(TaskCard, { props: { task: mkTask(), hydrated: false } });
    // 用户指令面板原样
    expect(wrapper.find('.task-user-panel__text').text()).toBe('帮我写一份季度总结');
    // meta-bar：状态徽章 + 耗时
    expect(wrapper.find('.task-meta-bar').exists()).toBe(true);
    expect(wrapper.find('.task-meta-bar__duration').text()).toContain('1m30s');
    // 零执行过程 DOM
    expect(wrapper.find('.turn-list').exists()).toBe(false);
    expect(wrapper.find('.task-card__collapse-btn').exists()).toBe(false);
  });

  it('collapsed card click emits hydrate', async () => {
    const wrapper = mount(TaskCard, { props: { task: mkTask(), hydrated: false } });
    await wrapper.find('.task-card').trigger('click');
    expect(wrapper.emitted('hydrate')?.length).toBe(1);
    expect(wrapper.emitted('hydrate')?.[0]).toEqual([mkTask()]);
  });

  it('action buttons do not bubble to card click (no hydrate on copy/regenerate)', async () => {
    const wrapper = mount(TaskCard, { props: { task: mkTask(), hydrated: false } });
    await wrapper.find('.task-user-panel__action-btn').trigger('click');
    expect(wrapper.emitted('hydrate')).toBeUndefined();
  });

  it('loading state renders shimmer skeleton instead of meta-bar', () => {
    const wrapper = mount(TaskCard, {
      props: { task: mkTask(), hydrated: false, hydrationState: 'loading' },
    });
    expect(wrapper.find('.task-card__skeleton').exists()).toBe(true);
    expect(wrapper.findAll('.task-card__skeleton-bar').length).toBe(3);
    expect(wrapper.find('.task-meta-bar').exists()).toBe(false);
  });

  it('error state meta-bar shows retry hint and re-emits hydrate on click', async () => {
    const wrapper = mount(TaskCard, {
      props: { task: mkTask(), hydrated: false, hydrationState: 'error' },
    });
    expect(wrapper.find('.task-meta-bar--error').exists()).toBe(true);
    await wrapper.find('.task-card').trigger('click');
    expect(wrapper.emitted('hydrate')?.length).toBe(1);
  });

  it('hydrated + !collapsed: full render + collapse button emits toggle-collapse', async () => {
    const store = useChatActivityStore();
    store.upsertTask(mkTask());
    const wrapper = mount(TaskCard, { props: { task: mkTask(), hydrated: true, collapsed: false } });
    expect(wrapper.find('.task-meta-bar').exists()).toBe(false);
    const btn = wrapper.find('.task-card__collapse-btn');
    expect(btn.exists()).toBe(true);
    await btn.trigger('click');
    expect(wrapper.emitted('toggle-collapse')?.length).toBe(1);
  });

  it('hydrated + collapsed: meta-bar again, click emits toggle-collapse (no refetch)', async () => {
    const wrapper = mount(TaskCard, { props: { task: mkTask(), hydrated: true, collapsed: true } });
    expect(wrapper.find('.task-meta-bar').exists()).toBe(true);
    await wrapper.find('.task-card').trigger('click');
    expect(wrapper.emitted('toggle-collapse')?.length).toBe(1);
    expect(wrapper.emitted('hydrate')).toBeUndefined();
  });
});
