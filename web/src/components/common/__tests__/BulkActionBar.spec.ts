import { afterEach, describe, expect, it } from 'vitest';
import { mount, type VueWrapper } from '@vue/test-utils';
import { nextTick } from 'vue';
import { createI18n } from 'vue-i18n';
import BulkActionBar from '../BulkActionBar.vue';
import type { BulkAction } from '../bulkAction';

const stubs = {
  'q-icon': { template: '<i />' },
  'q-btn': {
    props: ['label', 'disable', 'loading', 'icon', 'ariaLabel'],
    emits: ['click'],
    template:
      '<button type="button" :disabled="disable" :aria-label="ariaLabel" v-bind="$attrs" @click="$emit(\'click\')">{{ label || icon }}</button>',
  },
  'q-tooltip': { template: '<span class="q-tooltip-stub"><slot /></span>' },
  'q-separator': { template: '<hr />' },
};

const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN',
  messages: {
    'zh-CN': {
      shell: {
        common: { confirm: '确认', cancel: '取消', retry: '重试' },
        bulkBar: { ariaLabel: '批量操作', selected: '已选 {n} 项', clear: '取消选择' },
      },
    },
  },
});

const actions: BulkAction[] = [
  { key: 'enable', label: '启用', icon: 'play_circle' },
  { key: 'archive', label: '归档', icon: 'archive', disabled: true, hint: '无权限' },
  {
    key: 'delete',
    label: '删除',
    icon: 'delete',
    tone: 'danger',
    requiresConfirm: true,
    confirmLabel: '确认删除所选？',
  },
];

const wrappers: VueWrapper[] = [];
function bar(props: Record<string, unknown> = {}) {
  const wrapper = mount(BulkActionBar, {
    attachTo: document.body,
    props: { count: 3, total: 10, actions, ...props },
    global: { plugins: [i18n], stubs },
  });
  wrappers.push(wrapper);
  return wrapper;
}

afterEach(() => {
  for (const wrapper of wrappers.splice(0)) wrapper.unmount();
  document.body.innerHTML = '';
});

describe('BulkActionBar', () => {
  it('渲染已选计数（aria-live）与动作按钮，根节点可聚焦且不 busy', () => {
    const wrapper = bar();
    const root = wrapper.get('[data-test="bulk-bar"]');

    expect(wrapper.get('.bulk-bar__count').text()).toContain('已选 3 项');
    expect(wrapper.get('.bulk-bar__count').text()).toContain('/ 10');
    expect(wrapper.get('.bulk-bar__count').attributes('aria-live')).toBe('polite');
    expect(wrapper.find('[data-test="bulk-action-enable"]').exists()).toBe(true);
    expect(root.attributes('aria-busy')).toBe('false');
    expect(root.attributes('tabindex')).toBe('-1');
  });

  it('出现时把焦点交给操作条，保证 Esc 与 Tab 可用', async () => {
    const wrapper = bar();
    await nextTick();

    expect(document.activeElement).toBe(wrapper.get('[data-test="bulk-bar"]').element);
  });

  it('普通动作一次点击即上抛 run，disabled 动作不可触发', async () => {
    const wrapper = bar();

    await wrapper.get('[data-test="bulk-action-enable"]').trigger('click');
    await wrapper.get('[data-test="bulk-action-archive"]').trigger('click');

    expect(wrapper.emitted('run')).toEqual([['enable']]);
  });

  it('requiresConfirm 动作先进入确认态，确认后才上抛 run', async () => {
    const wrapper = bar();

    await wrapper.get('[data-test="bulk-action-delete"]').trigger('click');
    expect(wrapper.emitted('run')).toBeUndefined();
    expect(wrapper.get('[data-test="bulk-bar-confirm"]').text()).toContain('确认删除所选？');

    await wrapper.get('[data-test="bulk-confirm"]').trigger('click');
    expect(wrapper.emitted('run')).toEqual([['delete']]);
    expect(wrapper.find('[data-test="bulk-bar-confirm"]').exists()).toBe(false);
  });

  it('Esc 在确认态只回退确认，再按 Esc 才清除选择', async () => {
    const wrapper = bar();

    await wrapper.get('[data-test="bulk-action-delete"]').trigger('click');
    await wrapper.get('[data-test="bulk-bar"]').trigger('keydown', { key: 'Escape' });
    expect(wrapper.emitted('clear')).toBeUndefined();

    await wrapper.get('[data-test="bulk-bar"]').trigger('keydown', { key: 'Escape' });
    expect(wrapper.emitted('clear')).toEqual([[]]);
  });

  it('busy 时全部按钮禁用、运行项进入 loading，aria-busy 置真', () => {
    const wrapper = bar({ busy: 'enable' });

    expect(wrapper.get('[data-test="bulk-bar"]').attributes('aria-busy')).toBe('true');
    expect(wrapper.get('[data-test="bulk-action-enable"]').attributes('disabled')).toBeDefined();
    expect(wrapper.getComponent('[data-test="bulk-action-enable"]').props('loading')).toBe(true);
    expect(wrapper.getComponent('[data-test="bulk-action-delete"]').props('loading')).toBe(false);
  });

  it('error 以 role=alert 就地展示并可重试上一次动作', async () => {
    const wrapper = bar({ error: '批量删除失败：3 条被占用' });

    expect(wrapper.get('[data-test="bulk-bar-error"]').attributes('role')).toBe('alert');
    expect(wrapper.get('[data-test="bulk-bar-error"]').text()).toContain('批量删除失败');

    await wrapper.get('[data-test="bulk-action-enable"]').trigger('click');
    await wrapper.get('[data-test="bulk-retry"]').trigger('click');
    expect(wrapper.emitted('run')).toEqual([['enable'], ['enable']]);
  });

  it('清除按钮上抛 clear', async () => {
    const wrapper = bar();

    await wrapper.get('[data-test="bulk-clear"]').trigger('click');
    expect(wrapper.emitted('clear')).toEqual([[]]);
  });
});
