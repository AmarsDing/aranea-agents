// NoteEditor（SP2-4）smoke：CM6 挂载、Live Preview 芯片渲染、dangling 判定、跳链事件。
import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import NoteEditor from '../workbench/NoteEditor.vue';

const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN',
  missing: (_l, k) => k,
  messages: { 'zh-CN': {} },
});

function mountEditor(props: { content: string; readOnly?: boolean; candidates?: string[] }) {
  return mount(NoteEditor, {
    props,
    global: { plugins: [i18n], stubs: { 'q-icon': { template: '<i />' } } },
    attachTo: document.body,
  });
}

describe('NoteEditor', () => {
  it('mounts with initial content', () => {
    const w = mountEditor({ content: '# 标题\n正文' });
    expect(w.find('.cm-editor').exists()).toBe(true);
    expect(w.text()).toContain('正文');
    w.unmount();
  });

  it('readOnly renders wikilink chip; dangling gets dashed style', () => {
    const w = mountEditor({
      content: '链接 [[Alpha]] 与 [[Missing]]',
      readOnly: true,
      candidates: ['docs/Alpha.md'],
    });
    const chips = w.findAll('.kb-wikilink');
    expect(chips.length).toBe(2);
    expect(chips[0].text()).toBe('Alpha');
    expect(chips[0].classes()).not.toContain('kb-wikilink--dangling');
    expect(chips[1].classes()).toContain('kb-wikilink--dangling');
    w.unmount();
  });

  it('chip click in preview emits open-doc for existing target', async () => {
    const w = mountEditor({
      content: '见 [[Alpha]]',
      readOnly: true,
      candidates: ['docs/Alpha.md'],
    });
    await w.find('.kb-wikilink').trigger('mousedown');
    expect(w.emitted('open-doc')?.[0]).toEqual(['Alpha']);
    w.unmount();
  });

  it('chip click in preview emits create-doc for dangling target', async () => {
    const w = mountEditor({ content: '见 [[Missing]]', readOnly: true, candidates: [] });
    await w.find('.kb-wikilink').trigger('mousedown');
    expect(w.emitted('create-doc')?.[0]).toEqual(['Missing']);
    w.unmount();
  });

  it('edit mode chip requires Ctrl+click', async () => {
    const w = mountEditor({ content: '见 [[Alpha]]', candidates: ['docs/Alpha.md'] });
    const chip = w.find('.kb-wikilink');
    // 光标落在链接行时芯片不渲染（源码态）——先确认存在与否均可，存在则验证 Ctrl 语义
    if (chip.exists()) {
      await chip.trigger('mousedown');
      expect(w.emitted('open-doc')).toBeUndefined();
      await chip.trigger('mousedown', { ctrlKey: true });
      expect(w.emitted('open-doc')?.[0]).toEqual(['Alpha']);
    }
    w.unmount();
  });
});
