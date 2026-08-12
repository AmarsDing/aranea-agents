// FocusCard.spec：M4 聚焦节点信息卡契约（渲染/操作 emit/收起/关闭）。
import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import FocusCard from '../FocusCard.vue';
import zhCN from '../../../../i18n/locales/zh-CN';

const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } });

const quasarStubs = {
  'q-icon': { template: '<i />' },
  'q-btn': {
    props: ['disable'],
    emits: ['click'],
    template: '<button :disabled="disable" @click="$emit(\'click\', $event)"><slot /></button>',
  },
};

const node = {
  docId: 'd1',
  name: '架构设计',
  relPath: 'docs/arch.md',
  docType: 'note',
  degree: 7,
};

function mountCard(props: { node: typeof node; canReembed: boolean }) {
  return mount(FocusCard, { props, global: { plugins: [i18n], stubs: quasarStubs } });
}

describe('FocusCard（M4）', () => {
  it('渲染节点信息：标题/doc_type/度数/路径', () => {
    const w = mountCard({ node, canReembed: true });
    expect(w.text()).toContain('架构设计');
    expect(w.text()).toContain('docs/arch.md');
    expect(w.text()).toContain('7');
  });

  it('「在编辑器打开」emit open-in-explorer', async () => {
    const w = mountCard({ node, canReembed: true });
    await w.find('[data-test="focus-open"]').trigger('click');
    expect(w.emitted('open-in-explorer')).toEqual([[{ docId: 'd1', relPath: 'docs/arch.md' }]]);
  });

  it('「重新向量化」emit reembed（B1 入口②）；canReembed=false 时禁用', async () => {
    const w = mountCard({ node, canReembed: true });
    await w.find('[data-test="focus-reembed"]').trigger('click');
    expect(w.emitted('reembed')).toEqual([['d1']]);
    const w2 = mountCard({ node, canReembed: false });
    expect(w2.find('[data-test="focus-reembed"]').attributes('disabled')).toBeDefined();
  });

  it('收起态只显示标题条', async () => {
    const w = mountCard({ node, canReembed: true });
    await w.find('[data-test="focus-collapse"]').trigger('click');
    expect(w.find('[data-test="focus-body"]').exists()).toBe(false);
    expect(w.text()).toContain('架构设计');
  });

  it('关闭 emit close', async () => {
    const w = mountCard({ node, canReembed: true });
    await w.find('[data-test="focus-close"]').trigger('click');
    expect(w.emitted('close')).toBeTruthy();
  });
});
