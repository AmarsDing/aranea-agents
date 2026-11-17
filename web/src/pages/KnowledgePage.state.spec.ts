import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import { uiShellZh } from '../i18n/locales/ui-shell';
import { resolvePageState, type PageStateVariant } from './pageState';

// KnowledgePage 直接挂载成本过高（KnowledgeWorkbench + 3D 图谱 + 上传/分享/移动浮层 + graph.delta WS），
// 因此这里断言它模板真正消费的「状态判定」纯函数，并用真实的 AppStatePanel 渲染同一判定结果：
// 只要判定为 error，模板的 v-if 链就只会命中错误面板，结构上不可能再出现 empty 面板。
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

/** 复刻页面模板：只渲染判定出的那一态。 */
function mountResolvedState(state: PageStateVariant | null) {
  return mount(AppStatePanel, {
    props: state ? { variant: state } : { variant: 'empty' },
    global: { plugins: [i18n], stubs },
  });
}

describe('KnowledgePage 五态：加载失败绝不出现空态', () => {
  it('首屏加载失败且无知识库：只渲染 error 面板', () => {
    const state = resolvePageState({ loading: false, error: 'boom', count: 0 });
    expect(state).toBe('error');

    const wrapper = mountResolvedState(state);
    expect(wrapper.find('[data-state="error"]').exists()).toBe(true);
    expect(wrapper.find('[data-state="empty"]').exists()).toBe(false);
    // 「错误 + 空态并存」不可能发生：只存在一个状态面板。
    expect(wrapper.findAll('[data-test="app-state-panel"]')).toHaveLength(1);
  });

  it('刷新失败但已有知识库：降级为 partial，不清空工作台', () => {
    const state = resolvePageState({ loading: false, error: 'boom', count: 2 });
    expect(state).toBe('partial');
    const wrapper = mountResolvedState(state);
    expect(wrapper.find('[data-state="partial"]').exists()).toBe(true);
    expect(wrapper.find('[data-state="empty"]').exists()).toBe(false);
  });

  it('加载中且无数据：渲染 loading 骨架', () => {
    const state = resolvePageState({ loading: true, error: '', count: 0 });
    expect(state).toBe('loading');
    const wrapper = mountResolvedState(state);
    expect(wrapper.find('[data-state="loading"]').exists()).toBe(true);
    expect(wrapper.find('[data-state="empty"]').exists()).toBe(false);
  });

  it('403：渲染 forbidden，且不带重试按钮', () => {
    const state = resolvePageState({ loading: false, error: '', count: 0, forbidden: true });
    expect(state).toBe('forbidden');
    const wrapper = mountResolvedState(state);
    expect(wrapper.find('[data-state="forbidden"]').exists()).toBe(true);
    expect(wrapper.find('[data-state="empty"]').exists()).toBe(false);
    expect(wrapper.find('button').exists()).toBe(false);
  });

  it('成功且无数据：才允许出现 empty', () => {
    const state = resolvePageState({ loading: false, error: '', count: 0 });
    expect(state).toBe('empty');
    const wrapper = mountResolvedState(state);
    expect(wrapper.find('[data-state="empty"]').exists()).toBe(true);
    expect(wrapper.find('[data-state="error"]').exists()).toBe(false);
  });
});
