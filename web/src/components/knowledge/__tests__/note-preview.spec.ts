// NotePreview（G-04）：真渲染预览——Markdown 结构渲染、wikilink 芯片、dangling 语义、跳链事件、消毒。
import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import NotePreview from '../workbench/NotePreview.vue';

const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN',
  missing: (_l, k) => k,
  messages: { 'zh-CN': {} },
});

function mountPreview(props: { content: string; candidates?: string[] }) {
  return mount(NotePreview, {
    props,
    global: { plugins: [i18n] },
    attachTo: document.body,
  });
}

describe('NotePreview', () => {
  it('renders markdown structure (headings/table/task list), not source text', () => {
    const w = mountPreview({
      content: [
        '# 巡检手册',
        '',
        '| 机柜 | 温度 |',
        '| --- | --- |',
        '| A1 | 23 |',
        '',
        '- [ ] 检查 UPS',
        '- [x] 更换电池',
      ].join('\n'),
    });
    expect(w.find('h1').exists()).toBe(true);
    expect(w.find('h1').text()).toBe('巡检手册');
    expect(w.find('table').exists()).toBe(true);
    expect(w.find('td').text()).toBe('A1');
    const boxes = w.findAll('input[type="checkbox"]');
    expect(boxes.length).toBe(2);
    expect((boxes[1].element as HTMLInputElement).checked).toBe(true);
    // 源文本标记不再作为文本呈现。
    expect(w.text()).not.toContain('| --- |');
    w.unmount();
  });

  it('renders wikilink chips; dangling gets dashed style', () => {
    const w = mountPreview({
      content: '链接 [[Alpha]] 与 [[Missing]]',
      candidates: ['docs/Alpha.md'],
    });
    const chips = w.findAll('.kb-wikilink');
    expect(chips.length).toBe(2);
    expect(chips[0].text()).toBe('Alpha');
    expect(chips[0].classes()).not.toContain('kb-wikilink--dangling');
    expect(chips[1].classes()).toContain('kb-wikilink--dangling');
    w.unmount();
  });

  it('chip click emits open-doc for existing target', async () => {
    const w = mountPreview({ content: '见 [[Alpha]]', candidates: ['docs/Alpha.md'] });
    await w.find('.kb-wikilink').trigger('click');
    expect(w.emitted('open-doc')?.[0]).toEqual(['Alpha']);
    w.unmount();
  });

  it('chip click emits create-doc for dangling target', async () => {
    const w = mountPreview({ content: '见 [[Missing]]', candidates: [] });
    await w.find('.kb-wikilink').trigger('click');
    expect(w.emitted('create-doc')?.[0]).toEqual(['Missing']);
    w.unmount();
  });

  it('chip with alias and heading keeps heading payload', async () => {
    const w = mountPreview({ content: '见 [[Alpha#部署|部署手册]]', candidates: ['docs/Alpha.md'] });
    const chip = w.find('.kb-wikilink');
    expect(chip.text()).toBe('部署手册');
    await chip.trigger('click');
    expect(w.emitted('open-doc')?.[0]).toEqual(['Alpha', '部署']);
    w.unmount();
  });

  it('does not transform wikilinks inside code fences', () => {
    const w = mountPreview({ content: '```\n[[Alpha]]\n```', candidates: ['docs/Alpha.md'] });
    expect(w.find('.kb-wikilink').exists()).toBe(false);
    w.unmount();
  });

  it('sanitizes raw HTML/script in source', () => {
    const w = mountPreview({ content: '<script>alert(1)</script><b>ok</b>' });
    expect(w.html()).not.toContain('<script>');
    w.unmount();
  });
});
