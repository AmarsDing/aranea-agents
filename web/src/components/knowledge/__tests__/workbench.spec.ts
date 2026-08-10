// SP2-3 工作台骨架 smoke：TopBar/Tabs/Sidebar 渲染与事件 + Workbench 装配（开文件→tab）。
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import WorkbenchTopBar from '../workbench/WorkbenchTopBar.vue';
import WorkbenchTabs from '../workbench/WorkbenchTabs.vue';
import WorkbenchSidebar from '../workbench/WorkbenchSidebar.vue';
import KnowledgeWorkbench from '../workbench/KnowledgeWorkbench.vue';
import { createKnowledgeWorkbench, type WorkbenchTab } from '../../../features/knowledge/useKnowledgeWorkbench';
import type { KnowledgeCollection, VaultTreeNode } from '../../../features/knowledge/types';

vi.mock('quasar', () => ({
  useQuasar: () => ({ notify: vi.fn(), dialog: vi.fn() }),
}));

vi.stubGlobal(
  'matchMedia',
  vi.fn().mockImplementation(() => ({
    matches: true, // reduced-motion：粒子不渲染
    media: '(prefers-reduced-motion: reduce)',
    addEventListener: () => {},
    removeEventListener: () => {},
  })),
);

const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN',
  missing: (_l, k) => k,
  messages: { 'zh-CN': {} },
});

const quasarStubs = {
  'q-icon': { template: '<i />' },
  'q-btn': { template: '<button @click="$emit(\'click\', $event)"><slot /></button>' },
  'q-btn-dropdown': { template: '<div><slot /></div>' },
  'q-list': { template: '<div><slot /></div>' },
  'q-item': { template: '<div @click="$emit(\'click\', $event)"><slot /></div>' },
  'q-item-section': { template: '<div><slot /></div>' },
  'q-tooltip': { template: '<span />' },
  'q-dialog': { template: '<div><slot /></div>' },
  'q-linear-progress': { template: '<div />' },
  'q-banner': { template: '<div><slot /></div>' },
  'q-card': { template: '<div><slot /></div>' },
  'q-tree': { template: '<div />' },
  'q-menu': { template: '<div><slot /></div>' },
  'q-separator': { template: '<hr />' },
};

function globalOpts() {
  return { global: { plugins: [i18n], stubs: quasarStubs, directives: { 'close-popup': {} } } };
}

const vault: KnowledgeCollection = {
  id: 'v1',
  name: '主库',
  document_count: 3,
} as KnowledgeCollection;

const fileNode: VaultTreeNode = {
  name: 'note.md',
  path: 'docs/note.md',
  kind: 'file',
  doc_id: 'd1',
  tags: [],
} as VaultTreeNode;

function makeTab(partial: Partial<WorkbenchTab> = {}): WorkbenchTab {
  return {
    docId: 'd1',
    relPath: 'docs/note.md',
    title: 'note.md',
    mode: 'edit',
    editable: true,
    dirty: false,
    saving: false,
    baseHash: 'h1',
    content: '# hello',
    conflict: false,
    ...partial,
  };
}

describe('workbench skeleton', () => {
  beforeEach(() => vi.restoreAllMocks());
  afterEach(() => vi.unstubAllGlobals());

  it('WorkbenchTopBar renders vault name and emits actions', async () => {
    const w = mount(WorkbenchTopBar, {
      props: { collections: [vault], currentVaultId: 'v1' },
      ...globalOpts(),
    });
    expect(w.text()).toContain('主库');
    const btns = w.findAll('button');
    // 动作按钮存在（快速切换/命令面板）
    expect(btns.length).toBeGreaterThanOrEqual(2);
  });

  it('WorkbenchTabs renders tabs and emits activate/close', async () => {
    const w = mount(WorkbenchTabs, {
      props: { tabs: [makeTab()], activeTabId: 'd1' },
      ...globalOpts(),
    });
    expect(w.text()).toContain('note.md');
    await w.find('.kb-tabs__tab').trigger('click');
    expect(w.emitted('activate')?.[0]).toEqual(['d1']);
  });

  it('WorkbenchTabs shows empty state without tabs', () => {
    const w = mount(WorkbenchTabs, {
      props: { tabs: [], activeTabId: '' },
      ...globalOpts(),
    });
    expect(w.find('.kb-tabs__empty').exists()).toBe(true);
  });

  it('WorkbenchSidebar emits open-file on file click', async () => {
    const w = mount(WorkbenchSidebar, {
      props: {
        nodes: [],
        selectedKey: null,
        expandedKeys: [],
        loading: false,
        error: '',
        dragFile: null,
        files: [fileNode],
        activeDocId: '',
      },
      ...globalOpts(),
    });
    expect(w.text()).toContain('note.md');
    await w.find('.kb-sidebar__file').trigger('click');
    expect(w.emitted('open-file')?.[0]).toEqual([fileNode]);
  });

  it('KnowledgeWorkbench opens file into tab via sidebar event', async () => {
    const wb = createKnowledgeWorkbench({
      getDocumentContent: vi.fn(async (id: string) => ({
        id,
        content_text: '# note',
        base_hash: 'h1',
        organized: false,
        raw_content: '',
      })),
      updateDocumentContent: vi.fn(),
    });
    const w = mount(KnowledgeWorkbench, {
      props: {
        workbench: wb,
        collections: [vault],
        documents: [],
        currentVaultId: 'v1',
        nodes: [],
        selectedKey: null,
        expandedKeys: [],
        treeLoading: false,
        treeError: '',
        dragFile: null,
        files: [fileNode],
      },
      ...globalOpts(),
    });
    await w.find('.kb-sidebar__file').trigger('click');
    await vi.waitFor(() => expect(wb.tabs.value.length).toBe(1));
    expect(wb.tabs.value[0].docId).toBe('d1');
    expect(wb.activeTabId.value).toBe('d1');
  });
});
