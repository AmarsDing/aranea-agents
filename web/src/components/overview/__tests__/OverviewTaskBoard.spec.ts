import { defineComponent } from 'vue';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import { describe, expect, it } from 'vitest';
import zh from '../../../i18n/locales/zh-CN';
import OverviewTaskBoard from '../OverviewTaskBoard.vue';
import type { TaskSituationItem } from '../../../features/session/types';

const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zh } });
const PaginationStub = defineComponent({
  props: { page: { type: Number, required: true } },
  emits: ['update:page'],
  template: '<button data-testid="next-page" @click="$emit(\'update:page\', page + 1)">next</button>',
});

function mountBoard(overrides: Record<string, unknown> = {}) {
  return mount(OverviewTaskBoard, {
    props: {
      items: [],
      total: 0,
      limit: 20,
      offset: 0,
      loading: false,
      error: '',
      query: { sort_by: 'updated_at', sort_order: 'desc' },
      ...overrides,
    },
    global: { plugins: [i18n], stubs: { AppRegistryPagination: PaginationStub } },
  });
}

const longItem: TaskSituationItem = {
  task_id: 'task-long',
  session_id: 'session-cross-project',
  status: 'waiting',
  title: '跨项目发布任务'.repeat(30),
  current_step_summary: '等待负责人确认上线窗口'.repeat(20),
};

describe('OverviewTaskBoard states and server pagination', () => {
  it('distinguishes loading, error, and actionable empty states', async () => {
    const loading = mountBoard({ loading: true });
    expect(loading.find('[data-testid="task-board-loading"]').exists()).toBe(true);
    expect(loading.find('[data-testid="task-board-empty"]').exists()).toBe(false);

    const failure = mountBoard({ error: 'offline' });
    expect(failure.find('[role="alert"]').text()).toContain('offline');
    expect(failure.find('[data-testid="task-board-empty"]').exists()).toBe(false);

    const empty = mountBoard({ query: { status: 'waiting', keyword: 'release' } });
    expect(empty.find('[data-testid="task-board-empty"]').exists()).toBe(true);
    await empty.get('[data-testid="task-board-create"]').trigger('click');
    expect(empty.emitted('create')).toHaveLength(1);
  });

  it('keeps authoritative absence visible and emits the whole drill target', async () => {
    const wrapper = mountBoard({ items: [longItem], total: 1 });
    expect(wrapper.get('[data-testid="task-title-task-long"]').text()).toBe(longItem.title);
    expect(wrapper.get('[data-testid="task-executor-task-long"]').text()).toBe('未采集');
    expect(wrapper.get('[data-testid="task-artifacts-task-long"]').text()).toBe('未采集');
    await wrapper.get('[data-testid="task-drill-task-long"]').trigger('click');
    expect(wrapper.emitted('drill')?.[0]).toEqual([longItem]);
  });

  it('emits a server offset instead of slicing the current page', async () => {
    const wrapper = mountBoard({ items: [longItem], total: 35, limit: 20, offset: 0 });
    await wrapper.get('[data-testid="next-page"]').trigger('click');
    expect(wrapper.emitted('page')?.[0]).toEqual([20]);
  });

  it('retains the previous rows while a later refresh fails', () => {
    const wrapper = mountBoard({ items: [longItem], total: 35, error: 'offline' });
    expect(wrapper.find('[role="alert"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="task-drill-task-long"]').exists()).toBe(true);
  });

  it('keeps expanded task details under the same authoritative projection', async () => {
    const wrapper = mountBoard({ items: [longItem], total: 1, expandable: true, expandedTaskIds: [] });
    expect(wrapper.find('[data-testid="task-detail-task-long"]').exists()).toBe(false);
    await wrapper.get('[data-testid="task-expand-task-long"]').trigger('click');
    expect(wrapper.emitted('toggle-expand')?.[0]).toEqual([longItem]);
    await wrapper.setProps({ expandedTaskIds: ['task-long'] });
    expect(wrapper.findAll('[data-testid^="task-detail-"]')).toHaveLength(1);
    expect(wrapper.get('[data-testid="task-detail-task-long"]').text()).toContain(longItem.current_step_summary);
  });

  it('uses workspace-specific empty guidance and keeps terminal tasks on the progress view', () => {
    const empty = mountBoard({
      emptyTitle: '先选择会话',
      emptyHint: '前往会话历史选择会话',
      createLabel: '选择会话',
    });
    expect(empty.get('[data-testid="task-board-empty"]').text()).toContain('先选择会话');
    expect(empty.get('[data-testid="task-board-create"]').text()).toContain('选择会话');

    const terminal = mountBoard({
      items: [{ ...longItem, task_id: 'task-done', status: 'completed' }],
      total: 1,
      expandable: true,
      expandedTaskIds: [],
    });
    expect(terminal.get('[data-task-id="task-done"]').text()).toContain('已完成');
    expect(terminal.find('[data-testid="task-board-empty"]').exists()).toBe(false);
  });
});
