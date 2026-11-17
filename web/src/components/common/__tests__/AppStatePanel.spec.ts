import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import AppStatePanel from '../AppStatePanel.vue';
import { uiShellZh } from '../../../i18n/locales/ui-shell';

const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN',
  messages: { 'zh-CN': { shell: uiShellZh } },
});

const stubs = {
  'q-skeleton': { template: '<div class="sk" />' },
  'q-icon': { template: '<i />' },
  'q-btn': { props: ['label'], template: '<button type="button">{{ label }}</button>' },
};

function mountPanel(props: Record<string, unknown>) {
  return mount(AppStatePanel, { props, global: { plugins: [i18n], stubs } });
}

describe('AppStatePanel state contract', () => {
  it('distinguishes loading / empty / partial / error / forbidden', () => {
    const loading = mountPanel({ variant: 'loading', rows: 2 });
    expect(loading.attributes('role')).toBe('status');
    expect(loading.attributes('aria-busy')).toBe('true');
    expect(loading.findAll('.sk')).toHaveLength(2);

    const empty = mountPanel({ variant: 'empty' });
    expect(empty.attributes('role')).toBe('status');
    expect(empty.find('.app-state-panel__title').text()).toBe('暂无数据');

    const partial = mountPanel({ variant: 'partial' });
    expect(partial.attributes('role')).toBe('alert');
    expect(partial.attributes('aria-live')).toBe('assertive');
    expect(partial.find('.app-state-panel__title').text()).toBe('部分内容加载失败');
    expect(partial.find('.app-state-panel__desc').text()).toContain('可单独重试');

    const error = mountPanel({ variant: 'error' });
    expect(error.find('.app-state-panel__title').text()).toBe('加载失败');

    const forbidden = mountPanel({ variant: 'forbidden' });
    expect(forbidden.find('.app-state-panel__title').text()).toBe('无权访问');
  });

  it('renders a retry action that emits action', async () => {
    const wrapper = mountPanel({ variant: 'partial', actionLabel: '重试' });
    const button = wrapper.find('button');
    expect(button.text()).toBe('重试');
    await button.trigger('click');
    expect(wrapper.emitted('action')).toHaveLength(1);
  });

  it('lets callers override the copy for a concrete failure', () => {
    const wrapper = mountPanel({
      variant: 'partial',
      title: 'Judge 一致性校准加载失败',
      description: 'upstream 502',
    });
    expect(wrapper.find('.app-state-panel__title').text()).toBe('Judge 一致性校准加载失败');
    expect(wrapper.find('.app-state-panel__desc').text()).toBe('upstream 502');
  });
});
