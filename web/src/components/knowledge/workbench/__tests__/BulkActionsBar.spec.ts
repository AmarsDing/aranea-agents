// U0-4 BulkActionsBar：浮现条件 + 按钮事件 + 移动对话框目标回传。
import { describe, expect, it, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import BulkActionsBar from '../BulkActionsBar.vue';
import KnowledgeMoveDialog from '../../KnowledgeMoveDialog.vue';

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
  // disable 语义落到 disabled 属性，且禁用时抑制 click；emits 声明避免父级 @click
  // 同时经 attrs 透传落地成原生 DOM 监听（双触发）。
  'q-btn': {
    props: ['disable'],
    emits: ['click'],
    template:
      '<button :disabled="disable ? true : undefined" @click="!disable && $emit(\'click\', $event)"><slot /></button>',
  },
  'q-tooltip': { template: '<span><slot /></span>' },
};

const moveOptions = [
  { label: 'B 库', value: 'v2', disable: false, dim: 1536 },
  { label: 'C 库', value: 'v3', disable: true, dim: 768 },
];

function mountBar(count = 3, hasSemantic = true) {
  return mount(BulkActionsBar, {
    props: { count, hasSemantic, moveOptions },
    global: { plugins: [i18n], stubs: quasarStubs, directives: { 'close-popup': {} } },
  });
}

describe('BulkActionsBar（U0-4 批量操作条）', () => {
  it('count > 0 浮现并显示选中数；0 时隐藏', () => {
    const w = mountBar(3);
    expect(w.find('.kb-bulk-bar').exists()).toBe(true);
    expect(w.find('[data-test="bulk-count"]').text()).toContain('selectedCount');
    const w0 = mountBar(0);
    expect(w0.find('.kb-bulk-bar').exists()).toBe(false);
  });

  it('删除 / 重嵌入 / 取消选择按钮分别 emit', async () => {
    const w = mountBar();
    await w.find('[data-test="bulk-delete"]').trigger('click');
    await w.find('[data-test="bulk-reembed"]').trigger('click');
    await w.find('[data-test="bulk-clear"]').trigger('click');
    expect(w.emitted('delete')).toHaveLength(1);
    expect(w.emitted('reembed')).toHaveLength(1);
    expect(w.emitted('clear')).toHaveLength(1);
  });

  it('词法库（hasSemantic=false）重嵌入按钮禁用且点击不 emit', async () => {
    const w = mountBar(2, false);
    const btn = w.find('[data-test="bulk-reembed"]');
    expect(btn.attributes('disabled')).toBeDefined();
    await btn.trigger('click');
    expect(w.emitted('reembed')).toBeUndefined();
  });

  it('移动：打开对话框并经对话框 submit 回传 targetId', async () => {
    const w = mountBar();
    await w.find('[data-test="bulk-move"]').trigger('click');
    const dlg = w.findComponent(KnowledgeMoveDialog);
    expect(dlg.props('open')).toBe(true);
    // v-model:target-id 更新 → submit 回传。
    await dlg.vm.$emit('update:targetId', 'v2');
    await dlg.vm.$emit('submit');
    expect(w.emitted('move')).toEqual([['v2']]);
    // 提交后对话框关闭、目标复位。
    expect(dlg.props('open')).toBe(false);
  });

  it('移动：空目标时 submit 不 emit move', async () => {
    const w = mountBar();
    await w.find('[data-test="bulk-move"]').trigger('click');
    const dlg = w.findComponent(KnowledgeMoveDialog);
    await dlg.vm.$emit('submit');
    expect(w.emitted('move')).toBeUndefined();
  });
});
