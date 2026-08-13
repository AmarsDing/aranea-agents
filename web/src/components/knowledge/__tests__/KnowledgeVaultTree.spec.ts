// B2-T2：vault 根菜单「启用语义检索」——仅词法库（lexicalVaultIds 命中）显示。
import { describe, expect, it, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import KnowledgeVaultTree from '../KnowledgeVaultTree.vue';
import type { VaultQTreeNode } from '../../../features/knowledge/useVaultExplorer';

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
  'q-btn': { template: '<button><slot /></button>' },
  'q-banner': { template: '<div><slot /></div>' },
  'q-card': { template: '<div><slot /></div>' },
  'q-list': { template: '<div><slot /></div>' },
  // q-item 声明 emits:['click']：避免父级 @click 既作 fallthrough 原生监听又被 $emit('click') 触发导致双击
  'q-item': { emits: ['click'], template: '<div @click="$emit(\'click\', $event)"><slot /></div>' },
  'q-item-section': { template: '<div><slot /></div>' },
  'q-tooltip': { template: '<span />' },
  'q-menu': { template: '<div><slot /></div>' },
  'q-separator': { template: '<hr />' },
  'q-linear-progress': { template: '<div />' },
  // q-tree _stub：直接以 default-header 插槽渲染一级节点（测试菜单可见性）。_
  'q-tree': {
    props: ['nodes'],
    template: '<div><div v-for="n in nodes" :key="n.key"><slot name="default-header" :node="n" /></div></div>',
  },
};

function globalOpts() {
  return { global: { plugins: [i18n], stubs: quasarStubs, directives: { 'close-popup': {} } } };
}

function vaultNode(vaultId: string): VaultQTreeNode {
  return { label: vaultId, key: `vault:${vaultId}`, icon: 'inventory_2', kind: 'vault', vaultId, prefix: '' };
}

function mountTree(nodes: VaultQTreeNode[], lexicalVaultIds: string[]) {
  return mount(KnowledgeVaultTree, {
    props: {
      nodes,
      selectedKey: null,
      expandedKeys: [],
      loading: false,
      error: '',
      dragFile: null,
      lexicalVaultIds,
    },
    ...globalOpts(),
  });
}

describe('KnowledgeVaultTree 启用语义检索菜单（B2-T2）', () => {
  it('词法库 vault 根菜单显示「启用语义检索」，语义库不显示', () => {
    const w = mountTree([vaultNode('v-lex'), vaultNode('v-sem')], ['v-lex']);
    const items = w.findAll('[data-test="vault-enable-semantic"]');
    expect(items).toHaveLength(1);
    expect(w.text()).toContain('knowledgePage.enableSemantic');
  });

  it('点击菜单项 emit node-action enable-semantic 并携带节点', async () => {
    const node = vaultNode('v-lex');
    const w = mountTree([node], ['v-lex']);
    await w.find('[data-test="vault-enable-semantic"]').trigger('click');
    expect(w.emitted('node-action')).toEqual([['enable-semantic', node]]);
  });

  it('lexicalVaultIds 为空时任何 vault 都不显示菜单项', () => {
    const w = mountTree([vaultNode('v-lex')], []);
    expect(w.find('[data-test="vault-enable-semantic"]').exists()).toBe(false);
  });
});
