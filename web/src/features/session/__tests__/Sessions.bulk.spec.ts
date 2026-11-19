// 会话列表批量操作条交互合同：选择 → 破坏性动作二次确认 → 逐项调用单条接口 → 失败就地展示/重试 → 清空。
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { flushPromises, mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import { defineComponent, h, ref } from 'vue';
import BulkActionBar from '../../../components/common/BulkActionBar.vue';
import { useSessionsBulk } from '../useSessionsBulk';
import type { Session } from '../types';

const api = vi.hoisted(() => ({
  archiveSession: vi.fn(),
  restoreSession: vi.fn(),
  deleteSession: vi.fn(),
}));

vi.mock('../api', () => api);

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
        'aria-busy': props.loading ? 'true' : undefined,
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
  { id: 's1', title: '会话一' },
  { id: 's2', title: '会话二' },
] as unknown as Session[];

const Harness = defineComponent({
  setup() {
    const data = ref<Session[]>(rows);
    const bulk = useSessionsBulk(data, async () => {});
    return () =>
      h('div', [
        ...data.value.map((row) =>
          h('input', {
            type: 'checkbox',
            'data-test': `row-select-${row.id}`,
            checked: bulk.isSelected(row.id),
            onChange: (event: Event) =>
              bulk.toggleRow(row.id, (event.target as HTMLInputElement).checked),
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

describe('Sessions 批量操作条', () => {
  it('勾选一行后出现批量条并显示已选数量', async () => {
    const wrapper = mountHarness();
    expect(wrapper.find('[data-test="bulk-bar"]').exists()).toBe(false);

    await wrapper.get('[data-test="row-select-s1"]').setValue(true);

    const bar = wrapper.get('[data-test="bulk-bar"]');
    expect(bar.text()).toContain('1');
    expect(wrapper.find('[data-test="bulk-action-delete"]').exists()).toBe(true);
  });

  it('删除为破坏性动作：确认前不调用接口，确认后逐项调用', async () => {
    const wrapper = mountHarness();
    await wrapper.get('[data-test="row-select-s1"]').setValue(true);

    await wrapper.get('[data-test="bulk-action-delete"]').trigger('click');

    expect(wrapper.find('[data-test="bulk-bar-confirm"]').exists()).toBe(true);
    expect(api.deleteSession).not.toHaveBeenCalled();

    await wrapper.get('[data-test="bulk-confirm"]').trigger('click');
    await flushPromises();

    expect(api.deleteSession).toHaveBeenCalledTimes(1);
    expect(api.deleteSession).toHaveBeenCalledWith('s1');
  });

  it('接口失败时在条内展示 role=alert 并提供重试', async () => {
    api.deleteSession.mockRejectedValueOnce(new Error('会话正在运行，无法删除'));
    const wrapper = mountHarness();
    await wrapper.get('[data-test="row-select-s1"]').setValue(true);

    await wrapper.get('[data-test="bulk-action-delete"]').trigger('click');
    await wrapper.get('[data-test="bulk-confirm"]').trigger('click');
    await flushPromises();

    const error = wrapper.get('[data-test="bulk-bar-error"]');
    expect(error.attributes('role')).toBe('alert');
    expect(error.text()).toContain('会话正在运行，无法删除');
    expect(wrapper.find('[data-test="bulk-retry"]').exists()).toBe(true);

    await wrapper.get('[data-test="bulk-retry"]').trigger('click');
    await flushPromises();
    expect(api.deleteSession).toHaveBeenCalledTimes(2);
  });

  it('执行中 busy 置为动作键：按钮 loading、其它动作禁用', async () => {
    let finish: (() => void) | undefined;
    api.archiveSession.mockImplementationOnce(
      () =>
        new Promise<void>((resolve) => {
          finish = resolve;
        }),
    );
    const wrapper = mountHarness();
    await wrapper.get('[data-test="row-select-s1"]').setValue(true);

    await wrapper.get('[data-test="bulk-action-archive"]').trigger('click');
    await flushPromises();

    expect(wrapper.get('[data-test="bulk-bar"]').attributes('aria-busy')).toBe('true');
    expect(wrapper.getComponent('[data-test="bulk-action-archive"]').props('loading')).toBe(true);
    expect(wrapper.getComponent('[data-test="bulk-action-delete"]').props('disable')).toBe(true);

    finish?.();
    await flushPromises();
  });

  it('点击清空后批量条消失且选择被清空', async () => {
    const wrapper = mountHarness();
    await wrapper.get('[data-test="row-select-s1"]').setValue(true);
    expect(wrapper.find('[data-test="bulk-bar"]').exists()).toBe(true);

    await wrapper.get('[data-test="bulk-clear"]').trigger('click');

    expect(wrapper.find('[data-test="bulk-bar"]').exists()).toBe(false);
    expect((wrapper.get('[data-test="row-select-s1"]').element as HTMLInputElement).checked).toBe(false);
  });
});
