import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import AgentsListSection from '../AgentsListSection.vue';
import zhCN from '../../../i18n/locales/zh-CN';
import { uiShellZh } from '../../../i18n/locales/ui-shell';

const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN',
  messages: { 'zh-CN': { ...zhCN, shell: uiShellZh } },
});

const baseProps = {
  loading: false,
  agents: [],
  keyword: '',
  viewMode: 'list',
  rowsPerPage: 20,
  tableColumns: [],
  isFavorite: () => false,
  getCategoryLabel: () => '',
  selectedIds: [],
};

const stubs = {
  'q-card': { template: '<div><slot /></div>' },
  'q-card-section': { template: '<div><slot /></div>' },
  'q-avatar': { template: '<div />' },
  'q-btn': { props: ['label'], template: '<button type="button">{{ label }}</button>' },
  'q-icon': { template: '<i />' },
  'q-skeleton': { template: '<div />' },
};

function mountSection(props: Record<string, unknown>) {
  return mount(AgentsListSection, {
    props: { ...baseProps, ...props },
    global: { plugins: [i18n], stubs },
  });
}

describe('AgentsListSection failure contract', () => {
  it('renders a retryable error state instead of the empty state when the load fails', async () => {
    const wrapper = mountSection({ error: '网络错误：无法加载 Agent 列表' });

    expect(wrapper.text()).toContain('网络错误：无法加载 Agent 列表');
    expect(wrapper.text()).not.toContain('暂无 Agent');

    const retry = wrapper.findAll('button').find((button) => button.text().includes('重试'));
    expect(retry).toBeTruthy();
    await retry!.trigger('click');
    expect(wrapper.emitted('retry')).toHaveLength(1);
  });

  it('keeps the empty state when there is no error', () => {
    const wrapper = mountSection({ error: '' });
    expect(wrapper.text()).toContain('暂无 Agent');
  });
});
