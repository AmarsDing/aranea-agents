import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import { uiShellZh } from '../i18n/locales/ui-shell';
import { resolvePageState, type PageStateVariant } from './pageState';

// EvaluationPage 的模板依赖 14 个子组件与 evaluation store，直接挂载需要大量桩件且无法在本轮离线验证；
// 因此断言其模板真正消费的「状态判定」纯函数，并用真实 AppStatePanel 渲染同一结果：
// error 与 empty 由判定顺序保证互斥，数据集为空时不可能同时出现「暂无」文案。
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

function mountResolvedState(state: PageStateVariant | null) {
  return mount(AppStatePanel, {
    props: state ? { variant: state } : { variant: 'empty' },
    global: { plugins: [i18n], stubs },
  });
}

describe('EvaluationPage 五态：加载失败绝不出现空态', () => {
  it('数据集加载失败且为空：只渲染 error 面板', () => {
    const state = resolvePageState({ loading: false, error: 'boom', count: 0 });
    expect(state).toBe('error');

    const wrapper = mountResolvedState(state);
    expect(wrapper.find('[data-state="error"]').exists()).toBe(true);
    expect(wrapper.find('[data-state="empty"]').exists()).toBe(false);
    expect(wrapper.findAll('[data-test="app-state-panel"]')).toHaveLength(1);
  });

  it('已有数据集但刷新失败：partial 横幅 + 保留列表', () => {
    const state = resolvePageState({ loading: false, error: 'boom', count: 3 });
    expect(state).toBe('partial');
    const wrapper = mountResolvedState(state);
    expect(wrapper.find('[data-state="partial"]').exists()).toBe(true);
    expect(wrapper.find('[data-state="empty"]').exists()).toBe(false);
  });

  it('首屏加载中：loading 骨架而非空态', () => {
    const state = resolvePageState({ loading: true, error: '', count: 0 });
    expect(state).toBe('loading');
    const wrapper = mountResolvedState(state);
    expect(wrapper.find('[data-state="loading"]').exists()).toBe(true);
    expect(wrapper.find('[data-state="empty"]').exists()).toBe(false);
  });

  it('403：forbidden 且无重试', () => {
    const state = resolvePageState({ loading: false, error: 'boom', count: 0, forbidden: true });
    expect(state).toBe('forbidden');
    const wrapper = mountResolvedState(state);
    expect(wrapper.find('[data-state="forbidden"]').exists()).toBe(true);
    expect(wrapper.find('[data-state="empty"]').exists()).toBe(false);
    expect(wrapper.find('button').exists()).toBe(false);
  });

  it('成功且无数据集：才允许出现 empty', () => {
    const state = resolvePageState({ loading: false, error: '', count: 0 });
    expect(state).toBe('empty');
    const wrapper = mountResolvedState(state);
    expect(wrapper.find('[data-state="empty"]').exists()).toBe(true);
    expect(wrapper.find('[data-state="error"]').exists()).toBe(false);
  });
});
