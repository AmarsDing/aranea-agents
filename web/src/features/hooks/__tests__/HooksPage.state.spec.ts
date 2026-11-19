import { beforeEach, describe, expect, it, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import HooksPage from '../../../pages/HooksPage.vue';
import AppStatePanel from '../../../components/common/AppStatePanel.vue';
import zhCN from '../../../i18n/locales/zh-CN';
import { uiShellZh } from '../../../i18n/locales/ui-shell';

const loadRows = vi.fn();

/** 每个用例挂载前写入：组合式对外暴露的「加载失败且无数据」等状态。 */
const state = { loading: false, error: 'boom', forbidden: false, rows: [] as unknown[] };

vi.mock('../useHooksPage', async () => {
  const { computed, reactive, ref } = await import('vue');
  const loading = ref(false);
  const error = ref('');
  const forbidden = ref(false);
  const pagedRows = ref<unknown[]>([]);
  return {
    useHooksPage: () => ({
      t: (key: string) => key,
      loading,
      saving: ref(false),
      error,
      forbidden,
      search: ref(''),
      filterPoint: ref(''),
      callbackPointOptions: reactive([]),
      editorOpen: ref(false),
      editingId: ref(''),
      busyId: ref(''),
      form: reactive({ key: '', name: '', url: '', rule: '', timeout: 0 }),
      formValid: computed(() => false),
      page: ref(1),
      pageSize: ref(20),
      total: ref(0),
      pagedRows,
      pageMax: computed(() => 1),
      resetFilters: vi.fn(),
      loadRows,
      openCreate: vi.fn(),
      openEdit: vi.fn(),
      saveHook: vi.fn(),
      toggleEnabled: vi.fn(),
      confirmDelete: vi.fn(),
    }),
    __applyState: (next: typeof state) => {
      pagedRows.value = next.rows;
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
  'q-input': true,
  'q-select': true,
  'q-toggle': true,
  AppDialog: { template: '<div><slot /></div>' },
  AppPageHero: { template: '<div><slot /></div>' },
  AppPageToolbar: { template: '<div><slot /></div>' },
  AppRegistryPagination: true,
  CallbackEditor: true,
  HooksTable: { template: '<div data-test="hooks-table" />' },
};

async function mountPage(next: Partial<typeof state>) {
  Object.assign(state, next);
  const { __applyState } = (await import('../useHooksPage')) as unknown as {
    __applyState: (s: typeof state) => void;
  };
  __applyState(state);
  return mount(HooksPage, { global: { plugins: [i18n], stubs: { ...stubs, AppStatePanel } } });
}

beforeEach(() => {
  loadRows.mockClear();
});

describe('HooksPage 五态契约', () => {
  it('加载失败且列表为空时只渲染 error，不渲染空态/部分失败', async () => {
    const wrapper = await mountPage({ loading: false, error: 'boom', forbidden: false, rows: [] });

    expect(wrapper.find('[data-state="error"]').exists()).toBe(true);
    expect(wrapper.find('[data-state="empty"]').exists()).toBe(false);
    expect(wrapper.find('[data-state="partial"]').exists()).toBe(false);
    expect(wrapper.find('[data-test="hooks-table"]').exists()).toBe(false);
  });

  it('error 面板带重试入口，点击重新拉取', async () => {
    const wrapper = await mountPage({ loading: false, error: 'boom', forbidden: false, rows: [] });
    const panel = wrapper.find('[data-state="error"]');

    expect(panel.text()).toContain('boom');
    const retry = panel.findAll('button').find((button) => button.text().length > 0);
    expect(retry).toBeTruthy();
    await retry?.trigger('click');
    expect(loadRows).toHaveBeenCalled();
  });

  it('成功且无钩子时渲染空态', async () => {
    const wrapper = await mountPage({ loading: false, error: '', forbidden: false, rows: [] });

    expect(wrapper.find('[data-state="empty"]').exists()).toBe(true);
    expect(wrapper.find('[data-state="error"]').exists()).toBe(false);
  });

  it('已有钩子但刷新失败时保留表格并加 partial 横幅', async () => {
    const wrapper = await mountPage({ loading: false, error: 'boom', forbidden: false, rows: [{ id: 'r1' }] });

    expect(wrapper.find('[data-state="partial"]').exists()).toBe(true);
    expect(wrapper.find('[data-state="error"]').exists()).toBe(false);
    expect(wrapper.find('[data-test="hooks-table"]').exists()).toBe(true);
  });

  it('403 只渲染 forbidden，不提供重试', async () => {
    const wrapper = await mountPage({ loading: false, error: 'boom', forbidden: true, rows: [] });
    const panel = wrapper.find('[data-state="forbidden"]');

    expect(panel.exists()).toBe(true);
    expect(wrapper.find('[data-state="error"]').exists()).toBe(false);
    expect(panel.findAll('button')).toHaveLength(0);
  });
});
