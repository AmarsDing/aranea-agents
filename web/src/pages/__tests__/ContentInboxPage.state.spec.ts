import { describe, expect, it, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import ContentInboxPage from '../ContentInboxPage.vue';
import AppStatePanel from '../../components/common/AppStatePanel.vue';
import zhCN from '../../i18n/locales/zh-CN';
import { uiShellZh } from '../../i18n/locales/ui-shell';

vi.mock('../../stores/mvp', async () => {
  const { ref } = await import('vue');
  return {
    useContentInboxStore: () => ({
      rows: ref([]),
      loading: ref(false),
      saving: ref(false),
      error: ref('boom'),
      feedback: ref(''),
      load: vi.fn(),
      create: vi.fn(),
      ingestRSS: vi.fn(),
      ingestSearch: vi.fn(),
    }),
  };
});

const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN',
  messages: { 'zh-CN': { ...zhCN, shell: uiShellZh } },
});

const stubs = {
  'q-page': { template: '<div><slot /></div>' },
  'q-card': { template: '<div><slot /></div>' },
  'q-card-section': { template: '<div><slot /></div>' },
  'q-form': { template: '<form><slot /></form>' },
  'q-input': true,
  'q-select': true,
  'q-skeleton': { template: '<div />' },
  'q-icon': { template: '<i />' },
  'q-btn': { props: ['label'], template: '<button type="button">{{ label }}</button>' },
  'q-banner': { template: '<div><slot /></div>' },
};

function mountPage() {
  return mount(ContentInboxPage, {
    global: { plugins: [i18n], stubs: { ...stubs, AppStatePanel } },
  });
}

describe('ContentInboxPage 五态契约', () => {
  it('加载失败且列表为空时只渲染 error，不渲染空态', () => {
    const wrapper = mountPage();

    expect(wrapper.find('[data-state="error"]').exists()).toBe(true);
    expect(wrapper.find('[data-state="empty"]').exists()).toBe(false);
    expect(wrapper.find('[data-state="partial"]').exists()).toBe(false);
  });

  it('error 面板带重试入口且透出失败原因', () => {
    const wrapper = mountPage();
    const panel = wrapper.find('[data-state="error"]');

    expect(panel.text()).toContain('boom');
    expect(panel.findAll('button').some((button) => button.text().includes('重试'))).toBe(true);
  });
});
