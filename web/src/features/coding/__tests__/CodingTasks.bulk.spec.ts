// 编码任务列表批量操作条交互合同：选择 → 中止二次确认 → 逐项调用真实接口 → 失败就地展示/重试 → 清空。
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { flushPromises, mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import { defineComponent, h, ref } from 'vue';
import BulkActionBar from '../../../components/common/BulkActionBar.vue';
import { useCodingTasksBulk } from '../useCodingTasksBulk';
import type { CodingTask } from '../api';

const api = vi.hoisted(() => ({
  cancelCodingTask: vi.fn(),
  reviewCodingTask: vi.fn(),
}));

vi.mock('../api', () => ({
  ...api,
  isActiveStatus: (status: string | undefined) =>
    ['dispatched', 'running', 'awaiting_approval', 'cancelling'].includes(status ?? ''),
  isTerminalStatus: (status: string | undefined) => ['done', 'failed', 'cancelled'].includes(status ?? ''),
}));

const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN',
  missing: (_locale, key) => key,
  messages: {
    'zh-CN': {
      shell: {
        common: { confirm: '确认', cancel: '取消', retry: '重试' },
        bulkBar: {
          ariaLabel: '批量操作',
          selected: '已选 {n} 项',
          clear: '取消选择',
          failure: '失败 {failed}/{total} 条：{reason}',
          failed: '批量操作失败',
        },
      },
    },
  },
});

const qBtnStub = defineComponent({
  props: {
    label: { type: String, default: '' },
    icon: { type: String, default: '' },
    disable: Boolean,
    loading: Boolean,
    ariaLabel: { type: String, default: undefined },
  },
  emits: ['click'],
  setup: (props, { attrs, emit, slots }) => () =>
    h(
      'button',
      {
        ...attrs,
        type: 'button',
        disabled: props.disable || props.loading ? true : undefined,
        'aria-label': props.ariaLabel,
        onClick: () => emit('click'),
      },
      [props.label || props.icon, slots.default?.()],
    ),
});

const stubs = {
  'q-btn': qBtnStub,
  'q-icon': defineComponent({ setup: () => () => h('i') }),
  'q-tooltip': defineComponent({ setup: (_props, { slots }) => () => h('span', slots.default?.()) }),
  'q-separator': defineComponent({ setup: () => () => h('hr') }),
};

const rows = [
  { id: 'c1', status: 'running' },
  { id: 'c2', status: 'done' },
] as unknown as CodingTask[];

const Harness = defineComponent({
  setup() {
    const data = ref<CodingTask[]>(rows);
    const bulk = useCodingTasksBulk(data, async () => {});
    return () =>
      h('div', [
        ...data.value.map((row) =>
          h('input', {
            type: 'checkbox',
            'data-test': `row-select-${row.id}`,
            checked: bulk.isSelected(row.id as string),
            onChange: (event: Event) =>
              bulk.toggleRow(row.id as string, (event.target as HTMLInputElement).checked),
          }),
        ),
        bulk.count.value > 0
          ? h(BulkActionBar, {
              count: bulk.count.value,
              total: data.value.length,
              actions: bulk.actions.value,
              busy: bulk.busy.value,
              error: bulk.error.value,
              onRun: bulk.run,
              onClear: bulk.clear,
            })
          : null,
      ]);
  },
});

function mountHarness() {
  return mount(Harness, { global: { plugins: [i18n], stubs } });
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('CodingTasks 批量操作条', () => {
  it('勾选一行后出现批量条并显示已选数量', async () => {
    const wrapper = mountHarness();
    expect(wrapper.find('[data-test="bulk-bar"]').exists()).toBe(false);

    await wrapper.get('[data-test="row-select-c1"]').setValue(true);

    const bar = wrapper.get('[data-test="bulk-bar"]');
    expect(bar.text()).toContain('1');
    expect(wrapper.find('[data-test="bulk-action-cancel"]').exists()).toBe(true);
  });

  it('中止为破坏性动作：确认前不调用接口，确认后逐项调用', async () => {
    const wrapper = mountHarness();
    await wrapper.get('[data-test="row-select-c1"]').setValue(true);

    await wrapper.get('[data-test="bulk-action-cancel"]').trigger('click');

    expect(wrapper.find('[data-test="bulk-bar-confirm"]').exists()).toBe(true);
    expect(api.cancelCodingTask).not.toHaveBeenCalled();

    await wrapper.get('[data-test="bulk-confirm"]').trigger('click');
    await flushPromises();

    expect(api.cancelCodingTask).toHaveBeenCalledTimes(1);
    expect(api.cancelCodingTask).toHaveBeenCalledWith('c1');
  });

  it('接口失败时在条内展示 role=alert 并提供重试', async () => {
    api.cancelCodingTask.mockRejectedValueOnce(new Error('任务已结束，无法中止'));
    const wrapper = mountHarness();
    await wrapper.get('[data-test="row-select-c1"]').setValue(true);

    await wrapper.get('[data-test="bulk-action-cancel"]').trigger('click');
    await wrapper.get('[data-test="bulk-confirm"]').trigger('click');
    await flushPromises();

    const error = wrapper.get('[data-test="bulk-bar-error"]');
    expect(error.attributes('role')).toBe('alert');
    expect(error.text()).toContain('任务已结束，无法中止');
    expect(wrapper.find('[data-test="bulk-retry"]').exists()).toBe(true);

    await wrapper.get('[data-test="bulk-retry"]').trigger('click');
    await flushPromises();
    expect(api.cancelCodingTask).toHaveBeenCalledTimes(2);
  });

  it('归档走真实 ReviewTask 接口', async () => {
    const wrapper = mountHarness();
    await wrapper.get('[data-test="row-select-c2"]').setValue(true);

    await wrapper.get('[data-test="bulk-action-archive"]').trigger('click');
    await flushPromises();

    expect(api.reviewCodingTask.mock.calls).toEqual([['c2', 'archive']]);
  });

  it('点击清空后批量条消失且选择被清空', async () => {
    const wrapper = mountHarness();
    await wrapper.get('[data-test="row-select-c1"]').setValue(true);
    expect(wrapper.find('[data-test="bulk-bar"]').exists()).toBe(true);

    await wrapper.get('[data-test="bulk-clear"]').trigger('click');

    expect(wrapper.find('[data-test="bulk-bar"]').exists()).toBe(false);
    expect((wrapper.get('[data-test="row-select-c1"]').element as HTMLInputElement).checked).toBe(false);
  });
});
