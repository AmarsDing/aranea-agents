import { beforeEach, describe, expect, it, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import PluginRunsPage from '../../../pages/PluginRunsPage.vue';
import AppStatePanel from '../../../components/common/AppStatePanel.vue';
import zhCN from '../../../i18n/locales/zh-CN';
import { uiShellZh } from '../../../i18n/locales/ui-shell';

const loadRows = vi.fn();

/** 每个用例挂载前写入：组合式对外暴露的「加载失败且无数据」等状态。 */
const state = { loading: false, error: 'boom', forbidden: false, rows: [] as unknown[] };

vi.mock('quasar', () => ({
  Dialog: { create: vi.fn() },
}));

vi.mock('../usePluginRunsPage', async () => {
  const { computed, reactive, ref } = await import('vue');
  const rows = ref<unknown[]>([]);
  const loading = ref(false);
  const error = ref('');
  const forbidden = ref(false);
  return {
    usePluginRunsPage: () => ({
      pluginKey: ref(''),
      agentId: ref(''),
      callbackPoint: ref(''),
      status: ref(''),
      from: ref(''),
      to: ref(''),
      rows,
      loading,
      error,
      forbidden,
      total: ref(0),
      page: ref(1),
      pageSize: ref(20),
      pageMax: computed(() => 1),
      detailOpen: ref(false),
      detailText: ref(''),
      clearing: ref(false),
      callbackPointOptions: reactive([]),
      statusOptions: reactive([]),
      pluginKeyOptions: reactive([]),
      columns: reactive([]),
      filterPluginKeys: vi.fn(),
      loadRows,
      onFilterChange: vi.fn(),
      resetFilters: vi.fn(),
      openDetail: vi.fn(),
      detailPreview: () => '',
      clearAll: vi.fn(),
    }),
    __applyState: (next: typeof state) => {
      rows.value = next.rows;
      loading.value = next.loading;
      error.value = next.error;
      forbidden.value = next.forbidden;
    },
  };
});

const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN',
  messages: { 'zh-CN': { ...zhCN, shell: uiShellZh } },
});

const stubs = {
  'q-page': { template: '<div><slot /></div>' },
  'q-btn': { props: ['label'], template: '<button type="button">{{ label }}</button>' },
  'q-select': true,
  'q-input': true,
  AppPageHero: { template: '<div><slot /></div>' },
  AppPageToolbar: { template: '<div><slot /></div>' },
  AppRegistryTable: { template: '<div data-test="registry-table" />' },
  AppRegistryHoverTip: { template: '<div><slot /></div>' },
  AppRegistryPagination: true,
  PluginRunDetailDialog: true,
  AppStatusChip: true,
};

async function mountPage(next: Partial<typeof state>) {
  Object.assign(state, next);
  const { __applyState } = (await import('../usePluginRunsPage')) as unknown as {
    __applyState: (s: typeof state) => void;
  };
  __applyState(state);
  return mount(PluginRunsPage, { global: { plugins: [i18n], stubs: { ...stubs, AppStatePanel } } });
}

beforeEach(() => {
  loadRows.mockClear();
});

describe('PluginRunsPage 五态契约', () => {
  it('加载失败且列表为空时只渲染 error，不渲染空态/部分失败', async () => {
    const wrapper = await mountPage({ loading: false, error: 'boom', forbidden: false, rows: [] });

    expect(wrapper.find('[data-state="error"]').exists()).toBe(true);
    expect(wrapper.find('[data-state="empty"]').exists()).toBe(false);
    expect(wrapper.find('[data-state="partial"]').exists()).toBe(false);
    expect(wrapper.find('[data-test="registry-table"]').exists()).toBe(false);
  });

  it('error 面板透出失败原因并带重试入口，点击重新拉取', async () => {
    const wrapper = await mountPage({ loading: false, error: 'boom', forbidden: false, rows: [] });
    const panel = wrapper.find('[data-state="error"]');

    expect(panel.text()).toContain('boom');
    const retry = panel.findAll('button').find((button) => button.text().includes('重试'));
    expect(retry).toBeTruthy();
    await retry?.trigger('click');
    expect(loadRows).toHaveBeenCalled();
  });

  it('成功且无运行记录时渲染空态', async () => {
    const wrapper = await mountPage({ loading: false, error: '', forbidden: false, rows: [] });

    expect(wrapper.find('[data-state="empty"]').exists()).toBe(true);
    expect(wrapper.find('[data-state="error"]').exists()).toBe(false);
  });

  it('已有记录但刷新失败时保留列表并加 partial 横幅', async () => {
    const wrapper = await mountPage({ loading: false, error: 'boom', forbidden: false, rows: [{ id: 'r1' }] });

    expect(wrapper.find('[data-state="partial"]').exists()).toBe(true);
    expect(wrapper.find('[data-state="error"]').exists()).toBe(false);
    expect(wrapper.find('[data-state="empty"]').exists()).toBe(false);
    expect(wrapper.find('[data-test="registry-table"]').exists()).toBe(true);
  });

  it('403 只渲染 forbidden，不提供重试', async () => {
    const wrapper = await mountPage({ loading: false, error: 'boom', forbidden: true, rows: [] });
    const panel = wrapper.find('[data-state="forbidden"]');

    expect(panel.exists()).toBe(true);
    expect(wrapper.find('[data-state="error"]').exists()).toBe(false);
    expect(panel.findAll('button')).toHaveLength(0);
  });

  it('首屏加载中渲染 loading 且不渲染列表', async () => {
    const wrapper = await mountPage({ loading: true, error: '', forbidden: false, rows: [] });

    expect(wrapper.find('[data-state="loading"]').exists()).toBe(true);
    expect(wrapper.find('[data-test="registry-table"]').exists()).toBe(false);
  });
});
