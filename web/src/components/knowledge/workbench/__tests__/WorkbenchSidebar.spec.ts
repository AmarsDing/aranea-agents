// B1 文档重嵌入入口①：WorkbenchSidebar 文件行菜单「重新向量化」（T4 菜单项 + emit；T5 词法库置灰）。
import { describe, expect, it, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import WorkbenchSidebar from '../WorkbenchSidebar.vue';
import type { KnowledgeCollection, VaultTreeNode } from '../../../../features/knowledge/types';

vi.mock('quasar', () => ({
  useQuasar: () => ({ notify: vi.fn(), dialog: vi.fn() }),
}));

const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN',
  missing: (_l, k) => k,
  messages: { 'zh-CN': {} },
});

const quasarStubs = {
  'q-icon': { template: '<i />' },
  'q-btn': { template: '<button @click="$emit(\'click\', $event)"><slot /></button>' },
  'q-list': { template: '<div><slot /></div>' },
  // disable 语义落到 data-disabled，且禁用时抑制 click（与 Quasar QItem 行为一致）。
  'q-item': {
    props: ['disable', 'clickable'],
    emits: ['click'],
    template:
      '<div :data-disabled="disable ? \'true\' : undefined" @click="!disable && $emit(\'click\', $event)"><slot /></div>',
  },
  'q-item-section': { template: '<div><slot /></div>' },
  'q-tooltip': { template: '<span><slot /></span>' },
  'q-menu': { template: '<div><slot /></div>' },
  'q-separator': { template: '<hr />' },
  'q-tree': { template: '<div />' },
};

function globalOpts() {
  return { global: { plugins: [i18n], stubs: quasarStubs, directives: { 'close-popup': {} } } };
}

const semanticVault: KnowledgeCollection = {
  id: 'v1',
  name: '语义库',
  embedding_model: 'text-embedding-3-small',
} as KnowledgeCollection;

const lexicalVault: KnowledgeCollection = {
  id: 'v1',
  name: '词法库',
  embedding_model: '',
} as KnowledgeCollection;

const fileNode: VaultTreeNode = {
  name: 'note.md',
  path: 'docs/note.md',
  kind: 'file',
  doc_id: 'd1',
  tags: [],
} as unknown as VaultTreeNode;

function mountSidebar(vault: KnowledgeCollection) {
  return mount(WorkbenchSidebar, {
    props: {
      nodes: [],
      selectedKey: null,
      expandedKeys: [],
      loading: false,
      error: '',
      dragFile: null,
      files: [fileNode],
      activeDocId: '',
      currentVaultId: vault.id,
      currentPrefix: 'docs/',
      collections: [vault],
    },
    ...globalOpts(),
  });
}

describe('WorkbenchSidebar 重嵌入入口（B1）', () => {
  it('文件行菜单含「重新向量化」项并发射 file-action reembed', async () => {
    const w = mountSidebar(semanticVault);
    const item = w.find('[data-test="file-reembed"]');
    expect(item.exists()).toBe(true);
    await item.trigger('click');
    expect(w.emitted('file-action')?.[0]).toEqual(['reembed', fileNode]);
  });

  it('词法库（embedding_model 空）时「重新向量化」菜单项置灰且不发射', async () => {
    const w = mountSidebar(lexicalVault);
    const item = w.find('[data-test="file-reembed"]');
    expect(item.exists()).toBe(true);
    expect(item.attributes('data-disabled')).toBe('true');
    await item.trigger('click');
    expect(w.emitted('file-action')).toBeUndefined();
  });

  it('文件行菜单含仅自己可见 / 库内可见并发射 file-action', async () => {
    const w = mountSidebar(semanticVault);
    await w.find('[data-test="file-private"]').trigger('click');
    await w.find('[data-test="file-collection"]').trigger('click');
    expect(w.emitted('file-action')).toEqual([
      ['private', fileNode],
      ['collection', fileNode],
    ]);
  });
});
